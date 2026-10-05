// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
)

// The route tree holds every route of a routeTable, static and parameterized,
// for every method, in one compressed radix tree. A terminal node carries a
// method table, so one walk finds the route for a method, and a miss can
// collect every method the path allows. At every level a literal edge is
// tried before a parameter, and a parameter before a catch-all, with
// backtracking. benchmarks/ROUTER_SPEC.md states the matching rules.

// nodeFor walks the static, parameter and catch-all edges of a validated
// pattern, creating them as needed, and returns the terminal node. Static
// labels are lowercased when routing is case-insensitive.
func (n *radixNode) nodeFor(a *treeArena, path string, caseInsensitive bool) *radixNode {
	current := n
	staticStart := 0
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			continue
		}
		if i > staticStart {
			current = current.addStaticPath(a, foldLiteral(path[staticStart:i], caseInsensitive))
		}
		end := i + 1 + indexByte(path[i+1:], '}')
		if rawName := path[i+1 : end]; len(rawName) > 3 && rawName[len(rawName)-3:] == "..." {
			current = current.addCatchAllChild(a)
		} else {
			current = current.addParamChild(a)
		}
		staticStart = end + 1
		i = end
	}
	if staticStart < len(path) {
		current = current.addStaticPath(a, foldLiteral(path[staticStart:], caseInsensitive))
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

// treeWalk is the part of a route-tree walk that doesn't change from node to
// node, passed by pointer so each recursive call carries only the node, the
// rest of the path, the offset and the count of captured parameters.
type treeWalk struct {
	values *paramRanges
	method string
	slot   int
	fold   bool
	// allowed, when set, receives the methods of every terminal that matches
	// the path without a route for method. A miss has visited every such
	// terminal, so it then holds Allow without a second walk.
	allowed *allowedMethodSet
}

// walk finds method's route for path. A terminal without a route for the
// method is not a match, so the walk backtracks past it to the next
// candidate.
func (n *radixNode) walk(path string, offset, captured int, w *treeWalk) *radixRoute {
	switch n.kind {
	case radixStatic:
		if w.fold {
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
		w.values.set(captured, paramRange{start: uint32(offset), end: uint32(offset + end)})
		captured++
		offset += end
		path = path[end:]
	case radixCatchAll:
		// The rest of the path, as sent: a doubled slash keeps its second
		// slash, so a value that starts with "/" routes back to itself.
		w.values.set(captured, paramRange{start: uint32(offset), end: uint32(offset + len(path))})
		return n.terminal(captured+1, w)
	}
	if len(path) == 0 {
		if matched := n.terminal(captured, w); matched != nil {
			return matched
		}
		if n.catchAllChild != nil {
			return n.catchAllChild.walk(path, offset, captured, w)
		}
		return nil
	}
	first := path[0]
	if w.fold {
		first = foldByte(first)
	}
	if idx := n.staticChildIndex(first); idx >= 0 {
		if matched := n.children[idx].walk(path, offset, captured, w); matched != nil {
			return matched
		}
	}
	if n.paramChild != nil {
		if matched := n.paramChild.walk(path, offset, captured, w); matched != nil {
			return matched
		}
	}
	if n.catchAllChild != nil {
		return n.catchAllChild.walk(path, offset, captured, w)
	}
	return nil
}

// terminal returns the node's route for the walk's method, recording the
// node's other methods when the walk collects Allow.
func (n *radixNode) terminal(captured int, w *treeWalk) *radixRoute {
	if n.methods == nil {
		return nil
	}
	if route := n.methods.get(w.slot, w.method); route != nil && int(route.paramCount) == captured {
		return route
	}
	if w.allowed != nil {
		n.methods.addAllowed(w.allowed, captured)
	}
	return nil
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

// trimTrailingSlash is the router's one trailing-slash rule. Unless routing
// is strict, a path and the same path without its trailing slash are one
// path: registration records a route under both spellings, and a request is
// matched without its trailing slash first. The root path "/" is kept.
func (r *routeTable) trimTrailingSlash(path string) string {
	if (r.config == nil || !r.config.StrictRouting) && len(path) > 1 && path[len(path)-1] == '/' {
		return path[:len(path)-1]
	}
	return path
}

// resolve finds method's route for path. Dispatch, Find and FindRoute all
// use it, so they agree on every request. On a hit, values holds the route's
// parameters as byte ranges of path. On a miss, allowed, when not nil,
// receives every method that resolve would find for path, for Allow.
//
// The order:
//
//  1. An exact static spelling, one map probe.
//  2. The tree, for the path without its trailing slash (trimTrailingSlash).
//  3. The tree, for the path as sent, if that differs. Every route is also
//     recorded without its trailing slash, so this step is for catch-alls,
//     as with /files/{path...} and "/files/".
//
// A catch-all keeps the request's trailing slash: matched in 2, its value is
// extended to the end of the path as sent.
func (r *routeTable) resolve(method, path string, values *paramRanges, allowed *allowedMethodSet) *radixRoute {
	if r.tree == nil {
		return nil
	}
	caseSensitive := r.config != nil && r.config.CaseSensitive
	trimmed := r.trimTrailingSlash(path)
	mask := methodMaskFor(method)
	slot := singleBitIndex(mask)

	var routes map[string]*radixRoute
	if slot >= 0 {
		routes = r.staticRoutes[slot]
	} else {
		routes = r.routes[method]
	}
	// The length masks rule out most probes that can't match, so dynamic
	// hits skip hashing the path.
	if len(routes) != 0 && (slot < 0 || r.hasStaticRouteLength(slot, mask, len(path)) ||
		(trimmed != path && r.hasStaticRouteLength(slot, mask, len(trimmed)))) {
		if route := lookupStaticRouteExact(routes, path, trimmed); route != nil {
			return route
		}
	}

	w := treeWalk{values: values, method: method, slot: slot, allowed: allowed}
	matched, fold := treeMatchPath(trimmed, caseSensitive)
	w.fold = fold
	route := r.tree.walk(matched, 0, 0, &w)
	extend := route != nil && trimmed != path
	if route == nil && trimmed != path {
		matched, w.fold = treeMatchPath(path, caseSensitive)
		route = r.tree.walk(matched, 0, 0, &w)
	}
	if route == nil || route.paramCount == 0 {
		return route
	}
	if matched != path {
		remapFoldedParams(values, int(route.paramCount), path, matched)
	}
	if extend && route.catchAll {
		last := int(route.paramCount) - 1
		value := values.at(last)
		value.end = uint32(len(path))
		values.set(last, value)
	}
	return route
}

// dispatchInto resolves and invokes a route. needAllowed asks for the
// methods that match on a miss, for 405 and automatic OPTIONS; ordinary
// not-found dispatch leaves it off.
func (r *routeTable) dispatchInto(method, path string, needAllowed bool, ctx *Context) (bool, allowedMethodSet, error) {
	var allowed allowedMethodSet
	var into *allowedMethodSet
	if needAllowed {
		into = &allowed
	}
	values := ctx.paramRangesScratch()
	route := r.resolve(method, path, values, into)
	if route == nil {
		return false, allowed, nil
	}
	if route.paramCount > 0 {
		ctx.applyRouteParams(path, route, *values)
	}
	ctx.setRouteIndex(route.infoIndex)
	return true, allowedMethodSet{}, route.handler(ctx)
}

// findInto resolves the route for method and path without invoking it,
// leaving its metadata and parameters in ctx.
func (r *routeTable) findInto(method, path string, ctx *Context) HandlerFunc {
	values := ctx.paramRangesScratch()
	route := r.resolve(method, path, values, nil)
	if route == nil {
		return nil
	}
	if route.paramCount > 0 {
		ctx.applyRouteParams(path, route, *values)
	}
	ctx.setRoute(r.routeMetaAt(route.infoIndex))
	return route.handler
}

// resolvesTo reports whether path resolves to the route at index with
// exactly these parameter values, or else describes what it resolves to.
func (r *routeTable) resolvesTo(method, path string, index uint32, want []string) (bool, string) {
	var values paramRanges
	route := r.resolve(method, path, &values, nil)
	if route == nil {
		return false, "no route"
	}
	if route.infoIndex != index {
		meta := r.routeMetaAt(route.infoIndex)
		return false, meta.method + " " + meta.path
	}
	for i := 0; i < int(route.paramCount); i++ {
		v := values.at(i)
		if got := path[v.start:v.end]; got != want[i] {
			return false, fmt.Sprintf("%s=%q", route.paramNameAt(i), got)
		}
	}
	return true, ""
}
