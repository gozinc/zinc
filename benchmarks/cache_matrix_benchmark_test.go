// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	. "github.com/0mjs/zinc"
)

const cacheMatrixCardinality = 16
const cacheMatrixPromotionWarmCycles = 4

func cacheMatrixGitHubScenario() benchmarkScenario {
	for _, scenario := range scenarioBenchmarks {
		if scenario.name == "GitHubAPI203" {
			return scenario
		}
	}
	panic("GitHubAPI203 benchmark scenario is missing")
}

func buildZincCacheMatrixHandler(routes []scenarioRoute, cacheSize int) http.Handler {
	cfg := DefaultConfig
	cfg.RouteCacheSize = cacheSize
	app := NewWithConfig(cfg)
	for _, route := range routes {
		app.Add(route.method, scenarioZincPattern(route.pattern), func(*Context) error {
			return nil
		})
	}
	return app
}

func materializeScenarioPathVariant(pattern string, variant int) string {
	if !strings.ContainsAny(pattern, ":*") {
		return pattern
	}

	suffix := "-v" + strconv.Itoa(variant)
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		switch {
		case strings.HasPrefix(segment, ":"):
			segments[i] = scenarioSampleValue(segment[1:]) + suffix
		case segment == "*":
			segments[i] = scenarioWildcardSampleValue(pattern) + suffix
		}
	}
	return strings.Join(segments, "/")
}

func buildCacheMatrixHighCardinalityRequests(scenario benchmarkScenario) []*http.Request {
	requests := make([]*http.Request, 0, len(scenario.routes)*cacheMatrixCardinality)
	for variant := 0; variant < cacheMatrixCardinality; variant++ {
		for _, route := range scenario.routes {
			target := route.requestPath
			if len(route.paramNames) != 0 {
				target = materializeScenarioPathVariant(route.pattern, variant)
			}
			requests = append(requests, httptest.NewRequest(route.method, target, nil))
		}
	}
	return requests
}

func buildCacheMatrixDynamicVariantRequests(scenario benchmarkScenario, variant int) []*http.Request {
	requests := make([]*http.Request, 0, len(scenario.routes))
	for _, route := range scenario.routes {
		if len(route.paramNames) == 0 {
			continue
		}
		target := materializeScenarioPathVariant(route.pattern, variant)
		requests = append(requests, httptest.NewRequest(route.method, target, nil))
	}
	return requests
}

func proveZincCacheMatrixRequests(t testing.TB, handler http.Handler, requests []*http.Request) {
	t.Helper()
	for i, req := range requests {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: %s %s returned status=%d want=%d", i, req.Method, req.URL.Path, rec.Code, http.StatusOK)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("request %d: %s %s returned body=%q want empty", i, req.Method, req.URL.Path, rec.Body.String())
		}
	}
}

func runZincCacheMatrixSequential(b *testing.B, cacheSize int, scenario benchmarkScenario, requests []*http.Request) {
	handler := buildZincCacheMatrixHandler(scenario.routes, cacheSize)
	proveZincCacheMatrixRequests(b, handler, requests)
	rw := newDiscardResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		handler.ServeHTTP(rw, requests[i%len(requests)])
	}
	benchmarkSinkInt = rw.status + rw.bytes
}

func warmZincCacheMatrix(handler http.Handler, requests []*http.Request, cycles int) {
	rw := newDiscardResponseWriter()
	for cycle := 0; cycle < cycles; cycle++ {
		for _, request := range requests {
			rw.reset()
			handler.ServeHTTP(rw, request)
		}
	}
}

func benchmarkZincCacheMatrixRequests(b *testing.B, handler http.Handler, requests []*http.Request) {
	rw := newDiscardResponseWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		handler.ServeHTTP(rw, requests[i%len(requests)])
	}
	benchmarkSinkInt = rw.status + rw.bytes
}

func runZincCacheMatrixParallel(b *testing.B, cacheSize int, scenario benchmarkScenario, requests []*http.Request) {
	handler := buildZincCacheMatrixHandler(scenario.routes, cacheSize)
	proveZincCacheMatrixRequests(b, handler, requests)
	workerRequests := make([][]*http.Request, runtime.GOMAXPROCS(0))
	for worker := range workerRequests {
		workerRequests[worker] = make([]*http.Request, len(requests))
		for i, request := range requests {
			workerRequests[worker][i] = request.Clone(request.Context())
		}
	}
	var workerID atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		worker := int(workerID.Add(1) - 1)
		requests := workerRequests[worker]
		index := worker % len(requests)
		rw := newDiscardResponseWriter()
		for pb.Next() {
			rw.reset()
			handler.ServeHTTP(rw, requests[index])
			index++
			if index == len(requests) {
				index = 0
			}
		}
	})
}

