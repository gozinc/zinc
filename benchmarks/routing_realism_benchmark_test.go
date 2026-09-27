// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/0mjs/zinc"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v5"
)

const (
	benchmarkNotFoundResponse = "NOT_FOUND"
)

func focusedFrameworkCases(zinc, chiCase, echoCase, ginCase func() http.Handler) []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: zinc},
		{name: "Chi", build: chiCase},
		{name: "Echo", build: echoCase},
		{name: "Gin", build: ginCase},
	}
}

// withBunRouter adds BunRouter to a routing scenario's cases.
func withBunRouter(cases []benchmarkCase, bun func() http.Handler) []benchmarkCase {
	return append(cases, benchmarkCase{name: "BunRouter", build: bun})
}

func runServeHTTPBenchmarksWithProof(
	b *testing.B,
	method,
	target string,
	cases []benchmarkCase,
	prove func(*testing.B, string, http.Handler),
) {
	proveCases(b, cases, 1, func(int) *http.Request { return httptest.NewRequest(method, target, nil) })
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
			if prove != nil {
				prove(b, bc.name, handler)
			}

			req := httptest.NewRequest(method, target, nil)
			rw := newDiscardResponseWriter()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rw.reset()
				handler.ServeHTTP(rw, req)
			}
			benchmarkSinkInt = rw.status + rw.bytes
		})
	}
}

// runPoolBenchmarksWithProof is runServeHTTPBenchmarksWithProof over a pool:
// prove checks each framework's exact answer on a canonical request, and the
// timed loop cycles through requests.
func runPoolBenchmarksWithProof(b *testing.B, requests []*http.Request, cases []benchmarkCase, prove func(*testing.B, string, http.Handler)) {
	proveCases(b, cases, len(requests), func(i int) *http.Request { return requests[i] })
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
			prove(b, bc.name, handler)
			rw := newDiscardResponseWriter()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rw.reset()
				handler.ServeHTTP(rw, requests[i%len(requests)])
			}
			benchmarkSinkInt = rw.status + rw.bytes
		})
	}
}

func proveResponse(t testing.TB, handler http.Handler, method, target string, wantStatus int, wantBody string) {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Fatalf("status=%d want=%d", rec.Code, wantStatus)
	}
	if rec.Body.String() != wantBody {
		t.Fatalf("body=%q want=%q", rec.Body.String(), wantBody)
	}
}

func proveResponseAndSinkInt(t testing.TB, handler http.Handler, method, target string, wantStatus int, wantBody string, wantSink int) {
	t.Helper()

	benchmarkSinkInt = 0
	proveResponse(t, handler, method, target, wantStatus, wantBody)
	if benchmarkSinkInt != wantSink {
		t.Fatalf("sink=%d want=%d", benchmarkSinkInt, wantSink)
	}
}

func proveResponseAndSinkString(t testing.TB, handler http.Handler, method, target string, wantStatus int, wantBody, wantSink string) {
	t.Helper()

	benchmarkSinkString = ""
	proveResponse(t, handler, method, target, wantStatus, wantBody)
	if benchmarkSinkString != wantSink {
		t.Fatalf("sink=%q want=%q", benchmarkSinkString, wantSink)
	}
}

func stringLengthSum(values ...string) int {
	total := 0
	for _, value := range values {
		total += len(value)
	}
	return total
}

func trimBenchmarkWildcard(value string) string {
	return strings.TrimPrefix(value, "/")
}

// The routing scenarios use one normalised miss response per framework, so
// they measure the lookup rather than each framework's default error body:
// a 404 writes benchmarkNotFoundResponse as text, and a 405 writes the status
// and the Allow header with no body. Each framework sets Allow itself before
// its 405 handler runs; Chi's default 405 handler is already this response,
// and a custom Chi handler couldn't see the allowed methods, so Chi keeps it.

func newZincErrorBenchmarkApp() *App {
	app := New()
	app.NotFound(func(c *Context) error {
		return c.Status(http.StatusNotFound).String(benchmarkNotFoundResponse)
	})
	app.MethodNotAllowed(func(c *Context) error {
		return c.Status(http.StatusMethodNotAllowed).NoContent()
	})
	return app
}

func newChiErrorBenchmarkRouter() chi.Router {
	r := chi.NewRouter()
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, benchmarkNotFoundResponse)
	})
	return r
}

