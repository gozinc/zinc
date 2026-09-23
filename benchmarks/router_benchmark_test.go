// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	. "github.com/0mjs/zinc"
)

func runZincServeHTTPBenchmark(b *testing.B, handler http.Handler, req *http.Request) {
	rw := newDiscardResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		handler.ServeHTTP(rw, req)
	}

	benchmarkSinkInt = rw.status + rw.bytes
}

func runZincServeHTTPRequestSetBenchmark(b *testing.B, handler http.Handler, requests []*http.Request) {
	rw := newDiscardResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		handler.ServeHTTP(rw, requests[i%len(requests)])
	}

	benchmarkSinkInt = rw.status + rw.bytes
}

func buildZincDiagnosticRouterApp(cfg Config) *App {
	app := NewWithConfig(cfg)
	app.Get("/hello", func(c *Context) error {
		benchmarkSinkString = c.FullPath()
		return c.String(benchmarkOKResponse)
	})
	app.Get("/teams/{teamId}/users/{userId}", func(c *Context) error {
		benchmarkSinkString = c.Param("teamId") + "|" + c.Param("userId")
		return c.String(benchmarkOKResponse)
	})
	app.Get("/teams/{teamId}/users/{userId}/preferences", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})
	app.Get("/items/{id}", func(c *Context) error {
		benchmarkSinkString = c.Param("id")
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildZincRouteCacheBenchmarkApp() *App {
	return buildZincRouteCacheBenchmarkAppWithSize(64)
}

func buildZincRouteCacheBenchmarkAppWithSize(cacheSize int) *App {
	cfg := DefaultConfig
	cfg.RouteCacheSize = cacheSize
	app := NewWithConfig(cfg)
	for i := 0; i < 64; i++ {
		path := "/cache/" + strconv.Itoa(i) + "/items/{id}"
		app.Get(path, func(c *Context) error {
			benchmarkSinkString = c.Param("id")
			return c.String(benchmarkOKResponse)
		})
	}
	return app
}

func buildZincCaseInsensitiveBenchmarkApp() *App {
	cfg := DefaultConfig
	cfg.CaseSensitive = false
	app := NewWithConfig(cfg)
	app.Get("/Reports/Daily", func(c *Context) error {
		benchmarkSinkString = c.FullPath()
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildZincStrictRoutingBenchmarkApp() *App {
	cfg := DefaultConfig
	cfg.StrictRouting = true
	cfg.RouteCacheSize = 0
	app := NewWithConfig(cfg)
	app.Get("/teams/{teamId}", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildZincMountBenchmarkApp() *App {
	app := New()
	app.Mount("/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = io.WriteString(w, benchmarkOKResponse)
		case "/css/app/site.css":
			_, _ = io.WriteString(w, benchmarkOKResponse)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return app
}

func buildZincColdCacheTargets(count int) []string {
	targets := make([]string, count)
	for i := 0; i < count; i++ {
		targets[i] = "/cache/" + strconv.Itoa(i%64) + "/items/" + strconv.Itoa(1000+i)
	}
	return targets
}

func BenchmarkZincRouterStatic(b *testing.B) {
	handler := buildZincDiagnosticRouterApp(DefaultConfig)
	proveResponseAndSinkString(b, handler, MethodGet, "/hello", http.StatusOK, benchmarkOKResponse, "/hello")
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, "/hello", nil))
}

func BenchmarkZincRouterParam(b *testing.B) {
	handler := buildZincDiagnosticRouterApp(DefaultConfig)
	proveResponseAndSinkString(b, handler, MethodGet, "/teams/42/users/7", http.StatusOK, benchmarkOKResponse, "42|7")
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, "/teams/42/users/7", nil))
}

func BenchmarkZincRouterNotFound(b *testing.B) {
	handler := buildZincDiagnosticRouterApp(DefaultConfig)
	proveResponse(b, handler, MethodGet, "/missing/path", http.StatusNotFound, http.StatusText(http.StatusNotFound))
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, "/missing/path", nil))
}

func BenchmarkZincRouterMethodMismatch(b *testing.B) {
	handler := buildZincDiagnosticRouterApp(DefaultConfig)
	target := "/teams/42/users/7/preferences"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(MethodPost, target, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		b.Fatalf("status=%d want=%d", rec.Code, http.StatusMethodNotAllowed)
	}
	if rec.Body.String() != http.StatusText(http.StatusMethodNotAllowed) {
		b.Fatalf("body=%q", rec.Body.String())
	}
	if allow := rec.Header().Get(HeaderAllow); !strings.Contains(allow, MethodGet) {
		b.Fatalf("allow=%q", allow)
	}

	req := httptest.NewRequest(MethodPost, target, nil)
	rw := newDiscardResponseWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		handler.ServeHTTP(rw, req)
		benchmarkSinkString = rw.Header().Get(HeaderAllow)
	}
}

func BenchmarkZincRouterCacheHitParam(b *testing.B) {
	handler := buildZincRouteCacheBenchmarkApp()
	target := "/cache/63/items/999"
	proveResponseAndSinkString(b, handler, MethodGet, target, http.StatusOK, benchmarkOKResponse, "999")
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, target, nil))
}

