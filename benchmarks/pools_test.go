// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	. "github.com/0mjs/zinc"
)

// Request pools.
//
// A scenario that sends one fixed request measures the best case of any
// per-path cache: after the first request every lookup is a hit. Zinc caches
// up to DefaultRouteCacheSize concrete paths, including 404s and 405s, so a
// pool must hold well over that many distinct paths before it measures the
// router rather than the cache. Gin, Echo, Chi and BunRouter have no route
// cache; a large pool costs them only the CPU-cache misses of touching more
// request objects, which every framework pays alike.
//
// Pools are built before timing starts, are deterministic, and every
// framework in a scenario receives the same requests in the same order.

// poolSize is the number of distinct request paths in a pool: ten times
// Zinc's default route cache.
const poolSize = 10 * DefaultRouteCacheSize

// poolSeed fixes every pool's contents and order across runs.
const poolSeed = 0x5a1c

func poolRand(stream uint64) *rand.Rand {
	return rand.New(rand.NewPCG(poolSeed, stream))
}

// poolValue is the i-th distinct value for a parameter named name. Values
// keep the shape of the scenario's sample value (numeric IDs stay numeric) so
// handlers do the same work they did with one fixed value.
func poolValue(name string, i int) string {
	base := scenarioSampleValue(name)
	if _, err := strconv.Atoi(base); err == nil {
		return strconv.Itoa(100000 + i)
	}
	return base + "-" + strconv.FormatInt(int64(i), 36)
}

// poolTail is the i-th distinct wildcard tail: one to five segments deep.
func poolTail(i int) string {
	depth := 1 + i%5
	segments := make([]string, depth)
	for d := range segments {
		segments[d] = "p" + strconv.FormatInt(int64(i*7+d), 36)
	}
	return strings.Join(segments, "/") + ".txt"
}

// poolPath fills pattern's :params and * tail with the i-th pool values.
func poolPath(pattern string, i int) string {
	if !strings.ContainsAny(pattern, ":*") {
		return pattern
	}
	segments := strings.Split(pattern, "/")
	for s, segment := range segments {
		switch {
		case strings.HasPrefix(segment, ":"):
			segments[s] = poolValue(segment[1:], i)
		case segment == "*":
			segments[s] = poolTail(i)
		}
	}
	return strings.Join(segments, "/")
}

// poolRequests builds n requests for method, with target(i) as the i-th path,
// in a shuffled order so consecutive requests don't share a prefix pattern.
func poolRequests(method string, n int, stream uint64, target func(i int) string) []*http.Request {
	reqs := make([]*http.Request, n)
	for i := range reqs {
		reqs[i] = httptest.NewRequest(method, target(i), nil)
	}
	poolRand(stream).Shuffle(n, func(i, j int) { reqs[i], reqs[j] = reqs[j], reqs[i] })
	return reqs
}

// poolRequestsFor is poolRequests with a method chosen per request.
func poolRequestsFor(n int, stream uint64, target func(i int) (method, path string)) []*http.Request {
	reqs := make([]*http.Request, n)
	for i := range reqs {
		method, path := target(i)
		reqs[i] = httptest.NewRequest(method, path, nil)
	}
	poolRand(stream).Shuffle(n, func(i, j int) { reqs[i], reqs[j] = reqs[j], reqs[i] })
	return reqs
}

// poolMissingPaths is a pool of paths that match no route: each is distinct,
// so no framework can answer a repeated miss from a cache.
func poolMissingPaths(prefix string, n int) func(i int) string {
	return func(i int) string {
		return prefix + "/missing-" + strconv.FormatInt(int64(i), 36) + "/" + strconv.Itoa(i%97)
	}
}

// zipfIndexes returns n route indexes drawn from a Zipf distribution over
// routes (exponent 1.1), so a few routes take most of the traffic, as in a
// real API. Route 0 is not always the hottest: ranks are shuffled first.
func zipfIndexes(routes, n int, stream uint64) []int {
	r := poolRand(stream)
	rank := r.Perm(routes)
	weights := make([]float64, routes)
	total := 0.0
	for k := range weights {
		weights[k] = 1 / math.Pow(float64(k+1), 1.1)
		total += weights[k]
	}
	out := make([]int, n)
	for i := range out {
		x := r.Float64() * total
		k := 0
		for ; k < routes-1 && x > weights[k]; k++ {
			x -= weights[k]
		}
		out[i] = rank[k]
	}
	return out
}

// poolID is the i-th distinct numeric ID.
func poolID(i int) string { return strconv.Itoa(100000 + i) }

// poolTeamUser is the i-th /teams/{teamID}/users/{userID} path: the team
// stays 42 and the user varies, so handlers that echo both do the same work.
func poolTeamUser(i int) string { return "/teams/42/users/" + poolID(i) }

// poolPrepared builds n prepared body requests. They share one copy of body,
// which reset never modifies, so a pool of large bodies costs one body.
func poolPrepared(method string, n int, stream uint64, target func(i int) string, body []byte, headers http.Header) []preparedBenchmarkRequest {
	shared := append([]byte(nil), body...)
	reqs := make([]preparedBenchmarkRequest, n)
	for i := range reqs {
		req := httptest.NewRequest(method, target(i), nil)
		for key, values := range headers {
			req.Header[key] = append([]string(nil), values...)
		}
		reqs[i] = preparedBenchmarkRequest{request: req, body: shared}
	}
	poolRand(stream).Shuffle(n, func(i, j int) { reqs[i], reqs[j] = reqs[j], reqs[i] })
	return reqs
}
