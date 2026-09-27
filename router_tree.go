// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import "sort"

// The route tree holds every route of a routeTable, static and parameterized,
// for every method, in one compressed radix tree. A terminal node carries a
// method table, so one walk finds the route for a method, and a miss can
// collect every method the path allows. Static routes are the tree's
// highest-priority edges, so matching keeps the precedence the separate
// static maps and per-method trees gave: static, then parameter, then
// catch-all, with backtracking. See audits/2026-09-26/p5-router-design.md.

// nodeMethods is a terminal node's routes, by method.
type nodeMethods struct {
	std    [routeMethodCount]*radixRoute
	custom []customRoute
}

type customRoute struct {
	method string
	route  *radixRoute
}

func (m *nodeMethods) get(slot int, method string) *radixRoute {
	if slot >= 0 {
		return m.std[slot]
	}
	for i := range m.custom {
		if m.custom[i].method == method {
			return m.custom[i].route
		}
	}
	return nil
}

// set records route for the method, reporting false if it already has one.
func (m *nodeMethods) set(slot int, method string, route *radixRoute) bool {
	if m.get(slot, method) != nil {
		return false
	}
	if slot >= 0 {
		m.std[slot] = route
		return true
	}
	m.custom = append(m.custom, customRoute{method: method, route: route})
	return true
}

// addAllowed adds the methods whose route here takes captured parameters,
// static routes to static and parameterized ones to dynamic.
func (m *nodeMethods) addAllowed(static, dynamic *allowedMethodSet, captured int) {
	into := dynamic
	if captured == 0 {
		into = static
	}
	for slot, route := range m.std {
		if route != nil && int(route.paramCount) == captured {
			into.mask |= methodMaskFor(routeMethods[slot])
		}
	}
	for _, c := range m.custom {
		if int(c.route.paramCount) == captured {
			into.addMethod(c.method)
		}
	}
}

// nodeFor walks the static, parameter and catch-all edges of a validated
// pattern, creating them as needed, and returns the terminal node. Static
// labels are lowercased when routing is case-insensitive.
func (n *radixNode) nodeFor(path string, caseInsensitive bool) *radixNode {
	current := n
	staticStart := 0
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			continue
		}
		if i > staticStart {
			current = current.addStaticPath(foldLiteral(path[staticStart:i], caseInsensitive))
		}
		end := i + 1 + indexByte(path[i+1:], '}')
		if rawName := path[i+1 : end]; len(rawName) > 3 && rawName[len(rawName)-3:] == "..." {
			current = current.addCatchAllChild()
		} else {
			current = current.addParamChild()
		}
		staticStart = end + 1
		i = end
	}
	if staticStart < len(path) {
		current = current.addStaticPath(foldLiteral(path[staticStart:], caseInsensitive))
	}
	return current
}