func BenchmarkZincGitHubCachePhaseShift(b *testing.B) {
	scenario := cacheMatrixGitHubScenario()
	phaseA := buildCacheMatrixDynamicVariantRequests(scenario, 0)
	phaseB := buildCacheMatrixDynamicVariantRequests(scenario, 1)

	b.Run("StableA", func(b *testing.B) {
		handler := buildZincCacheMatrixHandler(scenario.routes, DefaultConfig.RouteCacheSize)
		warmZincCacheMatrix(handler, phaseA, cacheMatrixPromotionWarmCycles)
		benchmarkZincCacheMatrixRequests(b, handler, phaseA)
	})

	b.Run("ShiftedRecoveredB", func(b *testing.B) {
		handler := buildZincCacheMatrixHandler(scenario.routes, DefaultConfig.RouteCacheSize)
		warmZincCacheMatrix(handler, phaseA, cacheMatrixPromotionWarmCycles)
		warmZincCacheMatrix(handler, phaseB, cacheMatrixPromotionWarmCycles)
		benchmarkZincCacheMatrixRequests(b, handler, phaseB)
	})

	b.Run("FreshB", func(b *testing.B) {
		handler := buildZincCacheMatrixHandler(scenario.routes, DefaultConfig.RouteCacheSize)
		warmZincCacheMatrix(handler, phaseB, cacheMatrixPromotionWarmCycles)
		benchmarkZincCacheMatrixRequests(b, handler, phaseB)
	})
}

func BenchmarkZincGitHubCacheMatrix(b *testing.B) {
	scenario := cacheMatrixGitHubScenario()
	highCardinality := buildCacheMatrixHighCardinalityRequests(scenario)

	b.Run("DefaultCache", func(b *testing.B) {
		runZincCacheMatrixSequential(b, DefaultConfig.RouteCacheSize, scenario, scenario.allRequests)
	})
	b.Run("CacheDisabled", func(b *testing.B) {
		runZincCacheMatrixSequential(b, 0, scenario, scenario.allRequests)
	})
	b.Run("HighCardinality", func(b *testing.B) {
		runZincCacheMatrixSequential(b, DefaultConfig.RouteCacheSize, scenario, highCardinality)
	})
	b.Run("ParallelHighCardinality", func(b *testing.B) {
		runZincCacheMatrixParallel(b, DefaultConfig.RouteCacheSize, scenario, highCardinality)
	})
}

func BenchmarkZincGitHubStaticCacheMatrix(b *testing.B) {
	scenario := cacheMatrixGitHubScenario()
	requests := []*http.Request{scenario.staticRequest}

	b.Run("DefaultCache", func(b *testing.B) {
		runZincCacheMatrixSequential(b, DefaultConfig.RouteCacheSize, scenario, requests)
	})
	b.Run("CacheDisabled", func(b *testing.B) {
		runZincCacheMatrixSequential(b, 0, scenario, requests)
	})
}

func TestZincGitHubCacheMatrixCorpus(t *testing.T) {
	scenario := cacheMatrixGitHubScenario()
	if got, want := len(scenario.allRequests), 203; got != want {
		t.Fatalf("fixed request count=%d want=%d", got, want)
	}

	highCardinality := buildCacheMatrixHighCardinalityRequests(scenario)
	if got, want := len(highCardinality), 203*cacheMatrixCardinality; got != want {
		t.Fatalf("high-cardinality request count=%d want=%d", got, want)
	}

	uniqueDynamic := make(map[string]struct{})
	for _, req := range highCardinality {
		key := req.Method + " " + req.URL.Path
		uniqueDynamic[key] = struct{}{}
	}
	if got := len(uniqueDynamic); got <= DefaultConfig.RouteCacheSize {
		t.Fatalf("unique request keys=%d must exceed default cache size=%d", got, DefaultConfig.RouteCacheSize)
	}

	proveZincCacheMatrixRequests(t, buildZincCacheMatrixHandler(scenario.routes, DefaultConfig.RouteCacheSize), highCardinality)
	proveZincCacheMatrixRequests(t, buildZincCacheMatrixHandler(scenario.routes, 0), highCardinality)
}
