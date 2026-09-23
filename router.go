// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"math/bits"
	"sort"
	"strings"
)

// RouteHandlerMap associates route patterns with handlers.
type RouteHandlerMap map[string]HandlerFunc

// RouteMap indexes static routes by method and normalized path.
type RouteMap map[string]map[string]*Route

// Route is the internal dispatch record for a registered static path.
type Route struct {
	handler   HandlerFunc
	infoIndex uint32
}

// Router matches exact paths through per-method maps and parameterized paths
// through compressed, per-method radix trees. Static lookup always runs first;
// cache and tree traversal are paid only when an exact route does not match.
//
// Registration derives accepted case and trailing-slash variants, composes
// route middleware, and appends stable metadata before serving begins. Router
// mutation is not safe concurrently with request dispatch.
type Router struct {
	cache  *RouteCache
	config *Config
	// routes aliases the standard-method maps in staticRoutes and owns custom methods.
	routes          RouteMap
	namedRoutes     map[string]uint32
	staticRoutes    [routeMethodCount]map[string]*Route
	staticAllowed   map[string]allowedMethodSet
	hasCustomStatic bool
	// Per-method trees keep leaves unambiguous and avoid a method branch during matching.
	dynamicRoots [routeMethodCount]*radixNode
	dynamicTrees dynamicMethodTrees
	// Metadata is append-only; routes and cache entries retain stable indexes into it.
	routeInfos        []routeMeta
	dynamicRouteCount int
	// Length masks are rejection filters only: false positives are safe, false negatives are not.
	staticRouteLens   [routeMethodCount]uint64
	staticLongMethods methodMask
}

// Add registers handlers for method and path.
func (r *Router) Add(method, path string, handlers ...HandlerFunc) error {
	return r.add(method, path, "", handlers...)
}

// AddNamed registers handlers and associates a name with the route.
func (r *Router) AddNamed(method, path, name string, handlers ...HandlerFunc) error {
	return r.add(method, path, name, handlers...)
}

