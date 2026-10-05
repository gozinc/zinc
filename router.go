// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// routeMap indexes static routes by method and exact spelling. Its values are
// the same route objects the tree holds.
type routeMap map[string]map[string]*radixRoute

// routeTable matches every route through one compressed radix tree (tree,
// router_tree.go) whose terminals carry a method table. Exact-spelling static
// hits take a map probe first, which beats walking the tree's shared prefixes.
//
// Registration derives the trailing-slash spelling, composes route
// middleware, and appends stable metadata before serving begins. routeTable
// mutation is not safe concurrently with request dispatch.
type routeTable struct {
	config *Config
	// routes aliases the standard-method maps in staticRoutes and owns custom
	// methods. They hold exact spellings only: the registered path and, unless
	// routing is strict, the path without its trailing slash.
	routes       routeMap
	namedRoutes  map[string]uint32
	staticRoutes [routeMethodCount]map[string]*radixRoute
	tree         *radixNode
	arena        treeArena
	// Metadata is append-only; routes retain stable indexes into it.
	routeInfos []routeMeta
	// Length masks are rejection filters only: false positives are safe, false negatives are not.
	staticRouteLens   [routeMethodCount]uint64
	staticLongMethods methodMask
	// Route lookup (resolve) reads none of the fields from here on; keep the
	// fields it reads above them, together.
	//
	// routeDocs holds OpenAPI metadata by route index, for documented routes
	// only.
	routeDocs map[uint32]*routeDoc
	// docsVersion counts changes to route metadata after registration, such
	// as Hidden or Summary, so a served spec knows when to rebuild.
	docsVersion uint64
	// entries and handlers hold each route's tree entry and its handler as
	// registered, by route index, so Route.Status can wrap the handler.
	entries  []*radixRoute
	handlers []HandlerFunc
	// patterns caches each route's net/http pattern, "METHOD /path", by route
	// index. Only routes reached through Wrap or FromHTTP need one, so it's
	// built on first use rather than at registration; see pattern.
	patterns []atomic.Pointer[string]
	// preflight holds, by "METHOD path", a route's middleware that also
	// answers CORS preflight requests, outermost first. Only automatic
	// OPTIONS reads it.
	preflight map[string][]HandlerFunc
}

// setDefaultStatus makes code the status of whatever the route writes,
// unless a handler sets another: the route's handler is wrapped to set it
// before the route's middleware and handlers run.
func (r *routeTable) setDefaultStatus(index uint32, code int) {
	entry, base := r.entries[index], r.handlers[index]
	entry.handler = func(c *Context) error {
		c.status = code
		return base(c)
	}
}

// Add registers handlers for method and path.
func (r *routeTable) Add(method, path string, handlers ...HandlerFunc) error {
	return r.add(method, path, "", handlers...)
}

// AddNamed registers handlers and associates a name with the route.
func (r *routeTable) AddNamed(method, path, name string, handlers ...HandlerFunc) error {
	return r.add(method, path, name, handlers...)
}

func (r *routeTable) add(method, path, name string, handlers ...HandlerFunc) error {
	_, err := r.register(method, path, name, handlers...)
	return err
}