func BenchmarkZincRouterCacheColdParam(b *testing.B) {
	handler := buildZincRouteCacheBenchmarkApp()
	targets := buildZincColdCacheTargets(256)
	requests := buildRequests(MethodGet, targets)

	proveResponseAndSinkString(b, handler, MethodGet, targets[0], http.StatusOK, benchmarkOKResponse, "1000")
	runZincServeHTTPRequestSetBenchmark(b, handler, requests)
}

func BenchmarkZincRouterCacheWorkloads(b *testing.B) {
	workloads := []struct {
		name  string
		count int
	}{
		{name: "Hot", count: 1},
		{name: "WorkingSet", count: 32},
		{name: "Unique", count: 4096},
	}
	cacheSizes := []struct {
		name string
		size int
	}{
		{name: "CacheEnabled", size: 64},
		{name: "CacheDisabled", size: 0},
	}

	for _, workload := range workloads {
		workload := workload
		b.Run(workload.name, func(b *testing.B) {
			targets := buildZincColdCacheTargets(workload.count)
			requests := buildRequests(MethodGet, targets)
			for _, cache := range cacheSizes {
				cache := cache
				b.Run(cache.name, func(b *testing.B) {
					handler := buildZincRouteCacheBenchmarkAppWithSize(cache.size)
					proveResponseAndSinkString(b, handler, MethodGet, targets[0], http.StatusOK, benchmarkOKResponse, "1000")
					runZincServeHTTPRequestSetBenchmark(b, handler, requests)
				})
			}
		})
	}
}

func BenchmarkZincRouterCaseInsensitiveStatic(b *testing.B) {
	handler := buildZincCaseInsensitiveBenchmarkApp()
	target := "/reports/daily"
	proveResponseAndSinkString(b, handler, MethodGet, target, http.StatusOK, benchmarkOKResponse, "/Reports/Daily")
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, target, nil))
}

func BenchmarkZincRouterStrictRoutingSlash(b *testing.B) {
	handler := buildZincStrictRoutingBenchmarkApp()
	target := "/teams/42/"
	proveResponse(b, handler, MethodGet, target, http.StatusNotFound, http.StatusText(http.StatusNotFound))
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, target, nil))
}

func BenchmarkZincRouterMethodNotAllowed(b *testing.B) {
	handler := buildZincDiagnosticRouterApp(DefaultConfig)
	target := "/items/42"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(MethodPost, target, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		b.Fatalf("status=%d want=%d", rec.Code, http.StatusMethodNotAllowed)
	}
	if rec.Body.String() != http.StatusText(http.StatusMethodNotAllowed) {
		b.Fatalf("body=%q", rec.Body.String())
	}
	if allow := rec.Header().Get(HeaderAllow); !strings.Contains(allow, MethodGet) {
		b.Fatalf("allow=%q", allow)
	}

	req := httptest.NewRequest(MethodPost, target, nil)
	rw := newDiscardResponseWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		handler.ServeHTTP(rw, req)
		benchmarkSinkString = rw.Header().Get(HeaderAllow)
	}
}

func BenchmarkZincMountStatic(b *testing.B) {
	handler := buildZincMountBenchmarkApp()
	target := "/assets/health"
	proveResponse(b, handler, MethodGet, target, http.StatusOK, benchmarkOKResponse)
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, target, nil))
}

func BenchmarkZincMountDeepPath(b *testing.B) {
	handler := buildZincMountBenchmarkApp()
	target := "/assets/css/app/site.css"
	proveResponse(b, handler, MethodGet, target, http.StatusOK, benchmarkOKResponse)
	runZincServeHTTPBenchmark(b, handler, httptest.NewRequest(MethodGet, target, nil))
}

func TestRunZincDiagnostics(t *testing.T) {
	t.Skip(`
From benchmarks/:
    go test -run=^$ -bench '^BenchmarkZinc(Bind|Router|Mount)' -benchmem

Notes:
- These are public-API Zinc diagnostics and stay inside the benchmarks module.
- The router slice now measures observable ServeHTTP behavior, route-cache paths, and mount dispatch instead of internal router methods.
- The bind slice measures request-level binding through handlers, including the cached-body path by binding twice on the same request.
- They are regression guards, not public framework-comparison numbers.
`)
}