// add validates first, then publishes either a static map entry or a radix leaf.
// A failed registration must not consume a metadata index or invalidate caches.
func (r *Router) add(method, path, name string, handlers ...HandlerFunc) error {
	if len(handlers) == 0 {
		return fmt.Errorf("no handler provided for %s %s", method, path)
	}
	if name != "" {
		if _, exists := r.namedRoutes[name]; exists {
			return fmt.Errorf("route name already registered: %s", name)
		}
	}

	path = r.normalizePath(path)
	if err := rejectLegacyRoutePattern(path); err != nil {
		return err
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
			return err
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

	if !isDynamic {
		if r.routes == nil {
			r.routes = make(map[string]map[string]*Route)
		}
		if mask == 0 {
			r.hasCustomStatic = true
		}
		methodRoutes := r.staticRoutesFor(method, mask)
		if methodRoutes == nil {
			methodRoutes = make(map[string]*Route)
			if slot := singleBitIndex(mask); slot >= 0 {
				r.staticRoutes[slot] = methodRoutes
			}
			r.routes[method] = methodRoutes
		}
		strictRouting := r.config != nil && r.config.StrictRouting
		caseSensitive := r.config == nil || r.config.CaseSensitive
		if staticRouteHasSingleCandidate(path, strictRouting, caseSensitive) {
			if methodRoutes[path] != nil {
				return fmt.Errorf("route already registered for %s", path)
			}
			route := &Route{
				handler:   precomposed,
				infoIndex: infoIndex,
			}
			methodRoutes[path] = route
			r.recordStaticRouteLength(mask, path)
			r.routeInfos = append(r.routeInfos, info)
			r.recordNamedRoute(name, infoIndex)
			r.invalidateCache()
			return nil
		}
		routeCandidates, routeCandidateCount := staticRouteCandidates(path, strictRouting, caseSensitive)
		for i := 0; i < routeCandidateCount; i++ {
			if methodRoutes[routeCandidates[i]] != nil {
				return fmt.Errorf("route already registered for %s", path)
			}
		}
		route := &Route{
			handler:   precomposed,
			infoIndex: infoIndex,
		}
		if r.staticAllowed == nil {
			r.staticAllowed = make(map[string]allowedMethodSet, routeCandidateCount)
		}
		// Store accepted spellings up front; request dispatch remains a direct map lookup.
		for i := 0; i < routeCandidateCount; i++ {
			candidate := routeCandidates[i]
			methodRoutes[candidate] = route
			r.recordStaticRouteLength(mask, candidate)
			r.staticAllowed[candidate] = addAllowedMethod(r.staticAllowed[candidate], method, mask)
		}
		r.routeInfos = append(r.routeInfos, info)
		r.recordNamedRoute(name, infoIndex)
		r.invalidateCache()
		return nil
	}

	route := newRadixRoute(precomposed, infoIndex, paramNames)
	tree := r.ensureDynamicTree(method, mask)
	// Publish metadata only after the tree accepts the route, preserving stable indexes on failure.
	err = tree.addBrace(path, route, r.config != nil && !r.config.CaseSensitive)
	if err != nil {
		return err
	}
	r.routeInfos = append(r.routeInfos, info)
	r.recordNamedRoute(name, infoIndex)
	r.dynamicRouteCount++
	r.invalidateCache()
	return nil
}

func (r *Router) staticRoutesFor(method string, mask methodMask) map[string]*Route {
	if slot := singleBitIndex(mask); slot >= 0 {
		return r.staticRoutes[slot]
	}
	return r.routes[method]
}

func (r *Router) recordStaticRouteLength(mask methodMask, path string) {
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

func (r *Router) hasStaticRouteLength(slot int, mask methodMask, length int) bool {
	if length >= 64 {
		return r.staticLongMethods&mask != 0
	}
	return r.staticRouteLens[slot]&(uint64(1)<<length) != 0
}

func (r *Router) recordNamedRoute(name string, index uint32) {
	if name == "" {
		return
	}
	if r.namedRoutes == nil {
		r.namedRoutes = make(map[string]uint32)
	}
	r.namedRoutes[name] = index
}

func (r *Router) invalidateCache() {
	if r.cache != nil {
		r.cache.invalidate()
	}
}

// Routes returns copies of registered route metadata in registration order.
func (r *Router) Routes() []RouteInfo {
	out := make([]RouteInfo, len(r.routeInfos))
	for i, info := range r.routeInfos {
		out[i] = info.export()
	}
	return out
}

func (r *Router) routeMetaByName(name string) (routeMeta, bool) {
	if r.namedRoutes == nil {
		return routeMeta{}, false
	}
	index, ok := r.namedRoutes[name]
	if !ok {
		return routeMeta{}, false
	}
	return r.routeMetaAt(index), true
}

type dynamicMethodTree struct {
	method string
	root   *radixNode
}

type dynamicMethodTrees []dynamicMethodTree

func (trees dynamicMethodTrees) get(method string) *radixNode {
	for i := range trees {
		if trees[i].method == method {
			return trees[i].root
		}
	}
	return nil
}

// Find resolves a route without invoking it and returns a standalone Context
// containing route metadata and parameters. Request dispatch reuses a pooled
// Context instead and calls dispatchInto directly.
func (r *Router) Find(method, path string) (HandlerFunc, *Context) {
	ctx := &Context{}
	handler := r.findInto(method, path, ctx)
	if handler == nil {
		return nil, nil
	}
	return handler, ctx
}

func (r *Router) routeMetaAt(index uint32) routeMeta {
	return r.routeInfos[index]
}

// findInto is the non-executing lookup path used by Find. It preserves the
// original path because parameter ranges must always index the caller's input,
// even when matching a normalized or case-folded candidate.
func (r *Router) findInto(method, path string, ctx *Context) HandlerFunc {
	originalPath := path
	strictRouting := r.config != nil && r.config.StrictRouting
	caseSensitive := r.config != nil && r.config.CaseSensitive
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	// Static routes are the common case and must not pay radix or cache overhead.
	if routes, ok := r.routes[method]; ok {
		if route := lookupStaticRouteExact(routes, originalPath, path); route != nil {
			ctx.setRoute(r.routeMetaAt(route.infoIndex))
			return route.handler
		}
	}
	if caseSensitive {
		if handler := r.findDynamicInto(method, path, ctx); handler != nil || path == originalPath {
			return handler
		}
		return r.findDynamicInto(method, originalPath, ctx)
	}
	if handler := r.findDynamicInto(method, path, ctx); handler != nil {
		return handler
	}
	if path != originalPath {
		if handler := r.findDynamicInto(method, originalPath, ctx); handler != nil {
			return handler
		}
	}
	if routes, ok := r.routes[method]; ok {
		if route := lookupStaticRouteLower(routes, originalPath, path); route != nil {
			ctx.setRoute(r.routeMetaAt(route.infoIndex))
			return route.handler
		}
	}
	if lower, changed := lowercasePath(path); changed {
		if handler := r.findDynamicInto(method, lower, ctx); handler != nil {
			return handler
		}
	}
	if path != originalPath {
		if lower, changed := lowercasePath(originalPath); changed {
			return r.findDynamicInto(method, lower, ctx)
		}
	}
	return nil
}

// dispatchInto resolves and invokes a route. needAllowed controls the more
// expensive alternate-method search required for 405 and automatic OPTIONS;
// ordinary not-found dispatch leaves it disabled.
//
// The lookup order is exact static, cached result, dynamic tree, case-folded
// fallback, then alternate methods. A cache entry may represent a hit or a
// known miss, but cache state never determines routing correctness.
func (r *Router) dispatchInto(method, path string, needAllowed bool, ctx *Context) (bool, allowedMethodSet, error) {
	originalPath := path
	strictRouting := r.config != nil && r.config.StrictRouting
	caseSensitive := r.config != nil && r.config.CaseSensitive
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	mask := methodMaskFor(method)
	slot := singleBitIndex(mask)
	var routes map[string]*Route
	if slot >= 0 {
		routes = r.staticRoutes[slot]
	} else {
		routes = r.routes[method]
	}
	staticLengthPossible := true
	if routes != nil && r.dynamicRouteCount != 0 && slot >= 0 {
		// The bitset can only skip impossible map probes; it never establishes a match.
		staticLengthPossible = r.hasStaticRouteLength(slot, mask, len(originalPath))
		if !staticLengthPossible && path != originalPath {
			staticLengthPossible = r.hasStaticRouteLength(slot, mask, len(path))
		}
	}
	if routes != nil && staticLengthPossible {
		if route := lookupStaticRouteExact(routes, originalPath, path); route != nil {
			ctx.setRouteIndex(route.infoIndex)
			return true, allowedMethodSet{}, route.handler(ctx)
		}
	}
	dispatchCacheEnabled := r.dispatchCacheEnabled()
	var key routeCacheKey
	if dispatchCacheEnabled {
		key = routeCacheKey{method: method, path: originalPath}
	}
	if dispatchCacheEnabled && routes != nil {
		if entry, ok := r.cache.getHot(key); ok && entry.route == nil {
			return false, entry.allowed, nil
		}
	}
	if dispatchCacheEnabled {
		if entry, ok := r.cache.getWithMask(key, mask); ok {
			if entry.route == nil {
				return false, entry.allowed, nil
			}
			ctx.applyRouteParams(originalPath, entry.route, entry.values)
			ctx.setRouteIndex(entry.route.infoIndex)
			return true, allowedMethodSet{}, entry.route.handler(ctx)
		}
	}
	// Capture offsets into the request path; materializing parameter strings would allocate.
	captured := ctx.paramRangesScratch()
	entry := r.lookupDynamicDispatch(method, mask, path, needAllowed, captured)
	if path != originalPath && entry.route == nil && entry.allowed.empty() {
		entry = r.lookupDynamicDispatch(method, mask, originalPath, needAllowed, captured)
	}
	if !caseSensitive && entry.route == nil {
		if routes := r.staticRoutesFor(method, mask); routes != nil {
			if route := lookupStaticRouteLower(routes, originalPath, path); route != nil {
				ctx.setRouteIndex(route.infoIndex)
				return true, allowedMethodSet{}, route.handler(ctx)
			}
		}
		if lower, changed := lowercasePath(path); changed {
			lowerEntry := r.lookupDynamicDispatch(method, mask, lower, needAllowed, captured)
			if path != originalPath && lowerEntry.route == nil && lowerEntry.allowed.empty() {
				if lowerOriginal, originalChanged := lowercasePath(originalPath); originalChanged {
					lowerEntry = r.lookupDynamicDispatch(method, mask, lowerOriginal, needAllowed, captured)
				}
			}
			if lowerEntry.route != nil || !lowerEntry.allowed.empty() {
				entry = lowerEntry
			}
		} else if path != originalPath {
			if lowerOriginal, changed := lowercasePath(originalPath); changed {
				lowerEntry := r.lookupDynamicDispatch(method, mask, lowerOriginal, needAllowed, captured)
				if lowerEntry.route != nil || !lowerEntry.allowed.empty() {
					entry = lowerEntry
				}
			}
		}
	}
	if needAllowed && entry.route == nil {
		entry.allowed.merge(r.lookupStaticAllowedMethods(originalPath, path, caseSensitive))
	}
	if dispatchCacheEnabled {
		if entry.route != nil {
			// Context scratch storage is request-owned; cached ranges require an independent copy.
			entry.values = cloneParamRangesForCache(entry.values, int(entry.route.paramCount))
		}
		r.cache.setMissWithMask(routeCacheKey{method: method, path: originalPath}, mask, entry)
	}
	if entry.route == nil {
		return false, entry.allowed, nil
	}
	ctx.applyRouteParams(originalPath, entry.route, entry.values)
	ctx.setRouteIndex(entry.route.infoIndex)
	return true, allowedMethodSet{}, entry.route.handler(ctx)
}

// findDynamicInto resolves one concrete path against one method tree. Cached
// parameter ranges are copied into the Context; uncached ranges use its scratch
// storage and are cloned only when the result enters the cache.
func (r *Router) findDynamicInto(method, path string, ctx *Context) HandlerFunc {
	mask := methodMaskFor(method)
	cacheEnabled := r.dynamicCacheEnabled()
	if cacheEnabled {
		key := routeCacheKey{method: method, path: path}
		if entry, ok := r.cache.getWithMask(key, mask); ok {
			if entry.route == nil {
				return nil
			}
			ctx.applyRouteParams(path, entry.route, entry.values)
			ctx.setRoute(r.routeMetaAt(entry.route.infoIndex))
			return entry.route.handler
		}
	}
	root := r.dynamicTree(method, mask)
	if root == nil {
		return nil
	}
	captured := ctx.paramRangesScratch()
	if captured == nil {
		captured = &paramRanges{}
	}
	matched := lookupDynamicRoute(root, path, captured)
	if matched == nil {
		return nil
	}
	ctx.applyRouteParams(path, matched, *captured)
	ctx.setRoute(r.routeMetaAt(matched.infoIndex))
	if cacheEnabled {
		entry := routeCacheEntry{
			route:  matched,
			values: cloneParamRangesForCache(*captured, int(matched.paramCount)),
		}
		key := routeCacheKey{method: method, path: path}
		r.cache.setMissWithMask(key, mask, entry)
	}
	return matched.handler
}

func (r *Router) dynamicCacheEnabled() bool {
	// Small trees are cheaper to walk than to synchronize through the cache.
	return r.cache != nil && r.dynamicRouteCount >= routeCacheMinRoutes
}

func (r *Router) dispatchCacheEnabled() bool {
	return r.cache != nil
}

// lookupDynamicDispatch returns the same shape stored by RouteCache so the
// caller can cache positive matches and method-aware misses without adapting it.
func (r *Router) lookupDynamicDispatch(method string, mask methodMask, path string, needAllowed bool, captured *paramRanges) routeCacheEntry {
	if r.dynamicRouteCount == 0 {
		return routeCacheEntry{}
	}
	if captured == nil {
		captured = &paramRanges{}
	}
	if root := r.dynamicTree(method, mask); root != nil {
		if route := lookupDynamicRoute(root, path, captured); route != nil {
			return routeCacheEntry{
				route:  route,
				values: *captured,
			}
		}
	}
	if !needAllowed {
		return routeCacheEntry{}
	}
	return routeCacheEntry{allowed: r.lookupAllowedInDynamicTrees(path, method)}
}

func (r *Router) hasDynamicTrees() bool {
	return r.dynamicRouteCount > 0
}

func (r *Router) dynamicTree(method string, mask methodMask) *radixNode {
	if slot := singleBitIndex(mask); slot >= 0 {
		return r.dynamicRoots[slot]
	}
	return r.dynamicTrees.get(method)
}

func (r *Router) ensureDynamicTree(method string, mask methodMask) *radixNode {
	if slot := singleBitIndex(mask); slot >= 0 {
		if r.dynamicRoots[slot] == nil {
			r.dynamicRoots[slot] = &radixNode{kind: radixRoot}
		}
		return r.dynamicRoots[slot]
	}
	if root := r.dynamicTrees.get(method); root != nil {
		return root
	}
	root := &radixNode{kind: radixRoot}
	r.dynamicTrees = append(r.dynamicTrees, dynamicMethodTree{method: method, root: root})
	return root
}

func lookupDynamicRoute(root *radixNode, path string, captured *paramRanges) *radixRoute {
	if root == nil {
		return nil
	}
	return root.lookup(path, 0, captured, 0)
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

func (r *Router) allowedMethods(path string, autoHead, autoOptions bool) []string {
	return r.lookupAllowedMethods(path).methods(autoHead, autoOptions)
}

func (r *Router) allowedMethodHeader(path string, autoHead, autoOptions bool) string {
	return r.lookupAllowedMethods(path).header(autoHead, autoOptions)
}

// lookupAllowedMethods merges static and dynamic matches for the same path.
// Automatic HEAD and OPTIONS are applied later so the stored set reflects only
// routes the application explicitly registered.
func (r *Router) lookupAllowedMethods(path string) allowedMethodSet {
	originalPath := path
	strictRouting := r.config != nil && r.config.StrictRouting
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	caseSensitive := r.config != nil && r.config.CaseSensitive

	allowed := r.lookupStaticAllowedMethods(originalPath, path, caseSensitive)
	if r.hasDynamicTrees() {
		allowed.merge(r.lookupAllowedDynamicMethods(originalPath, path, caseSensitive))
	}
	return allowed
}

func (r *Router) lookupAllowedDynamicMethods(originalPath, path string, caseSensitive bool) allowedMethodSet {
	tryLookup := func(candidate string) allowedMethodSet {
		if candidate == "" {
			return allowedMethodSet{}
		}
		return r.lookupAllowedInDynamicTrees(candidate, "")
	}

	if allowed := tryLookup(originalPath); !allowed.empty() {
		return allowed
	}
	if path != originalPath {
		if allowed := tryLookup(path); !allowed.empty() {
			return allowed
		}
	}
	if caseSensitive {
		return allowedMethodSet{}
	}
	if lower, changed := lowercasePath(originalPath); changed {
		if allowed := tryLookup(lower); !allowed.empty() {
			return allowed
		}
	}
	if path != originalPath {
		if lower, changed := lowercasePath(path); changed {
			if allowed := tryLookup(lower); !allowed.empty() {
				return allowed
			}
		}
	}
	return allowedMethodSet{}
}

func (r *Router) lookupAllowedInDynamicTrees(path, excludeMethod string) allowedMethodSet {
	if !r.hasAlternateDynamicMethods(excludeMethod) {
		return allowedMethodSet{}
	}
	return r.lookupAllowedInDynamicRoots(path, excludeMethod)
}

func (r *Router) hasAlternateDynamicMethods(excludeMethod string) bool {
	for slot, root := range r.dynamicRoots {
		if root == nil {
			continue
		}
		if routeMethods[slot] != excludeMethod {
			return true
		}
	}
	for i := range r.dynamicTrees {
		tree := r.dynamicTrees[i]
		if tree.root != nil && tree.method != excludeMethod {
			return true
		}
	}
	return false
}

func (r *Router) lookupAllowedInDynamicRoots(path, excludeMethod string) allowedMethodSet {
	var allowed allowedMethodSet
	for slot, root := range r.dynamicRoots {
		if root == nil {
			continue
		}
		method := routeMethods[slot]
		if method == excludeMethod {
			continue
		}
		if root.matchesPath(path, 0) {
			allowed.addMethod(method)
		}
	}
	for i := range r.dynamicTrees {
		tree := r.dynamicTrees[i]
		if tree.method == excludeMethod || tree.root == nil {
			continue
		}
		if tree.root.matchesPath(path, 0) {
			allowed.addMethod(tree.method)
		}
	}
	return allowed
}

func (r *Router) lookupStaticAllowedMethods(originalPath, path string, caseSensitive bool) allowedMethodSet {
	if len(r.staticAllowed) != 0 {
		if allowed := lookupStaticAllowed(r.staticAllowed, originalPath, path, caseSensitive); !allowed.empty() {
			return allowed
		}
	}
	return r.lookupStaticAllowedByScan(originalPath, path, caseSensitive)
}

func lookupStaticRouteExact(methodRoutes map[string]*Route, originalPath, path string) *Route {
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

func lookupStaticRouteLower(methodRoutes map[string]*Route, originalPath, path string) *Route {
	if len(methodRoutes) == 0 {
		return nil
	}
	if lower, changed := lowercasePath(originalPath); changed {
		if route := methodRoutes[lower]; route != nil {
			return route
		}
	}
	if path != originalPath {
		if lower, changed := lowercasePath(path); changed {
			if route := methodRoutes[lower]; route != nil {
				return route
			}
		}
	}
	return nil
}

func lookupStaticAllowed(staticAllowed map[string]allowedMethodSet, originalPath, path string, caseSensitive bool) allowedMethodSet {
	if len(staticAllowed) == 0 {
		return allowedMethodSet{}
	}
	if allowed := staticAllowed[originalPath]; !allowed.empty() {
		return allowed
	}
	if path != originalPath {
		if allowed := staticAllowed[path]; !allowed.empty() {
			return allowed
		}
	}
	if caseSensitive {
		return allowedMethodSet{}
	}
	if lower, changed := lowercasePath(originalPath); changed {
		if allowed := staticAllowed[lower]; !allowed.empty() {
			return allowed
		}
	}
	if path != originalPath {
		if lower, changed := lowercasePath(path); changed {
			if allowed := staticAllowed[lower]; !allowed.empty() {
				return allowed
			}
		}
	}
	return allowedMethodSet{}
}

// lookupStaticAllowedByScan is the fallback for routes that have a single
// registration candidate and therefore do not need entries in staticAllowed.
func (r *Router) lookupStaticAllowedByScan(originalPath, path string, caseSensitive bool) allowedMethodSet {
	if len(r.routes) == 0 {
		return allowedMethodSet{}
	}
	var allowed allowedMethodSet
	for slot, method := range routeMethods {
		routes := r.staticRoutes[slot]
		if len(routes) == 0 {
			continue
		}
		if lookupStaticRouteExact(routes, originalPath, path) != nil {
			allowed.mask |= methodMaskFor(method)
			continue
		}
		if !caseSensitive && lookupStaticRouteLower(routes, originalPath, path) != nil {
			allowed.mask |= methodMaskFor(method)
		}
	}
	if !r.hasCustomStatic {
		return allowed
	}
	var custom []string
	for method := range r.routes {
		if methodMaskFor(method) == 0 {
			custom = append(custom, method)
		}
	}
	if len(custom) == 0 {
		return allowed
	}
	sort.Strings(custom)
	for _, method := range custom {
		routes := r.routes[method]
		if len(routes) == 0 {
			continue
		}
		if lookupStaticRouteExact(routes, originalPath, path) != nil {
			allowed.addMethod(method)
			continue
		}
		if !caseSensitive && lookupStaticRouteLower(routes, originalPath, path) != nil {
			allowed.addMethod(method)
		}
	}
	return allowed
}

// staticRouteCandidates expands configuration-dependent spellings during
// registration. Four slots cover original, slash-normalized, case-folded, and
// case-folded-plus-normalized forms without allocating a temporary slice.
func staticRouteCandidates(path string, strictRouting, caseSensitive bool) ([4]string, int) {
	var candidates [4]string
	count := 0
	addCandidate := func(candidate string) {
		if candidate == "" {
			return
		}
		for i := 0; i < count; i++ {
			if candidates[i] == candidate {
				return
			}
		}
		candidates[count] = candidate
		count++
	}

	addCandidate(path)
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		addCandidate(path[:len(path)-1])
	}
	if caseSensitive {
		return candidates, count
	}
	if lower, changed := lowercasePath(path); changed {
		addCandidate(lower)
		if !strictRouting && len(lower) > 1 && lower[len(lower)-1] == '/' {
			addCandidate(lower[:len(lower)-1])
		}
	}
	return candidates, count
}

func staticRouteHasSingleCandidate(path string, strictRouting, caseSensitive bool) bool {
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		return false
	}
	if caseSensitive {
		return true
	}
	for i := 0; i < len(path); i++ {
		c := path[i]
		if (c >= 'A' && c <= 'Z') || c >= 0x80 {
			return false
		}
	}
	return true
}

func addAllowedMethod(allowed allowedMethodSet, method string, mask methodMask) allowedMethodSet {
	if mask != 0 {
		allowed.mask |= mask
		return allowed
	}
	allowed.addMethod(method)
	return allowed
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

func (s *allowedMethodSet) removeMethod(method string) {
	if method == "" {
		return
	}
	if mask := methodMaskFor(method); mask != 0 {
		s.mask &^= mask
		return
	}
	for i, existing := range s.extra {
		if existing != method {
			continue
		}
		copy(s.extra[i:], s.extra[i+1:])
		s.extra = s.extra[:len(s.extra)-1]
		return
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

func (s allowedMethodSet) methods(autoHead, autoOptions bool) []string {
	s = s.withAutomatic(autoHead, autoOptions)
	if s.empty() {
		return nil
	}
	allowed := make([]string, 0, len(routeMethods)+len(s.extra))
	for _, method := range routeMethods {
		if s.mask&methodMaskFor(method) == 0 {
			continue
		}
		allowed = append(allowed, method)
	}
	allowed = append(allowed, s.extra...)
	return allowed
}

func (s allowedMethodSet) header(autoHead, autoOptions bool) string {
	s = s.withAutomatic(autoHead, autoOptions)
	if s.empty() {
		return ""
	}
	if len(s.extra) == 0 {
		return allowHeader(s.mask)
	}
	return buildAllowHeaderWithExtra(s.mask, s.extra)
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
