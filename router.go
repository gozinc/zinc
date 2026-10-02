// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"fmt"
	"math/bits"
	"strings"
	"unicode/utf8"

	"github.com/0mjs/zinc/internal/preflight"
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
	// routeDocs holds OpenAPI metadata by route index, for documented routes
	// only. Dispatch never reads it, and it's the last field so adding it
	// moved nothing dispatch does read.
	routeDocs map[uint32]*routeDoc
	// docsVersion counts changes to route metadata after registration, such
	// as Hidden or Summary, so a served spec knows when to rebuild.
	docsVersion uint64
	// entries and handlers hold each route's tree entry and its handler as
	// registered, by route index, so Route.Status can wrap the handler.
	// Dispatch never reads them.
	entries  []*radixRoute
	handlers []HandlerFunc
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

// register validates first, then records the route in the tree (and a static
// route's exact spellings in the maps), and returns its metadata index. A failed registration
// must not consume a metadata index.
func (r *routeTable) register(method, path, name string, handlers ...HandlerFunc) (uint32, error) {
	if len(handlers) == 0 {
		return 0, fmt.Errorf("no handler provided for %s %s", method, path)
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

	// The tree checks for conflicts and records the route; it is the only
	// place a registration can fail from here on.
	route := newRadixRoute(&r.arena, precomposed, infoIndex, paramNames)
	if err := r.addToTree(method, mask, path, isDynamic, route); err != nil {
		return 0, err
	}
	if !isDynamic {
		r.addStaticSpellings(method, mask, path, route)
	}
	r.routeInfos = append(r.routeInfos, info)
	r.entries = append(r.entries, route)
	r.handlers = append(r.handlers, precomposed)
	for _, h := range handlers[:len(handlers)-1] {
		if preflight.Is(h) {
			if r.preflight == nil {
				r.preflight = map[string][]HandlerFunc{}
			}
			key := method + " " + registeredPath
			r.preflight[key] = append(r.preflight[key], h)
		}
	}
	if types, ok := describeHandler(finalHandler); ok {
		doc := r.doc(infoIndex)
		doc.in, doc.out, doc.typed = types.in, types.out, true
	}
	r.recordNamedRoute(name, infoIndex)
	return infoIndex, nil
}

// addStaticSpellings records a static route's exact spellings for the map
// fast path: the registered path and, unless routing is strict, the path
// without its trailing slash. Case-folded spellings are left to the tree.
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
	methodRoutes[path] = entry
	r.recordStaticRouteLength(mask, path)
	strictRouting := r.config != nil && r.config.StrictRouting
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		methodRoutes[path[:len(path)-1]] = entry
		r.recordStaticRouteLength(mask, path[:len(path)-1])
	}
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
// Context instead and calls dispatchInto directly.
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

