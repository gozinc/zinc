// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"

	. "github.com/0mjs/zinc"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v5"
)

// runServeHTTPParallelBenchmarksWithProof serves requests from every
// goroutine. Each goroutine gets its own copy of the pool, built before the
// timer starts, so no request is shared between goroutines, and starts at a
// different offset.
func runServeHTTPParallelBenchmarksWithProof(
	b *testing.B,
	requests []*http.Request,
	cases []benchmarkCase,
	prove func(*testing.B, string, http.Handler),
) {
	proveCases(b, cases, len(requests), func(i int) *http.Request { return requests[i] })
	workers := runtime.GOMAXPROCS(0)
	copies := make([][]*http.Request, workers)
	for w := range copies {
		copies[w] = make([]*http.Request, len(requests))
		for i, req := range requests {
			copies[w][i] = req.Clone(req.Context())
		}
	}
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
			if prove != nil {
				prove(b, bc.name, handler)
			}

			b.ReportAllocs()
			b.ResetTimer()
			var total, next atomic.Int64
			b.RunParallel(func(pb *testing.PB) {
				w := int(next.Add(1)-1) % workers
				reqs := copies[w]
				rw := newDiscardResponseWriter()
				for i := w * len(reqs) / workers; pb.Next(); i++ {
					rw.reset()
					handler.ServeHTTP(rw, reqs[i%len(reqs)])
				}
				total.Add(int64(rw.status + rw.bytes))
			})
			benchmarkSinkInt = int(total.Load())
		})
	}
}