// register validates first, then records the route in the tree (and a
// static route's exact spellings in the maps), and returns its metadata
// index. A failed registration doesn't consume a metadata index.
func (r *routeTable) register(method, path, name string, handlers ...HandlerFunc) (uint32, error) {
	if len(handlers) == 0 {
		return 0, fmt.Errorf("no handler provided for %s %s", method, path)
	}
	for _, h := range handlers {
		if h == nil {
			return 0, fmt.Errorf("nil handler for %s %s", method, path)
		}
	}
	if name != "" {
		if _, exists := r.namedRoutes[name]; exists {
			return 0, fmt.Errorf("route name already registered: %s", name)
		}
	}

	path = r.normalizePath(path)
	if err := rejectLegacyRoutePattern(path); err != nil {
		return 0, err
	}
	registeredPath := path
	bracePattern := strings.ContainsAny(path, "{}")
	var (
		paramNames collectedRouteParams
		err        error
	)
	if bracePattern {
		paramNames, err = collectBraceRouteParams(path)
		if err != nil {
			return 0, err
		}
		if paramNames.count > maxRouteParams {
			return 0, fmt.Errorf("route %s %s has %d parameters; a route can have at most %d", method, path, paramNames.count, maxRouteParams)
		}
	}
	isDynamic := bracePattern

	finalHandler := handlers[len(handlers)-1]
	precomposed := finalHandler
	if len(handlers) > 1 {
		// Compose once at registration so dispatch does not allocate a route-local chain.
		chain := append([]HandlerFunc(nil), handlers...)
		precomposed = func(c *Context) error {
			c.setHandlers(chain)
			return c.Next()
		}
	}

	infoIndex := uint32(len(r.routeInfos))
	info := newRouteMeta(method, registeredPath, name, finalHandler, paramNames.slice(), false)
	mask := methodMaskFor(method)

	types, typed := describeHandler(finalHandler)
	if typed {
		if err := ruleSupportError(types, r.config); err != nil {
			panic(err.Error())
		}
	}
	// The tree checks for conflicts and records the route; it is the only
	// place a registration can fail from here on.
	route := newRadixRoute(&r.arena, precomposed, infoIndex, paramNames)
	route.catchAll = strings.HasSuffix(path, "...}")
	if err := r.addToTree(method, mask, path, route); err != nil {
		return 0, err
	}
	if !isDynamic {
		r.addStaticSpellings(method, mask, path, route)
	}
	r.routeInfos = append(r.routeInfos, info)
	r.entries = append(r.entries, route)
	r.handlers = append(r.handlers, precomposed)
	// A slot per route, filled on first use. Growing copies the slots, which
	// is safe because registration doesn't run while requests are served.
	r.patterns = slices.Grow(r.patterns, 1)[:len(r.patterns)+1]
	for _, h := range handlers[:len(handlers)-1] {
		if middlewareMarks(h).Preflight {
			if r.preflight == nil {
				r.preflight = map[string][]HandlerFunc{}
			}
			key := method + " " + registeredPath
			r.preflight[key] = append(r.preflight[key], h)
		}
	}
	if typed {
		doc := r.doc(infoIndex)
		doc.in, doc.out, doc.typed = types.in, types.out, true
	}
	r.recordNamedRoute(name, infoIndex)
	return infoIndex, nil
}

// addStaticSpellings records a static route's exact spellings for the map
// fast path (routeSpellings). Case-folded spellings are left to the tree.
func (r *routeTable) addStaticSpellings(method string, mask methodMask, path string, entry *radixRoute) {
	if r.routes == nil {
		r.routes = make(routeMap)
	}
	methodRoutes := r.routes[method]
	if methodRoutes == nil {
		methodRoutes = make(map[string]*radixRoute)
		if slot := singleBitIndex(mask); slot >= 0 {
			r.staticRoutes[slot] = methodRoutes
		}
		r.routes[method] = methodRoutes
	}
	spellings, count := r.routeSpellings(path)
	for _, spelling := range spellings[:count] {
		methodRoutes[spelling] = entry
		r.recordStaticRouteLength(mask, spelling)
	}
}

// routeSpellings returns the paths a route is recorded under: its pattern
// and, unless routing is strict, the pattern without its trailing slash
// (trimTrailingSlash).
func (r *routeTable) routeSpellings(path string) ([2]string, int) {
	if trimmed := r.trimTrailingSlash(path); trimmed != path {
		return [2]string{path, trimmed}, 2
	}
	return [2]string{path}, 1
}

func (r *routeTable) recordStaticRouteLength(mask methodMask, path string) {
	// One bit per length avoids a map probe when mixed static/dynamic trees make
	// misses common. Paths of 64 bytes or more share the conservative long bit.
	slot := singleBitIndex(mask)
	if slot < 0 {
		return
	}
	length := len(path)
	if length >= 64 {
		r.staticLongMethods |= mask
		return
	}
	r.staticRouteLens[slot] |= uint64(1) << length
}

func (r *routeTable) hasStaticRouteLength(slot int, mask methodMask, length int) bool {
	if length >= 64 {
		return r.staticLongMethods&mask != 0
	}
	return r.staticRouteLens[slot]&(uint64(1)<<length) != 0
}

// nameRoute gives an existing route a unique name, replacing any earlier one.
func (r *routeTable) nameRoute(index uint32, name string) error {
	if name == "" {
		return errors.New("route name is empty")
	}
	if existing, ok := r.namedRoutes[name]; ok && existing != index {
		return fmt.Errorf("route name already registered: %s", name)
	}
	if previous := r.routeInfos[index].name; previous != "" && previous != name {
		delete(r.namedRoutes, previous)
	}
	r.routeInfos[index].name = name
	r.recordNamedRoute(name, index)
	return nil
}

func (r *routeTable) recordNamedRoute(name string, index uint32) {
	if name == "" {
		return
	}
	if r.namedRoutes == nil {
		r.namedRoutes = make(map[string]uint32)
	}
	r.namedRoutes[name] = index
}