func foldLiteral(literal string, caseInsensitive bool) string {
	if caseInsensitive {
		literal, _ = lowercasePath(literal)
	}
	return literal
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// methodRoute is this node's route for the method, if it takes captured
// parameters.
func (n *radixNode) methodRoute(slot int, method string, captured int) *radixRoute {
	if n.methods == nil {
		return nil
	}
	route := n.methods.get(slot, method)
	if route == nil || int(route.paramCount) != captured {
		return nil
	}
	return route
}

// lookupMethod is lookup for one method in the route tree. A terminal without
// a route for the method is not a match, so the walk backtracks past it, as
// a per-method tree would never have entered that branch.
func (n *radixNode) lookupMethod(path string, offset int, values *paramRanges, captured int, fold bool, slot int, method string) *radixRoute {
	switch n.kind {
	case radixStatic:
		if fold {
			if !hasFoldedPrefix(path, n.prefix) {
				return nil
			}
		} else if len(path) < len(n.prefix) || path[:len(n.prefix)] != n.prefix {
			return nil
		}
		offset += len(n.prefix)
		path = path[len(n.prefix):]
	case radixParam:
		if len(path) == 0 || path[0] == '/' {
			return nil
		}
		end := nextSlash(path)
		if end < 0 {
			end = len(path)
		}
		values.set(captured, paramRange{start: uint32(offset), end: uint32(offset + end)})
		captured++
		offset += end
		path = path[end:]
	case radixCatchAll:
		start := offset
		if len(path) > 0 && path[0] == '/' {
			start++
		}
		values.set(captured, paramRange{start: uint32(start), end: uint32(offset + len(path))})
		return n.methodRoute(slot, method, captured+1)
	}
	if len(path) == 0 {
		if matched := n.methodRoute(slot, method, captured); matched != nil {
			return matched
		}
		if n.catchAllChild != nil {
			return n.catchAllChild.lookupMethod(path, offset, values, captured, fold, slot, method)
		}
		return nil
	}
	first := path[0]
	if fold {
		first = foldByte(first)
	}
	if idx := n.staticChildIndex(first); idx >= 0 {
		if matched := n.children[idx].lookupMethod(path, offset, values, captured, fold, slot, method); matched != nil {
			return matched
		}
	}
	if n.paramChild != nil {
		if matched := n.paramChild.lookupMethod(path, offset, values, captured, fold, slot, method); matched != nil {
			return matched
		}
	}
	if n.catchAllChild != nil {
		return n.catchAllChild.lookupMethod(path, offset, values, captured, fold, slot, method)
	}
	return nil
}

// collectAllowed adds every method of every terminal that matches path. It
// explores all branches rather than stopping at the first match, so Allow is
// the union of the methods that could serve the path.
func (n *radixNode) collectAllowed(path string, captured int, fold bool, static, dynamic *allowedMethodSet) {
	switch n.kind {
	case radixStatic:
		if fold {
			if !hasFoldedPrefix(path, n.prefix) {
				return
			}
		} else if len(path) < len(n.prefix) || path[:len(n.prefix)] != n.prefix {
			return
		}
		path = path[len(n.prefix):]
	case radixParam:
		if len(path) == 0 || path[0] == '/' {
			return
		}
		end := nextSlash(path)
		if end < 0 {
			end = len(path)
		}
		captured++
		path = path[end:]
	case radixCatchAll:
		if n.methods != nil {
			n.methods.addAllowed(static, dynamic, captured+1)
		}
		return
	}
	if len(path) == 0 {
		if n.methods != nil {
			n.methods.addAllowed(static, dynamic, captured)
		}
		if n.catchAllChild != nil {
			n.catchAllChild.collectAllowed(path, captured, fold, static, dynamic)
		}
		return
	}
	first := path[0]
	if fold {
		first = foldByte(first)
	}
	if idx := n.staticChildIndex(first); idx >= 0 {
		n.children[idx].collectAllowed(path, captured, fold, static, dynamic)
	}
	if n.paramChild != nil {
		n.paramChild.collectAllowed(path, captured, fold, static, dynamic)
	}
	if n.catchAllChild != nil {
		n.catchAllChild.collectAllowed(path, captured, fold, static, dynamic)
	}
}

// sortedExtra returns custom methods in sorted order for Allow.
func sortedExtra(extra []string) []string {
	if len(extra) < 2 || sort.StringsAreSorted(extra) {
		return extra
	}
	sorted := append([]string(nil), extra...)
	sort.Strings(sorted)
	return sorted
}

// treeMatchPath prepares path for a walk of the route tree: an ASCII path
// with capital letters is walked in place with folded compares; any other
// path is lowercased (a no-op for lowercase ASCII) unless routing is
// case-sensitive.
func treeMatchPath(path string, caseSensitive bool) (matched string, fold bool) {
	if caseSensitive {
		return path, false
	}
	if hasUpper, ascii := asciiFoldKind(path); ascii {
		return path, hasUpper
	}
	matched, _ = lowercasePath(path)
	return matched, false
}

// dispatchTree resolves and invokes a route through the route tree. Order:
// the opt-in cache, the walk for the path without its trailing slash, then
// (only when that found no route and no other methods) the path as sent.
// A 405 collects every method that matches.
func (r *routeTable) dispatchTree(method, path string, needAllowed bool, ctx *Context) (bool, allowedMethodSet, error) {
	if r.tree == nil {
		return false, allowedMethodSet{}, nil
	}
	originalPath := path
	strictRouting := r.config != nil && r.config.StrictRouting
	caseSensitive := r.config != nil && r.config.CaseSensitive
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	mask := methodMaskFor(method)
	slot := singleBitIndex(mask)

	// Fast path: an exact-spelling static hit is one map probe, cheaper than
	// walking the tree's shared prefixes. Anything else walks the tree.
	var routes map[string]*routeEntry
	if slot >= 0 {
		routes = r.staticRoutes[slot]
	} else {
		routes = r.routes[method]
	}
	// The length masks rule out most probes that can't match: dynamic hits
	// skip hashing the path.
	if len(routes) != 0 && (slot < 0 || r.hasStaticRouteLength(slot, mask, len(originalPath)) ||
		(path != originalPath && r.hasStaticRouteLength(slot, mask, len(path)))) {
		if route := lookupStaticRouteExact(routes, originalPath, path); route != nil {
			ctx.setRouteIndex(route.infoIndex)
			return true, allowedMethodSet{}, route.handler(ctx)
		}
	}

	cacheEnabled := r.dispatchCacheEnabled()
	var key routeCacheKey
	if cacheEnabled {
		key = routeCacheKey{method: method, path: originalPath}
		if entry, ok := r.cache.getWithMask(key, mask); ok {
			if entry.route == nil {
				return false, entry.allowed, nil
			}
			if entry.route.paramCount > 0 {
				ctx.applyRouteParams(originalPath, entry.route, entry.values)
			}
			ctx.setRouteIndex(entry.route.infoIndex)
			return true, allowedMethodSet{}, entry.route.handler(ctx)
		}
	}

	captured := ctx.paramRangesScratch()
	matched, fold := treeMatchPath(path, caseSensitive)
	route := r.tree.lookupMethod(matched, 0, captured, 0, fold, slot, method)
	// As before the tree: the path as sent is tried only when the path
	// without its trailing slash matched no route and no other method of a
	// parameterized route; Allow adds the static routes of both spellings.
	var allowed allowedMethodSet
	if route == nil {
		var static, dynamic allowedMethodSet
		if needAllowed {
			r.tree.collectAllowed(matched, 0, fold, &static, &dynamic)
		}
		if path != originalPath {
			originalMatched, originalFold := treeMatchPath(originalPath, caseSensitive)
			if dynamic.empty() {
				route = r.tree.lookupMethod(originalMatched, 0, captured, 0, originalFold, slot, method)
				if route != nil {
					matched, fold = originalMatched, originalFold
				} else if needAllowed {
					r.tree.collectAllowed(originalMatched, 0, originalFold, &static, &dynamic)
				}
			} else {
				var ignored allowedMethodSet
				r.tree.collectAllowed(originalMatched, 0, originalFold, &static, &ignored)
			}
		}
		allowed = dynamic
		allowed.merge(static)
	}
	if route == nil {
		if cacheEnabled {
			r.cache.setMissWithMask(key, mask, routeCacheEntry{allowed: allowed})
		}
		return false, allowed, nil
	}
	if route.paramCount > 0 {
		if matched != originalPath {
			remapFoldedParams(captured, int(route.paramCount), originalPath, matched)
		}
		ctx.applyRouteParams(originalPath, route, *captured)
	}
	ctx.setRouteIndex(route.infoIndex)
	// Static hits aren't cached, as before: their walk is already cheap.
	if cacheEnabled && route.paramCount > 0 {
		r.cache.setMissWithMask(key, mask, routeCacheEntry{
			route:  route,
			values: cloneParamRangesForCache(*captured, int(route.paramCount)),
		})
	}
	return true, allowedMethodSet{}, route.handler(ctx)
}

// findTree is findInto through the route tree: it resolves the route for
// method and path without invoking it.
func (r *routeTable) findTree(method, path string, ctx *Context) HandlerFunc {
	if r.tree == nil {
		return nil
	}
	originalPath := path
	strictRouting := r.config != nil && r.config.StrictRouting
	caseSensitive := r.config != nil && r.config.CaseSensitive
	if !strictRouting && len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	slot := singleBitIndex(methodMaskFor(method))
	captured := ctx.paramRangesScratch()
	if captured == nil {
		captured = &paramRanges{}
	}
	matched, fold := treeMatchPath(path, caseSensitive)
	route := r.tree.lookupMethod(matched, 0, captured, 0, fold, slot, method)
	if route == nil && path != originalPath {
		matched, fold = treeMatchPath(originalPath, caseSensitive)
		route = r.tree.lookupMethod(matched, 0, captured, 0, fold, slot, method)
	}
	if route == nil {
		return nil
	}
	if route.paramCount > 0 {
		if matched != originalPath {
			remapFoldedParams(captured, int(route.paramCount), originalPath, matched)
		}
		ctx.applyRouteParams(originalPath, route, *captured)
	}
	ctx.setRoute(r.routeMetaAt(route.infoIndex))
	return route.handler
}