func buildZincParallelParamHandler() http.Handler {
	app := New()
	app.Get("/hello/{name}", func(c *Context) error {
		if c.Param("name") == "" {
			return c.Status(http.StatusInternalServerError).String("BAD")
		}
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiParallelParamHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/hello/{name}", func(w http.ResponseWriter, req *http.Request) {
		if chi.URLParam(req, "name") == "" {
			w.WriteHeader(http.StatusInternalServerError)
			writeText(w, "BAD")
			return
		}
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoParallelParamHandler() http.Handler {
	e := echo.New()
	e.GET("/hello/:name", func(c *echo.Context) error {
		if c.Param("name") == "" {
			return c.String(http.StatusInternalServerError, "BAD")
		}
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinParallelParamHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/hello/:name", func(c *gin.Context) {
		if c.Param("name") == "" {
			c.String(http.StatusInternalServerError, "BAD")
			return
		}
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func buildZincParallelMiddlewareHandler() http.Handler {
	app := New()
	app.Use(
		zincMiddleware("mw1"),
		zincMiddleware("mw2"),
		zincMiddleware("mw3"),
		zincMiddleware("mw4"),
		zincMiddleware("mw5"),
	)
	app.Get("/middleware", func(c *Context) error {
		if !zincMiddlewareSatisfied(c) {
			return c.Status(http.StatusInternalServerError).String("BAD")
		}
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiParallelMiddlewareHandler() http.Handler {
	r := chi.NewRouter()
	r.Use(chiMiddleware(middlewareKey1))
	r.Use(chiMiddleware(middlewareKey2))
	r.Use(chiMiddleware(middlewareKey3))
	r.Use(chiMiddleware(middlewareKey4))
	r.Use(chiMiddleware(middlewareKey5))
	r.Get("/middleware", func(w http.ResponseWriter, req *http.Request) {
		if !requestContextMiddlewareSatisfied(req) {
			w.WriteHeader(http.StatusInternalServerError)
			writeText(w, "BAD")
			return
		}
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoParallelMiddlewareHandler() http.Handler {
	e := echo.New()
	e.Use(
		echoMiddleware("mw1"),
		echoMiddleware("mw2"),
		echoMiddleware("mw3"),
		echoMiddleware("mw4"),
		echoMiddleware("mw5"),
	)
	e.GET("/middleware", func(c *echo.Context) error {
		if !echoMiddlewareSatisfied(c) {
			return c.String(http.StatusInternalServerError, "BAD")
		}
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinParallelMiddlewareHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.Use(
		ginMiddleware("mw1"),
		ginMiddleware("mw2"),
		ginMiddleware("mw3"),
		ginMiddleware("mw4"),
		ginMiddleware("mw5"),
	)
	r.GET("/middleware", func(c *gin.Context) {
		if !ginMiddlewareSatisfied(c) {
			c.String(http.StatusInternalServerError, "BAD")
			return
		}
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func buildZincParallelAPIHappyPathHandler() http.Handler {
	app := New()
	app.Use(
		zincMiddleware("mw1"),
		zincMiddleware("mw2"),
		zincMiddleware("mw3"),
		zincMiddleware("mw4"),
		zincMiddleware("mw5"),
	)
	app.Get("/teams/{teamID}/users/{userID}", func(c *Context) error {
		var input benchmarkAPIBindInput
		if err := c.Bind().All(&input); err != nil {
			return c.Status(http.StatusBadRequest).String("BAD")
		}
		if !zincMiddlewareSatisfied(c) || input.TeamID != 42 || input.UserID != 7 || !input.Verbose || input.Limit != 25 {
			return c.Status(http.StatusInternalServerError).String("BAD")
		}
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiParallelAPIHappyPathHandler() http.Handler {
	r := chi.NewRouter()
	r.Use(chiMiddleware(middlewareKey1))
	r.Use(chiMiddleware(middlewareKey2))
	r.Use(chiMiddleware(middlewareKey3))
	r.Use(chiMiddleware(middlewareKey4))
	r.Use(chiMiddleware(middlewareKey5))
	r.Get("/teams/{teamID}/users/{userID}", func(w http.ResponseWriter, req *http.Request) {
		input := benchmarkAPIBindInput{
			TeamID: parseBenchmarkInt(chi.URLParam(req, "teamID")),
			UserID: parseBenchmarkInt(chi.URLParam(req, "userID")),
		}
		fillBenchmarkAPIQuery(&input, req)
		if !requestContextMiddlewareSatisfied(req) || input.TeamID != 42 || input.UserID != 7 || !input.Verbose || input.Limit != 25 {
			w.WriteHeader(http.StatusInternalServerError)
			writeText(w, "BAD")
			return
		}
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoParallelAPIHappyPathHandler() http.Handler {
	e := echo.New()
	e.Use(
		echoMiddleware("mw1"),
		echoMiddleware("mw2"),
		echoMiddleware("mw3"),
		echoMiddleware("mw4"),
		echoMiddleware("mw5"),
	)
	e.GET("/teams/:teamID/users/:userID", func(c *echo.Context) error {
		var input benchmarkAPIBindInput
		if err := c.Bind(&input); err != nil {
			return c.String(http.StatusBadRequest, "BAD")
		}
		if !echoMiddlewareSatisfied(c) || input.TeamID != 42 || input.UserID != 7 || !input.Verbose || input.Limit != 25 {
			return c.String(http.StatusInternalServerError, "BAD")
		}
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinParallelAPIHappyPathHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.Use(
		ginMiddleware("mw1"),
		ginMiddleware("mw2"),
		ginMiddleware("mw3"),
		ginMiddleware("mw4"),
		ginMiddleware("mw5"),
	)
	r.GET("/teams/:teamID/users/:userID", func(c *gin.Context) {
		var input benchmarkAPIBindInput
		if err := c.ShouldBindUri(&input); err != nil {
			c.String(http.StatusBadRequest, "BAD")
			return
		}
		if err := c.ShouldBindQuery(&input); err != nil {
			c.String(http.StatusBadRequest, "BAD")
			return
		}
		if !ginMiddlewareSatisfied(c) || input.TeamID != 42 || input.UserID != 7 || !input.Verbose || input.Limit != 25 {
			c.String(http.StatusInternalServerError, "BAD")
			return
		}
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func parallelStaticCases() []benchmarkCase {
	return withBunRouter(focusedFrameworkCases(
		buildZincStaticHandler,
		buildChiStaticHandler,
		buildEchoStaticHandler,
		buildGinStaticHandler,
	), buildBunRouterStaticHandler)
}

func parallelParamCases() []benchmarkCase {
	return withBunRouter(focusedFrameworkCases(
		buildZincParallelParamHandler,
		buildChiParallelParamHandler,
		buildEchoParallelParamHandler,
		buildGinParallelParamHandler,
	), buildBunRouterParallelParamHandler)
}

func parallelMiddlewareCases() []benchmarkCase {
	return focusedFrameworkCases(
		buildZincParallelMiddlewareHandler,
		buildChiParallelMiddlewareHandler,
		buildEchoParallelMiddlewareHandler,
		buildGinParallelMiddlewareHandler,
	)
}

func parallelAPIHappyPathCases() []benchmarkCase {
	return focusedFrameworkCases(
		buildZincParallelAPIHappyPathHandler,
		buildChiParallelAPIHappyPathHandler,
		buildEchoParallelAPIHappyPathHandler,
		buildGinParallelAPIHappyPathHandler,
	)
}

func BenchmarkParallelStaticRoute(b *testing.B) {
	runServeHTTPParallelBenchmarksWithProof(b, []*http.Request{httptest.NewRequest(http.MethodGet, "/hello", nil)}, parallelStaticCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, "/hello", http.StatusOK, benchmarkHelloResponse)
	})
}

func BenchmarkParallelRouterParam(b *testing.B) {
	pool := poolRequests(http.MethodGet, poolSize, 30, func(i int) string { return "/hello/" + poolValue("name", i) })
	runServeHTTPParallelBenchmarksWithProof(b, pool, parallelParamCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, "/hello/world", http.StatusOK, benchmarkOKResponse)
	})
}

// BenchmarkParallelRouteSetTraffic is ScenarioRouteSetTraffic on the GitHub
// API from every core at once.
func BenchmarkParallelRouteSetTraffic(b *testing.B) {
	github := scenarioNamed("GitHubAPI203")
	routes := github.routes
	picks := zipfIndexes(len(routes), poolSize, 46)
	pool := poolRequestsFor(poolSize, 47, func(i int) (string, string) {
		route := routes[picks[i]]
		return route.method, poolPath(route.pattern, i)
	})
	runServeHTTPParallelBenchmarksWithProof(b, pool, scenarioCases(github), nil)
}

func BenchmarkParallelMiddlewareChain(b *testing.B) {
	runServeHTTPParallelBenchmarksWithProof(b, []*http.Request{httptest.NewRequest(http.MethodGet, "/middleware", nil)}, parallelMiddlewareCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, "/middleware", http.StatusOK, benchmarkOKResponse)
	})
}

func BenchmarkParallelAPIHappyPath(b *testing.B) {
	target := benchmarkAPIQueryTarget()
	pool := poolRequests(http.MethodGet, poolSize, 31, func(i int) string { return poolTeamUser(i) + "?verbose=true&limit=25" })
	runServeHTTPParallelBenchmarksWithProof(b, pool, parallelAPIHappyPathCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse)
	})
}