// Routes returns copies of registered route metadata in registration order.
func (r *routeTable) Routes() []RouteInfo {
	out := make([]RouteInfo, len(r.routeInfos))
	for i, info := range r.routeInfos {
		out[i] = info.export()
	}
	return out
}

func (r *routeTable) routeMetaByName(name string) (routeMeta, bool) {
	if r.namedRoutes == nil {
		return routeMeta{}, false
	}
	index, ok := r.namedRoutes[name]
	if !ok {
		return routeMeta{}, false
	}
	return r.routeMetaAt(index), true
}

// Find resolves a route without invoking it and returns a standalone Context
// containing route metadata and parameters. Request dispatch reuses a pooled
// Context instead and calls dispatchInto.
func (r *routeTable) Find(method, path string) (HandlerFunc, *Context) {
	ctx := &Context{}
	handler := r.findInto(method, path, ctx)
	if handler == nil {
		return nil, nil
	}
	return handler, ctx
}

func (r *routeTable) routeMetaAt(index uint32) routeMeta {
	return r.routeInfos[index]
}

// Unicode lowercasing can change byte widths (for example K to k). Walk both
// spellings once, mapping ordered parameter boundaries back to original bytes.
func remapFoldedParams(values *paramRanges, count int, original, folded string) {
	if original == folded {
		return
	}
	ascii := true
	for i := 0; i < len(original); i++ {
		if original[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return
	}
	oi, fi := 0, 0
	offset := func(target uint32) uint32 {
		for fi < int(target) && oi < len(original) && fi < len(folded) {
			_, os := utf8.DecodeRuneInString(original[oi:])
			_, fs := utf8.DecodeRuneInString(folded[fi:])
			oi += os
			fi += fs
		}
		return uint32(oi)
	}
	for i := 0; i < count; i++ {
		v := values.at(i)
		start := offset(v.start)
		end := offset(v.end)
		values.set(i, paramRange{start: start, end: end})
	}
}

const indexedParamThreshold = 10

// maxRouteParams is the most parameters a route can have: a wide route's
// name index (radixRoute.paramIndices) holds each position in a byte.
const maxRouteParams = 256

func lookupStaticRouteExact(methodRoutes map[string]*radixRoute, originalPath, path string) *radixRoute {
	if len(methodRoutes) == 0 {
		return nil
	}
	if route := methodRoutes[originalPath]; route != nil {
		return route
	}
	if path != originalPath {
		if route := methodRoutes[path]; route != nil {
			return route
		}
	}
	return nil
}

// addToTree records route in the route tree under each of its spellings
// (routeSpellings). Two routes for one method that share a spelling would
// match the same requests, so the second is rejected, naming both.
func (r *routeTable) addToTree(method string, mask methodMask, path string, route *radixRoute) error {
	if r.tree == nil {
		r.tree = r.arena.node()
		r.tree.kind = radixRoot
	}
	caseInsensitive := r.config != nil && !r.config.CaseSensitive
	paths, count := r.routeSpellings(path)
	slot := singleBitIndex(mask)
	// Create every spelling's node first: creating one can split a node
	// another spelling returned. Then look them up again (which creates
	// nothing) and check them all before recording any, so a conflict leaves
	// no trace of the route in the tree.
	var nodes [2]*radixNode
	for i := 0; i < count; i++ {
		r.tree.nodeFor(&r.arena, paths[i], caseInsensitive)
	}
	for i := 0; i < count; i++ {
		nodes[i] = r.tree.nodeFor(&r.arena, paths[i], caseInsensitive)
		if nodes[i].methods == nil {
			continue
		}
		if existing := nodes[i].methods.get(slot, method); existing != nil {
			return r.conflictError(method, path, existing)
		}
	}
	for i := 0; i < count; i++ {
		if nodes[i].methods == nil {
			nodes[i].methods = r.arena.methodTable()
		}
		nodes[i].methods.set(slot, method, route)
	}
	return nil
}

// conflictError reports a route that matches the same requests as an
// existing one.
func (r *routeTable) conflictError(method, path string, existing *radixRoute) error {
	other := r.routeMetaAt(existing.infoIndex)
	if other.path == path {
		return fmt.Errorf("route already registered: %s %s", method, path)
	}
	return fmt.Errorf("route already registered: %s %s matches the same requests as %s %s", method, path, other.method, other.path)
}

// pattern returns the net/http pattern of the route at index, building it
// the first time it's asked for.
func (r *routeTable) pattern(index uint32) string {
	slot := &r.patterns[index]
	if p := slot.Load(); p != nil {
		return *p
	}
	meta := r.routeInfos[index]
	p := meta.method + " " + meta.path
	slot.Store(&p)
	return p
}
