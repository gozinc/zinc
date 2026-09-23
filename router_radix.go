// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"strings"
)

type radixNodeKind uint8

const (
	radixRoot radixNodeKind = iota
	radixStatic
	radixParam
	radixCatchAll
)

// radixRoute is stored only at terminal nodes. Parameter names belong to the
// route rather than wildcard nodes because multiple routes share tree edges.
type radixRoute struct {
	handler          HandlerFunc
	extraParamNames  []string
	inlineParamNames [2]string
	paramIndices     map[string]uint8
	infoIndex        uint32
	paramCount       uint16
}

// radixNode stores compressed static prefixes and dedicated wildcard edges.
// Child precedence is static, parameter, then catch-all and must match lookup order.
type radixNode struct {
	kind          radixNodeKind
	prefix        string
	route         *radixRoute
	indices       []byte
	indexTable    *[256]uint16
	children      []*radixNode
	paramChild    *radixNode
	catchAllChild *radixNode
}

type paramRange struct {
	// Ranges index the original request path; they deliberately do not own strings.
	start uint32
	end   uint32
}

// paramRanges keeps the common two-parameter case inline. Extra storage is
// allocated only for wider routes and is reusable through the request Context.
type paramRanges struct {
	inline [inlineParamSlotCount]paramRange
	extra  []paramRange
}

func commonPrefixLen(a, b string) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func nextSlash(path string) int {
	return strings.IndexByte(path, '/')
}

func newRadixRoute(handler HandlerFunc, infoIndex uint32, names collectedRouteParams) *radixRoute {
	route := &radixRoute{
		handler:   handler,
		infoIndex: infoIndex,
	}
	count := names.count
	route.paramCount = uint16(count)
	inlineCount := count
	if inlineCount > len(route.inlineParamNames) {
		inlineCount = len(route.inlineParamNames)
	}
	for i := 0; i < inlineCount; i++ {
		route.inlineParamNames[i] = names.inline[i]
	}
	if len(names.extra) != 0 {
		route.extraParamNames = names.extra
	}
	if count >= indexedParamThreshold {
		// Linear search wins for ordinary routes; only unusually wide routes pay for a map.
		route.paramIndices = make(map[string]uint8, count)
		for i := 0; i < count; i++ {
			route.paramIndices[route.paramNameAt(i)] = uint8(i)
		}
	}
	return route
}

func (r *radixRoute) paramNameAt(index int) string {
	if index < len(r.inlineParamNames) {
		return r.inlineParamNames[index]
	}
	return r.extraParamNames[index-len(r.inlineParamNames)]
}

func (r *radixRoute) paramIndex(name string) (int, bool) {
	if r == nil {
		return 0, false
	}
	if len(r.paramIndices) != 0 {
		index, ok := r.paramIndices[name]
		return int(index), ok
	}
	count := int(r.paramCount)
	if count == 0 {
		return 0, false
	}
	if count > 0 && r.inlineParamNames[0] == name {
		return 0, true
	}
	if count > 1 && r.inlineParamNames[1] == name {
		return 1, true
	}
	for i := 2; i < count; i++ {
		if r.extraParamNames[i-len(r.inlineParamNames)] == name {
			return i, true
		}
	}
	return 0, false
}

func (p *paramRanges) set(index int, value paramRange) {
	if index < len(p.inline) {
		p.inline[index] = value
		return
	}
	extraIndex := index - len(p.inline)
	if extraIndex >= len(p.extra) {
		grown := make([]paramRange, extraIndex+1)
		copy(grown, p.extra)
		p.extra = grown
	}
	p.extra[extraIndex] = value
}

func (p paramRanges) at(index int) paramRange {
	if index < len(p.inline) {
		return p.inline[index]
	}
	return p.extra[index-len(p.inline)]
}

