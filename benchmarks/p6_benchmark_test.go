// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"net/http"
	"testing"
)

// Zinc-only benchmarks for 0.5 P6 (the route cache). Each workload runs on
// the same route set with the cache off (the default since P4) and on at the
// size the docs suggest, warmed first, so the cache's cost and benefit on
// the route tree are read side by side.

var p6Scenarios = []string{"GitHubAPI203", "ParseAPI26", "NestedAPI36", "Services1024"}

// p6Workloads returns request pools from most to least cache-friendly.
func p6Workloads(s benchmarkScenario) []struct {
	name string
	reqs []*http.Request
} {
	routes := s.routes
	param := s.paramRoutePattern()
	// hot: Zipf-weighted routes over 64 distinct URLs, repeated.
	hotPicks := zipfIndexes(len(routes), 64, 60)
	hot := poolRequestsFor(64, 61, func(i int) (string, string) {
		route := routes[hotPicks[i]]
		return route.method, poolPath(route.pattern, i)
	})
	hotReqs := make([]*http.Request, poolSize)
	for i, k := range zipfIndexes(64, poolSize, 62) {
		hotReqs[i] = hot[k]
	}
	picks := zipfIndexes(len(routes), poolSize, 44)
	return []struct {
		name string
		reqs []*http.Request
	}{
		{"OneURL", []*http.Request{s.paramRequest}},
		{"Hot64", hotReqs},
		{"Traffic", poolRequestsFor(poolSize, 45, func(i int) (string, string) {
			route := routes[picks[i]]
			return route.method, poolPath(route.pattern, i)
		})},
		{"ParamPool", poolRequests(param.method, poolSize, 40, func(i int) string { return poolPath(param.pattern, i) })},
		{"NotFoundPool", poolRequests(http.MethodGet, poolSize, 41, poolMissingPaths(scenarioMissingPath(s.name), poolSize))},
	}
}

var p6CacheSizes = []struct {
	name string
	size int
}{{"Off", 0}, {"On", cacheMatrixSize}}

func BenchmarkP6Cache(b *testing.B) {
	for _, name := range p6Scenarios {
		s := scenarioNamed(name)
		for _, w := range p6Workloads(s) {
			for _, c := range p6CacheSizes {
				b.Run(name+"/"+w.name+"/"+c.name, func(b *testing.B) {
					h := buildZincCacheMatrixHandler(s.routes, c.size)
					warmZincCacheMatrix(h, w.reqs, cacheMatrixPromotionWarmCycles)
					runP5(b, h, w.reqs)
				})
			}
		}
	}
}

func BenchmarkP6CacheParallel(b *testing.B) {
	for _, name := range []string{"GitHubAPI203", "Services1024"} {
		s := scenarioNamed(name)
		for _, w := range p6Workloads(s)[1:3] {
			for _, c := range p6CacheSizes {
				b.Run(name+"/"+w.name+"/"+c.name, func(b *testing.B) {
					h := buildZincCacheMatrixHandler(s.routes, c.size)
					warmZincCacheMatrix(h, w.reqs, cacheMatrixPromotionWarmCycles)
					reqs := w.reqs
					b.ReportAllocs()
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						rw := newDiscardResponseWriter()
						i := 0
						for pb.Next() {
							rw.reset()
							h.ServeHTTP(rw, reqs[i%len(reqs)])
							i++
						}
					})
				})
			}
		}
	}
}
