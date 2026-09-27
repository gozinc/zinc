// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
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
	indices       []byte
	indexTable    *[256]uint16
	children      []*radixNode
	paramChild    *radixNode
	catchAllChild *radixNode
	// methods holds a terminal's routes by method (router_tree.go).
	methods *nodeMethods
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

func newRadixRoute(a *treeArena, handler HandlerFunc, infoIndex uint32, names collectedRouteParams) *radixRoute {
	route := a.route()
	route.handler = handler
	route.infoIndex = infoIndex
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

// addStaticPath inserts a literal into the compressed tree. When an existing
// prefix partially overlaps the new path, the common prefix becomes the parent:
// inserting "/teams" beside "/terms" turns "/te" into their shared node.
func (n *radixNode) addStaticPath(a *treeArena, path string) *radixNode {
	current := n
	remaining := path
	for len(remaining) > 0 {
		idx := current.staticChildIndex(remaining[0])
		if idx < 0 {
			child := a.node()
			child.kind, child.prefix = radixStatic, remaining
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
		existing := a.node()
		*existing = radixNode{
			kind:          radixStatic,
			prefix:        child.prefix[common:],
			indices:       child.indices,
			indexTable:    child.indexTable,
			children:      child.children,
			paramChild:    child.paramChild,
			catchAllChild: child.catchAllChild,
			methods:       child.methods,
		}
		child.prefix = child.prefix[:common]
		child.indices = nil
		child.indexTable = nil
		child.children = nil
		child.paramChild = nil
		child.catchAllChild = nil
		child.methods = nil
		child.addStaticChild(existing)
		if common == len(remaining) {
			return child
		}
		inserted := a.node()
		inserted.kind, inserted.prefix = radixStatic, remaining[common:]
		child.addStaticChild(inserted)
		return inserted
	}
	return current
}

func (n *radixNode) addParamChild(a *treeArena) *radixNode {
	if n.paramChild != nil {
		return n.paramChild
	}
	child := a.node()
	child.kind = radixParam
	n.paramChild = child
	return child
}

func (n *radixNode) addCatchAllChild(a *treeArena) *radixNode {
	if n.catchAllChild != nil {
		return n.catchAllChild
	}
	child := a.node()
	child.kind = radixCatchAll
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