func newEchoErrorBenchmarkApp() *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		switch {
		case errors.Is(err, echo.ErrMethodNotAllowed):
			_ = c.NoContent(http.StatusMethodNotAllowed)
		case errors.Is(err, echo.ErrNotFound):
			_ = c.String(http.StatusNotFound, benchmarkNotFoundResponse)
		default:
			_ = c.String(http.StatusInternalServerError, err.Error())
		}
	}
	return e
}

func newGinErrorBenchmarkRouter() *gin.Engine {
	r := newGinBenchmarkRouter()
	r.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, benchmarkNotFoundResponse)
	})
	r.NoMethod(func(c *gin.Context) {
		// Gin writes its own body unless the handler has written.
		c.Status(http.StatusMethodNotAllowed)
		c.Writer.WriteHeaderNow()
	})
	return r
}

func buildZincMultiParamHandler(pattern string, paramNames []string) http.Handler {
	app := New()
	app.Get(pattern, func(c *Context) error {
		scenarioParamScoreZinc(paramNames, c)
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiMultiParamHandler(pattern string, paramNames []string) http.Handler {
	r := chi.NewRouter()
	r.Get(pattern, func(w http.ResponseWriter, req *http.Request) {
		scenarioParamScoreChi(paramNames, req)
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoMultiParamHandler(pattern string, paramNames []string) http.Handler {
	e := echo.New()
	e.GET(pattern, func(c *echo.Context) error {
		scenarioParamScoreEcho(paramNames, c)
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinMultiParamHandler(pattern string, paramNames []string) http.Handler {
	r := newGinBenchmarkRouter()
	r.GET(pattern, func(c *gin.Context) {
		scenarioParamScoreGin(paramNames, c)
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func param5Cases() []benchmarkCase {
	paramNames := []string{"org", "repo", "issue", "comment", "reaction"}
	return withBunRouter(focusedFrameworkCases(
		func() http.Handler {
			return buildZincMultiParamHandler("/orgs/{org}/repos/{repo}/issues/{issue}/comments/{comment}/reactions/{reaction}", paramNames)
		},
		func() http.Handler {
			return buildChiMultiParamHandler("/orgs/{org}/repos/{repo}/issues/{issue}/comments/{comment}/reactions/{reaction}", paramNames)
		},
		func() http.Handler {
			return buildEchoMultiParamHandler("/orgs/:org/repos/:repo/issues/:issue/comments/:comment/reactions/:reaction", paramNames)
		},
		func() http.Handler {
			return buildGinMultiParamHandler("/orgs/:org/repos/:repo/issues/:issue/comments/:comment/reactions/:reaction", paramNames)
		},
	), func() http.Handler {
		return buildBunRouterMultiParamHandler("/orgs/:org/repos/:repo/issues/:issue/comments/:comment/reactions/:reaction", paramNames)
	})
}

func param10Cases() []benchmarkCase {
	paramNames := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9", "p10"}
	return withBunRouter(focusedFrameworkCases(
		func() http.Handler {
			return buildZincMultiParamHandler("/v1/{p1}/a/{p2}/b/{p3}/c/{p4}/d/{p5}/e/{p6}/f/{p7}/g/{p8}/h/{p9}/i/{p10}", paramNames)
		},
		func() http.Handler {
			return buildChiMultiParamHandler("/v1/{p1}/a/{p2}/b/{p3}/c/{p4}/d/{p5}/e/{p6}/f/{p7}/g/{p8}/h/{p9}/i/{p10}", paramNames)
		},
		func() http.Handler {
			return buildEchoMultiParamHandler("/v1/:p1/a/:p2/b/:p3/c/:p4/d/:p5/e/:p6/f/:p7/g/:p8/h/:p9/i/:p10", paramNames)
		},
		func() http.Handler {
			return buildGinMultiParamHandler("/v1/:p1/a/:p2/b/:p3/c/:p4/d/:p5/e/:p6/f/:p7/g/:p8/h/:p9/i/:p10", paramNames)
		},
	), func() http.Handler {
		return buildBunRouterMultiParamHandler("/v1/:p1/a/:p2/b/:p3/c/:p4/d/:p5/e/:p6/f/:p7/g/:p8/h/:p9/i/:p10", paramNames)
	})
}

func buildZincNestedGroupHandler() http.Handler {
	app := newZincErrorBenchmarkApp()
	api := app.Group("/api")
	v1 := api.Group("/v1")
	v1.Get("/health", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})

	teams := v1.Group("/teams")
	teams.Get("/status/health", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})
	teams.Get("/{teamID}", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})

	users := teams.Group("/{teamID}/users")
	users.Get("/", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})
	users.Get("/{userID}", func(c *Context) error {
		benchmarkSinkString = c.Param("teamID") + "|" + c.Param("userID")
		return c.String(benchmarkOKResponse)
	})
	users.Get("/{userID}/preferences", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})

	projects := v1.Group("/projects")
	projects.Get("/{projectId}/builds", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})
	projects.Get("/{projectId}/builds/{number}", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})

	return app
}

func buildChiNestedGroupHandler() http.Handler {
	r := newChiErrorBenchmarkRouter()
	r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
				writeText(w, benchmarkOKResponse)
			})
			r.Route("/teams", func(r chi.Router) {
				r.Get("/status/health", func(w http.ResponseWriter, r *http.Request) {
					writeText(w, benchmarkOKResponse)
				})
				r.Get("/{teamID}", func(w http.ResponseWriter, r *http.Request) {
					writeText(w, benchmarkOKResponse)
				})
				r.Route("/{teamID}/users", func(r chi.Router) {
					r.Get("/", func(w http.ResponseWriter, r *http.Request) {
						writeText(w, benchmarkOKResponse)
					})
					r.Get("/{userID}", func(w http.ResponseWriter, req *http.Request) {
						benchmarkSinkString = chi.URLParam(req, "teamID") + "|" + chi.URLParam(req, "userID")
						writeText(w, benchmarkOKResponse)
					})
					r.Get("/{userID}/preferences", func(w http.ResponseWriter, r *http.Request) {
						writeText(w, benchmarkOKResponse)
					})
				})
			})
			r.Route("/projects", func(r chi.Router) {
				r.Get("/{projectId}/builds", func(w http.ResponseWriter, r *http.Request) {
					writeText(w, benchmarkOKResponse)
				})
				r.Get("/{projectId}/builds/{number}", func(w http.ResponseWriter, r *http.Request) {
					writeText(w, benchmarkOKResponse)
				})
			})
		})
	})
	return r
}