func (p *paramRanges) cloneFrom(other *paramRanges, count int) {
	p.inline = other.inline
	extraCount := count - len(p.inline)
	if extraCount <= 0 {
		p.extra = nil
		return
	}
	if cap(p.extra) < extraCount {
		p.extra = make([]paramRange, extraCount)
	} else {
		p.extra = p.extra[:extraCount]
	}
	copy(p.extra, other.extra[:extraCount])
}

func cloneParamRangesForCache(values paramRanges, count int) paramRanges {
	var cloned paramRanges
	cloned.cloneFrom(&values, count)
	return cloned
}

// addBrace converts a validated pattern into alternating static and wildcard
// edges. Parameter names are already stored on radixRoute, so wildcard nodes
// encode matching behaviour only and can be shared by differently named paths.
func (n *radixNode) addBrace(path string, route *radixRoute, caseInsensitive bool) error {
	current := n
	staticStart := 0
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			continue
		}
		if i > staticStart {
			literal := path[staticStart:i]
			if caseInsensitive {
				literal, _ = lowercasePath(literal)
			}
			current = current.addStaticPath(literal)
		}
		end := i + 1 + strings.IndexByte(path[i+1:], '}')
		rawName := path[i+1 : end]
		if strings.HasSuffix(rawName, "...") {
			current = current.addCatchAllChild()
		} else {
			current = current.addParamChild()
		}
		staticStart = end + 1
		i = end
	}
	if staticStart < len(path) {
		literal := path[staticStart:]
		if caseInsensitive {
			literal, _ = lowercasePath(literal)
		}
		current = current.addStaticPath(literal)
	}
	if !current.trySetRoute(route) {
		return fmt.Errorf("route already registered for %s", path)
	}
	return nil
}

func (n *radixNode) trySetRoute(route *radixRoute) bool {
	if route == nil || n.route != nil {
		return false
	}
	n.route = route
	return true
}

// addStaticPath inserts a literal into the compressed tree. When an existing
// prefix partially overlaps the new path, the common prefix becomes the parent:
// inserting "/teams" beside "/terms" turns "/te" into their shared node.
func (n *radixNode) addStaticPath(path string) *radixNode {
	current := n
	remaining := path
	for len(remaining) > 0 {
		idx := current.staticChildIndex(remaining[0])
		if idx < 0 {
			child := &radixNode{kind: radixStatic, prefix: remaining}
			current.addStaticChild(child)
			return child
		}
		child := current.children[idx]
		common := commonPrefixLen(child.prefix, remaining)
		if common == len(child.prefix) {
			current = child
			remaining = remaining[common:]
			continue
		}
		existing := &radixNode{
			kind:          radixStatic,
			prefix:        child.prefix[common:],
			route:         child.route,
			indices:       child.indices,
			indexTable:    child.indexTable,
			children:      child.children,
			paramChild:    child.paramChild,
			catchAllChild: child.catchAllChild,
		}
		child.prefix = child.prefix[:common]
		child.route = nil
		child.indices = nil
		child.indexTable = nil
		child.children = nil
		child.paramChild = nil
		child.catchAllChild = nil
		child.addStaticChild(existing)
		if common == len(remaining) {
			return child
		}
		inserted := &radixNode{kind: radixStatic, prefix: remaining[common:]}
		child.addStaticChild(inserted)
		return inserted
	}
	return current
}

func (n *radixNode) addParamChild() *radixNode {
	if n.paramChild != nil {
		return n.paramChild
	}
	child := &radixNode{kind: radixParam}
	n.paramChild = child
	return child
}

func (n *radixNode) addCatchAllChild() *radixNode {
	if n.catchAllChild != nil {
		return n.catchAllChild
	}
	child := &radixNode{kind: radixCatchAll}
	n.catchAllChild = child
	return child
}

