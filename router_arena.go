// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

// treeArena hands out the tree's nodes, method tables and routes from
// chunks, so registering a route costs a few allocations per chunk instead
// of several per route. The tree lives as long as the app, so nothing in a
// chunk is ever freed early. A nil arena allocates each one on its own.
type treeArena struct {
	nodes   arenaChunk[radixNode]
	methods arenaChunk[nodeMethods]
	routes  arenaChunk[radixRoute]
}

// arenaChunkMax bounds a chunk, so a small app wastes little.
const arenaChunkMax = 64

type arenaChunk[T any] struct {
	free []T
	size int
}

func (c *arenaChunk[T]) alloc() *T {
	if len(c.free) == 0 {
		c.size = min(max(c.size*2, 4), arenaChunkMax)
		c.free = make([]T, c.size)
	}
	v := &c.free[0]
	c.free = c.free[1:]
	return v
}

func (a *treeArena) node() *radixNode {
	if a == nil {
		return new(radixNode)
	}
	return a.nodes.alloc()
}

func (a *treeArena) methodTable() *nodeMethods {
	if a == nil {
		return new(nodeMethods)
	}
	return a.methods.alloc()
}

func (a *treeArena) route() *radixRoute {
	if a == nil {
		return new(radixRoute)
	}
	return a.routes.alloc()
}