func buildEchoNestedGroupHandler() http.Handler {
	e := newEchoErrorBenchmarkApp()
	api := e.Group("/api")
	v1 := api.Group("/v1")
	v1.GET("/health", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})

	teams := v1.Group("/teams")
	teams.GET("/status/health", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	teams.GET("/:teamID", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})

	users := teams.Group("/:teamID/users")
	users.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	users.GET("/:userID", func(c *echo.Context) error {
		benchmarkSinkString = c.Param("teamID") + "|" + c.Param("userID")
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	users.GET("/:userID/preferences", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})

	projects := v1.Group("/projects")
	projects.GET("/:projectId/builds", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	projects.GET("/:projectId/builds/:number", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})

	return e
}

func buildGinNestedGroupHandler() http.Handler {
	r := newGinErrorBenchmarkRouter()
	api := r.Group("/api")
	v1 := api.Group("/v1")
	v1.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})

	teams := v1.Group("/teams")
	teams.GET("/status/health", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	teams.GET("/:teamID", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})

	users := teams.Group("/:teamID/users")
	users.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	users.GET("/:userID", func(c *gin.Context) {
		benchmarkSinkString = c.Param("teamID") + "|" + c.Param("userID")
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	users.GET("/:userID/preferences", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})

	projects := v1.Group("/projects")
	projects.GET("/:projectId/builds", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	projects.GET("/:projectId/builds/:number", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})

	return r
}

func nestedGroupCases() []benchmarkCase {
	return withBunRouter(focusedFrameworkCases(
		buildZincNestedGroupHandler,
		buildChiNestedGroupHandler,
		buildEchoNestedGroupHandler,
		buildGinNestedGroupHandler,
	), buildBunRouterNestedGroupHandler)
}

