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
	benchmarkNotFoundResponse         = "NOT_FOUND"
	benchmarkMethodNotAllowedResponse = "METHOD_NOT_ALLOWED"
)

func focusedFrameworkCases(zinc, chiCase, echoCase, ginCase func() http.Handler) []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: zinc},
		{name: "Chi", build: chiCase},
		{name: "Echo", build: echoCase},
		{name: "Gin", build: ginCase},
	}
}

func runServeHTTPBenchmarksWithProof(
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

func newZincErrorBenchmarkApp() *App {
	app := New()
	app.NotFound(func(c *Context) error {
		return c.Status(http.StatusNotFound).String(benchmarkNotFoundResponse)
	})
	app.MethodNotAllowed(func(c *Context) error {
		return c.Status(http.StatusMethodNotAllowed).String(benchmarkMethodNotAllowedResponse)
	})
	return app
}

func newChiErrorBenchmarkRouter() chi.Router {
	r := chi.NewRouter()
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, benchmarkNotFoundResponse)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, benchmarkMethodNotAllowedResponse)
	})
	return r
}

func newEchoErrorBenchmarkApp() *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		switch {
		case errors.Is(err, echo.ErrMethodNotAllowed):
			_ = c.String(http.StatusMethodNotAllowed, benchmarkMethodNotAllowedResponse)
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
		c.String(http.StatusMethodNotAllowed, benchmarkMethodNotAllowedResponse)
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
		_, _ = io.WriteString(w, benchmarkOKResponse)
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
	return focusedFrameworkCases(
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
	)
}

func param10Cases() []benchmarkCase {
	paramNames := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9", "p10"}
	return focusedFrameworkCases(
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
	)
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
				_, _ = io.WriteString(w, benchmarkOKResponse)
			})
			r.Route("/teams", func(r chi.Router) {
				r.Get("/status/health", func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, benchmarkOKResponse)
				})
				r.Get("/{teamID}", func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, benchmarkOKResponse)
				})
				r.Route("/{teamID}/users", func(r chi.Router) {
					r.Get("/", func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.WriteString(w, benchmarkOKResponse)
					})
					r.Get("/{userID}", func(w http.ResponseWriter, req *http.Request) {
						benchmarkSinkString = chi.URLParam(req, "teamID") + "|" + chi.URLParam(req, "userID")
						_, _ = io.WriteString(w, benchmarkOKResponse)
					})
					r.Get("/{userID}/preferences", func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.WriteString(w, benchmarkOKResponse)
					})
				})
			})
			r.Route("/projects", func(r chi.Router) {
				r.Get("/{projectId}/builds", func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, benchmarkOKResponse)
				})
				r.Get("/{projectId}/builds/{number}", func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, benchmarkOKResponse)
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
	return focusedFrameworkCases(
		buildZincNestedGroupHandler,
		buildChiNestedGroupHandler,
		buildEchoNestedGroupHandler,
		buildGinNestedGroupHandler,
	)
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
		_, _ = io.WriteString(w, benchmarkOKResponse)
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
	return focusedFrameworkCases(
		buildZincWildcardHandler,
		buildChiWildcardHandler,
		buildEchoWildcardHandler,
		buildGinWildcardHandler,
	)
}

func BenchmarkParam5(b *testing.B) {
	target := "/orgs/openai/repos/zinc/issues/42/comments/7/reactions/heart"
	want := stringLengthSum("openai", "zinc", "42", "7", "heart")
	runServeHTTPBenchmarksWithProof(b, http.MethodGet, target, param5Cases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponseAndSinkInt(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse, want)
	})
}

func BenchmarkParam10(b *testing.B) {
	target := "/v1/1/a/2/b/3/c/4/d/5/e/6/f/7/g/8/h/9/i/10"
	want := stringLengthSum("1", "2", "3", "4", "5", "6", "7", "8", "9", "10")
	runServeHTTPBenchmarksWithProof(b, http.MethodGet, target, param10Cases(), func(b *testing.B, _ string, handler http.Handler) {
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
	runServeHTTPBenchmarksWithProof(b, http.MethodGet, target, nestedGroupCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponseAndSinkString(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse, "42|7")
	})
}

func BenchmarkNestedGroupNotFound(b *testing.B) {
	target := "/api/v1/teams/42/users/7/missing"
	runServeHTTPBenchmarksWithProof(b, http.MethodGet, target, nestedGroupCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, target, http.StatusNotFound, benchmarkNotFoundResponse)
	})
}

func BenchmarkNestedGroupMethodMismatch(b *testing.B) {
	target := "/api/v1/teams/42/users/7/preferences"
	runServeHTTPBenchmarksWithProof(b, http.MethodPost, target, nestedGroupCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodPost, target, http.StatusMethodNotAllowed, benchmarkMethodNotAllowedResponse)
	})
}

func BenchmarkWildcardTail(b *testing.B) {
	target := "/files/css/app/main.css"
	runServeHTTPBenchmarksWithProof(b, http.MethodGet, target, wildcardCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponseAndSinkString(b, handler, http.MethodGet, target, http.StatusOK, benchmarkOKResponse, "css/app/main.css")
	})
}

func BenchmarkWildcardTailNotFound(b *testing.B) {
	target := "/assets/css/app/main.css"
	runServeHTTPBenchmarksWithProof(b, http.MethodGet, target, wildcardCases(), func(b *testing.B, _ string, handler http.Handler) {
		proveResponse(b, handler, http.MethodGet, target, http.StatusNotFound, benchmarkNotFoundResponse)
	})
}
