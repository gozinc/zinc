// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/0mjs/zinc"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v5"
)

func runServeHTTPParallelBenchmarksWithProof(
	b *testing.B,
	method,
	target string,
	cases []benchmarkCase,
	prove func(*testing.B, string, http.Handler),
) {
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
			if prove != nil {
				prove(b, bc.name, handler)
			}

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				req := httptest.NewRequest(method, target, nil)
				rw := newDiscardResponseWriter()
				for pb.Next() {
					rw.reset()
					handler.ServeHTTP(rw, req)
				}
				benchmarkSinkInt = rw.status + rw.bytes
			})
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
			_, _ = io.WriteString(w, "BAD")
			return
		}
		_, _ = io.WriteString(w, benchmarkOKResponse)
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
			_, _ = io.WriteString(w, "BAD")
			return
		}
		_, _ = io.WriteString(w, benchmarkOKResponse)
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
			_, _ = io.WriteString(w, "BAD")
			return
		}
		_, _ = io.WriteString(w, benchmarkOKResponse)
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
	return focusedFrameworkCases(
		buildZincStaticHandler,
		buildChiStaticHandler,
		buildEchoStaticHandler,
		buildGinStaticHandler,
	)
}

func parallelParamCases() []benchmarkCase {
	return focusedFrameworkCases(
		buildZincParallelParamHandler,
		buildChiParallelParamHandler,
		buildEchoParallelParamHandler,
		buildGinParallelParamHandler,
	)
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
	runServeHTTPParallelBenchmarksWithProof(b, http.MethodGet, "/hello", parallelStaticCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, "/hello", http.StatusOK, benchmarkHelloResponse)
	})
}

func BenchmarkParallelRouterParam(b *testing.B) {
	runServeHTTPParallelBenchmarksWithProof(b, http.MethodGet, "/hello/world", parallelParamCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, "/hello/world", http.StatusOK, benchmarkOKResponse)
	})
}

func BenchmarkParallelMiddlewareChain(b *testing.B) {
	runServeHTTPParallelBenchmarksWithProof(b, http.MethodGet, "/middleware", parallelMiddlewareCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, "/middleware", http.StatusOK, benchmarkOKResponse)
	})
}

func BenchmarkParallelAPIHappyPath(b *testing.B) {
	target := benchmarkAPIQueryTarget()
	runServeHTTPParallelBenchmarksWithProof(b, http.MethodGet, target, parallelAPIHappyPathCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse)
	})
}