func buildZincWildcardHandler() http.Handler {
	app := newZincErrorBenchmarkApp()
	app.Get("/files/{tail...}", func(c *Context) error {
		benchmarkSinkString = trimBenchmarkWildcard(c.Param("tail"))
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiWildcardHandler() http.Handler {
	r := newChiErrorBenchmarkRouter()
	r.Get("/files/*", func(w http.ResponseWriter, req *http.Request) {
		benchmarkSinkString = trimBenchmarkWildcard(chi.URLParam(req, "*"))
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoWildcardHandler() http.Handler {
	e := newEchoErrorBenchmarkApp()
	e.GET("/files/*", func(c *echo.Context) error {
		benchmarkSinkString = trimBenchmarkWildcard(c.Param("*"))
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinWildcardHandler() http.Handler {
	r := newGinErrorBenchmarkRouter()
	r.GET("/files/*tail", func(c *gin.Context) {
		benchmarkSinkString = trimBenchmarkWildcard(c.Param("tail"))
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func wildcardCases() []benchmarkCase {
	return withBunRouter(focusedFrameworkCases(
		buildZincWildcardHandler,
		buildChiWildcardHandler,
		buildEchoWildcardHandler,
		buildGinWildcardHandler,
	), buildBunRouterWildcardHandler)
}

func BenchmarkParam5(b *testing.B) {
	target := "/orgs/openai/repos/zinc/issues/42/comments/7/reactions/heart"
	want := stringLengthSum("openai", "zinc", "42", "7", "heart")
	pool := poolRequests(http.MethodGet, poolSize, 20, func(i int) string {
		return "/orgs/" + poolValue("org", i) + "/repos/zinc/issues/" + poolID(i) + "/comments/7/reactions/heart"
	})
	runPoolBenchmarksWithProof(b, pool, param5Cases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponseAndSinkInt(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse, want)
	})
}

func BenchmarkParam10(b *testing.B) {
	target := "/v1/1/a/2/b/3/c/4/d/5/e/6/f/7/g/8/h/9/i/10"
	want := stringLengthSum("1", "2", "3", "4", "5", "6", "7", "8", "9", "10")
	pool := poolRequests(http.MethodGet, poolSize, 21, func(i int) string {
		return "/v1/" + poolID(i) + "/a/2/b/3/c/4/d/5/e/6/f/7/g/8/h/9/i/" + poolID(poolSize-i)
	})
	runPoolBenchmarksWithProof(b, pool, param10Cases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponseAndSinkInt(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse, want)
	})
}

func BenchmarkNestedGroupStatic(b *testing.B) {
	runServeHTTPBenchmarksWithProof(
		b,
		http.MethodGet,
		"/api/v1/teams/status/health",
		nestedGroupCases(),
		func(b *testing.B, _ string, handler http.Handler) {
			proveResponse(b, handler, http.MethodGet, "/api/v1/teams/status/health", http.StatusOK, benchmarkOKResponse)
		},
	)
}

func BenchmarkNestedGroupParam(b *testing.B) {
	target := "/api/v1/teams/42/users/7"
	pool := poolRequests(http.MethodGet, poolSize, 22, func(i int) string { return "/api/v1" + poolTeamUser(i) })
	runPoolBenchmarksWithProof(b, pool, nestedGroupCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponseAndSinkString(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse, "42|7")
	})
}

func BenchmarkNestedGroupNotFound(b *testing.B) {
	target := "/api/v1/teams/42/users/7/missing"
	pool := poolRequests(http.MethodGet, poolSize, 23, func(i int) string { return "/api/v1" + poolTeamUser(i) + "/missing" })
	runPoolBenchmarksWithProof(b, pool, nestedGroupCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, target, http.StatusNotFound, benchmarkNotFoundResponse)
	})
}

func BenchmarkNestedGroupMethodMismatch(b *testing.B) {
	target := "/api/v1/teams/42/users/7/preferences"
	pool := poolRequests(http.MethodPost, poolSize, 24, func(i int) string { return "/api/v1" + poolTeamUser(i) + "/preferences" })
	runPoolBenchmarksWithProof(b, pool, withoutBunRouter(nestedGroupCases()), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodPost, target, http.StatusMethodNotAllowed, "")
	})
}

func BenchmarkWildcardTail(b *testing.B) {
	target := "/files/css/app/main.css"
	pool := poolRequests(http.MethodGet, poolSize, 25, func(i int) string { return "/files/" + poolTail(i) })
	runPoolBenchmarksWithProof(b, pool, wildcardCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponseAndSinkString(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse, "css/app/main.css")
	})
}

func BenchmarkWildcardTailNotFound(b *testing.B) {
	target := "/assets/css/app/main.css"
	pool := poolRequests(http.MethodGet, poolSize, 26, func(i int) string { return "/assets/" + poolTail(i) })
	runPoolBenchmarksWithProof(b, pool, wildcardCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, target, http.StatusNotFound, benchmarkNotFoundResponse)
	})
}