func (n *radixNode) addStaticChild(child *radixNode) {
	// indices and children are parallel; the optional table stores index+1 so zero means absent.
	n.indices = append(n.indices, child.prefix[0])
	n.children = append(n.children, child)
	if n.indexTable != nil {
		n.indexTable[child.prefix[0]] = uint16(len(n.children))
		return
	}
	if len(n.children) < 8 {
		return
	}
	table := new([256]uint16)
	for i, index := range n.indices {
		table[index] = uint16(i + 1)
	}
	n.indexTable = table
}

func (n *radixNode) staticChildIndex(b byte) int {
	if n.indexTable != nil {
		if idx := n.indexTable[b]; idx > 0 {
			return int(idx) - 1
		}
		return -1
	}
	for i, index := range n.indices {
		if index == b {
			return i
		}
	}
	return -1
}

// lookup performs depth-first matching in specificity order: static, parameter,
// then catch-all. A failed static branch may therefore fall back to a wildcard
// sibling. offset always refers to the original path, while path is the
// unconsumed suffix used by the current node.
func (n *radixNode) lookup(path string, offset int, values *paramRanges, captured int) *radixRoute {
	switch n.kind {
	case radixStatic:
		if len(path) < len(n.prefix) || path[:len(n.prefix)] != n.prefix {
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
		values.set(captured, paramRange{
			start: uint32(offset),
			end:   uint32(offset + end),
		})
		captured++
		offset += end
		path = path[end:]
	case radixCatchAll:
		start := offset
		if len(path) > 0 && path[0] == '/' {
			start++
		}
		values.set(captured, paramRange{
			start: uint32(start),
			end:   uint32(offset + len(path)),
		})
		captured++
		return n.matchRoute(captured)
	}
	if len(path) == 0 {
		if matched := n.matchRoute(captured); matched != nil {
			return matched
		}
		if n.catchAllChild != nil {
			return n.catchAllChild.lookup(path, offset, values, captured)
		}
		return nil
	}
	if idx := n.staticChildIndex(path[0]); idx >= 0 {
		if matched := n.children[idx].lookup(path, offset, values, captured); matched != nil {
			return matched
		}
	}
	if n.paramChild != nil {
		if matched := n.paramChild.lookup(path, offset, values, captured); matched != nil {
			return matched
		}
	}
	if n.catchAllChild != nil {
		if matched := n.catchAllChild.lookup(path, offset, values, captured); matched != nil {
			return matched
		}
	}
	return nil
}

// matchesPath mirrors lookup without recording parameter ranges. Method
// negotiation uses it to discover whether another method owns the same shape.
func (n *radixNode) matchesPath(path string, captured int) bool {
	switch n.kind {
	case radixStatic:
		if len(path) < len(n.prefix) || path[:len(n.prefix)] != n.prefix {
			return false
		}
		path = path[len(n.prefix):]
	case radixParam:
		if len(path) == 0 || path[0] == '/' {
			return false
		}
		end := nextSlash(path)
		if end < 0 {
			end = len(path)
		}
		captured++
		path = path[end:]
	case radixCatchAll:
		captured++
		return n.hasPathRoute(captured)
	}

	if len(path) == 0 {
		if n.hasPathRoute(captured) {
			return true
		}
		return n.catchAllChild != nil && n.catchAllChild.matchesPath(path, captured)
	}
	if idx := n.staticChildIndex(path[0]); idx >= 0 && n.children[idx].matchesPath(path, captured) {
		return true
	}
	if n.paramChild != nil && n.paramChild.matchesPath(path, captured) {
		return true
	}
	if n.catchAllChild != nil && n.catchAllChild.matchesPath(path, captured) {
		return true
	}
	return false
}

func (n *radixNode) hasPathRoute(captured int) bool {
	return n.route != nil && captured == int(n.route.paramCount)
}

func (n *radixNode) matchRoute(captured int) *radixRoute {
	if n.route == nil || captured != int(n.route.paramCount) {
		return nil
	}
	return n.route
}

func (n *radixNode) hasRoutes() bool {
	return n.route != nil
}