// findInto is the non-executing lookup used by Find (router_tree.go).
func (r *routeTable) findInto(method, path string, ctx *Context) HandlerFunc {
	return r.findTree(method, path, ctx)
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

// dispatchInto resolves and invokes a route (router_tree.go). needAllowed
// asks for the methods that match on a miss, for 405 and automatic OPTIONS;
// ordinary not-found dispatch leaves it off.
func (r *routeTable) dispatchInto(method, path string, needAllowed bool, ctx *Context) (bool, allowedMethodSet, error) {
	return r.dispatchTree(method, path, needAllowed, ctx)
}

type allowedMethodSet struct {
	// Standard methods use a mask; extension methods allocate only when present.
	mask  methodMask
	extra []string
}

var routeMethods = []string{
	MethodGet,
	MethodHead,
	MethodPost,
	MethodPut,
	MethodPatch,
	MethodDelete,
	MethodOptions,
	MethodConnect,
	MethodTrace,
}

const routeMethodCount = 9
const indexedParamThreshold = 10

type methodMask uint16

const (
	methodMaskGet methodMask = 1 << iota
	methodMaskHead
	methodMaskPost
	methodMaskPut
	methodMaskPatch
	methodMaskDelete
	methodMaskOptions
	methodMaskConnect
	methodMaskTrace
)

const allowHeaderTableSize = 1 << 9

var allowHeaderByMask [allowHeaderTableSize]string

func init() {
	// Every standard-method combination has a canonical, allocation-free Allow value.
	for raw := 0; raw < len(allowHeaderByMask); raw++ {
		allowHeaderByMask[raw] = buildAllowHeader(methodMask(raw))
	}
}

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

func singleBitIndex(mask methodMask) int {
	raw := uint16(mask)
	if raw == 0 || raw&(raw-1) != 0 {
		return -1
	}
	index := bits.TrailingZeros16(raw)
	if index >= routeMethodCount {
		return -1
	}
	return index
}

func (s *allowedMethodSet) addMethod(method string) {
	if method == "" {
		return
	}
	if mask := methodMaskFor(method); mask != 0 {
		s.mask |= mask
		return
	}
	for _, existing := range s.extra {
		if existing == method {
			return
		}
	}
	s.extra = append(s.extra, method)
}

func (s *allowedMethodSet) merge(other allowedMethodSet) {
	s.mask |= other.mask
	for _, method := range other.extra {
		s.addMethod(method)
	}
}

func (s allowedMethodSet) empty() bool {
	return s.mask == 0 && len(s.extra) == 0
}

func (s allowedMethodSet) withAutomatic(autoHead, autoOptions bool) allowedMethodSet {
	if s.empty() {
		return allowedMethodSet{}
	}
	if autoHead && s.mask&methodMaskGet != 0 {
		s.mask |= methodMaskHead
	}
	if autoOptions {
		s.mask |= methodMaskOptions
	}
	return s
}

func (s allowedMethodSet) header(autoHead, autoOptions bool) string {
	s = s.withAutomatic(autoHead, autoOptions)
	if s.empty() {
		return ""
	}
	if len(s.extra) == 0 {
		return allowHeader(s.mask)
	}
	return buildAllowHeaderWithExtra(s.mask, sortedExtra(s.extra))
}

func methodMaskFor(method string) methodMask {
	switch method {
	case MethodGet:
		return methodMaskGet
	case MethodHead:
		return methodMaskHead
	case MethodPost:
		return methodMaskPost
	case MethodPut:
		return methodMaskPut
	case MethodPatch:
		return methodMaskPatch
	case MethodDelete:
		return methodMaskDelete
	case MethodOptions:
		return methodMaskOptions
	case MethodConnect:
		return methodMaskConnect
	case MethodTrace:
		return methodMaskTrace
	default:
		return 0
	}
}

func allowHeader(mask methodMask) string {
	if mask == 0 {
		return ""
	}
	if int(mask) < len(allowHeaderByMask) {
		return allowHeaderByMask[int(mask)]
	}
	return buildAllowHeader(mask)
}

func buildAllowHeader(mask methodMask) string {
	if mask == 0 {
		return ""
	}
	return buildAllowHeaderWithExtra(mask, nil)
}

func buildAllowHeaderWithExtra(mask methodMask, extra []string) string {
	if mask == 0 && len(extra) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, method := range routeMethods {
		if mask&methodMaskFor(method) == 0 {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(method)
	}
	for _, method := range extra {
		if builder.Len() > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(method)
	}
	return builder.String()
}

// addToTree records route in the route tree. A static route registered with
// a trailing slash is also reachable without it unless routing is strict, as
// its map spellings made it before.
func (r *routeTable) addToTree(method string, mask methodMask, path string, isDynamic bool, route *radixRoute) error {
	if r.tree == nil {
		r.tree = r.arena.node()
		r.tree.kind = radixRoot
	}
	strictRouting := r.config != nil && r.config.StrictRouting
	caseInsensitive := r.config != nil && !r.config.CaseSensitive
	paths := [2]string{path}
	count := 1
	if !isDynamic && !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		paths[1] = path[:len(path)-1]
		count = 2
	}
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
		if nodes[i].methods != nil && nodes[i].methods.get(slot, method) != nil {
			return fmt.Errorf("route already registered for %s", path)
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
