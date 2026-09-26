// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	. "github.com/0mjs/zinc"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v5"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.ReleaseMode)
	// Gin's binding runs the go-playground validator on every ShouldBind
	// call; Zinc and Echo validate only when asked. Turning it off gives all
	// three the same bind-only work. APIBindValidationFailure measures
	// validation with one shared check for every framework.
	binding.Validator = nil
	os.Exit(m.Run())
}

const (
	benchmarkHelloResponse = "Hello World!"
	benchmarkOKResponse    = "OK"
	staticColdRouteCount   = 64
	largeStaticRouteCount  = 256
	largeParamRouteCount   = 128
	coldPathRequestCount   = 256
)

var (
	benchmarkSinkString  string
	benchmarkSinkBool    bool
	benchmarkSinkHandler http.Handler
)

type benchmarkResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
	Data    struct {
		Items []string `json:"items"`
		Count int      `json:"count"`
	} `json:"data"`
}

var benchmarkJSONData = func() benchmarkResponse {
	resp := benchmarkResponse{
		Message: "Success",
		Status:  http.StatusOK,
	}
	resp.Data.Items = []string{"item1", "item2", "item3", "item4", "item5"}
	resp.Data.Count = len(resp.Data.Items)
	return resp
}()

type benchmarkAPIBindInput struct {
	TeamID  int      `path:"teamID" uri:"teamID" param:"teamID"`
	UserID  int      `path:"userID" uri:"userID" param:"userID"`
	Verbose bool     `query:"verbose" form:"verbose"`
	Limit   int      `query:"limit" form:"limit"`
	Name    string   `json:"name"`
	Roles   []string `json:"roles"`
}

type benchmarkAPIResponse struct {
	OK        bool   `json:"ok"`
	TeamID    int    `json:"team_id"`
	UserID    int    `json:"user_id"`
	Limit     int    `json:"limit"`
	RoleCount int    `json:"role_count"`
	Name      string `json:"name"`
}

var benchmarkAPIBindBody = []byte(`{"name":"alice","roles":["admin","editor"]}`)

type discardResponseWriter struct {
	header http.Header
	status int
	bytes  int
}

func newDiscardResponseWriter() *discardResponseWriter {
	return &discardResponseWriter{
		header: make(http.Header, 8),
	}
}

func (w *discardResponseWriter) Header() http.Header {
	return w.header
}

func (w *discardResponseWriter) WriteHeader(code int) {
	w.status = code
}

func (w *discardResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.bytes += len(p)
	return len(p), nil
}

func (w *discardResponseWriter) WriteString(s string) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.bytes += len(s)
	return len(s), nil
}

func (w *discardResponseWriter) reset() {
	clear(w.header)
	w.status = 0
	w.bytes = 0
}

type benchmarkCase struct {
	name  string
	build func() http.Handler
}

type preparedBenchmarkRequest struct {
	request *http.Request
	body    []byte
}

type middlewareContextKey int

const (
	middlewareKey1 middlewareContextKey = iota
	middlewareKey2
	middlewareKey3
	middlewareKey4
	middlewareKey5
)

func mustNoErr(err error) {
	if err != nil {
		panic(err)
	}
}

// writeText writes s as text/plain, the Content-Type that Zinc's, Echo's
// and Gin's String helpers set, the way a Chi or BunRouter handler would.
func writeText(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, s)
}

func newGinBenchmarkRouter() *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	return r
}

func newPreparedBenchmarkRequest(method, target string, body []byte, headers http.Header) preparedBenchmarkRequest {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequest(method, target, reader)
	for key, values := range headers {
		req.Header[key] = append([]string(nil), values...)
	}

	return preparedBenchmarkRequest{
		request: req,
		body:    append([]byte(nil), body...),
	}
}

func (r *preparedBenchmarkRequest) reset() *http.Request {
	if len(r.body) == 0 {
		return r.request
	}

	r.request.Body = io.NopCloser(bytes.NewReader(r.body))
	r.request.ContentLength = int64(len(r.body))
	r.request.Form = nil
	r.request.PostForm = nil
	r.request.MultipartForm = nil
	return r.request
}

func runServeHTTPBenchmarks(b *testing.B, method, target string, cases []benchmarkCase) {
	proveCases(b, cases, 1, func(int) *http.Request { return httptest.NewRequest(method, target, nil) })
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
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

func runServeHTTPRequestSetBenchmarks(b *testing.B, cases []benchmarkCase, requests []*http.Request) {
	proveCases(b, cases, len(requests), func(i int) *http.Request { return requests[i] })
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
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

func runPreparedRequestBenchmarks(b *testing.B, cases []benchmarkCase, request preparedBenchmarkRequest) {
	proveCases(b, cases, 1, func(int) *http.Request { return request.reset() })
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
			rw := newDiscardResponseWriter()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rw.reset()
				handler.ServeHTTP(rw, request.reset())
			}
			benchmarkSinkInt = rw.status + rw.bytes
		})
	}
}

func runPreparedRequestSetBenchmarks(b *testing.B, cases []benchmarkCase, requests []preparedBenchmarkRequest) {
	proveCases(b, cases, len(requests), func(i int) *http.Request { return requests[i].reset() })
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			handler := bc.build()
			rw := newDiscardResponseWriter()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rw.reset()
				req := &requests[i%len(requests)]
				handler.ServeHTTP(rw, req.reset())
			}
			benchmarkSinkInt = rw.status + rw.bytes
		})
	}
}

func runRegistrationBenchmarks(b *testing.B, cases []benchmarkCase) {
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchmarkSinkHandler = bc.build()
			}
		})
	}
}

func buildRequests(method string, targets []string) []*http.Request {
	requests := make([]*http.Request, len(targets))
	for i, target := range targets {
		requests[i] = httptest.NewRequest(method, target, nil)
	}
	return requests
}

func chiMiddleware(key middlewareContextKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), key, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func echoMiddleware(key string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set(key, true)
			return next(c)
		}
	}
}

func zincMiddleware(key string) Middleware {
	return func(c *Context) error {
		c.Set(key, true)
		return c.Next()
	}
}

func ginMiddleware(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(key, true)
		c.Next()
	}
}

func largeStaticPath(i int) string {
	return "/static/" + strconv.Itoa(i)
}

func largeParamPatternColon(i int) string {
	return "/teams/" + strconv.Itoa(i) + "/users/:id"
}

func largeParamPatternBrace(i int) string {
	return "/teams/" + strconv.Itoa(i) + "/users/{id}"
}

func mixedLargeStaticTargets(count int) []string {
	targets := make([]string, count)
	for i := 0; i < count; i++ {
		targets[i] = largeStaticPath(i % largeStaticRouteCount)
	}
	return targets
}

func benchmarkAPIQueryTarget() string {
	return "/teams/42/users/7?verbose=true&limit=25"
}

func benchmarkAPIBodyHeaders() http.Header {
	header := make(http.Header, 1)
	header.Set(HeaderContentType, "application/json")
	return header
}

func consumeBenchmarkAPIInput(input benchmarkAPIBindInput) {
	benchmarkSinkString = input.Name
	benchmarkSinkBool = input.Verbose
	benchmarkSinkInt = input.TeamID + input.UserID + input.Limit + len(input.Roles)
}

func benchmarkAPIResponseFrom(input benchmarkAPIBindInput) benchmarkAPIResponse {
	return benchmarkAPIResponse{
		OK:        true,
		TeamID:    input.TeamID,
		UserID:    input.UserID,
		Limit:     input.Limit,
		RoleCount: len(input.Roles),
		Name:      input.Name,
	}
}

func parseBenchmarkInt(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func fillBenchmarkAPIQuery(input *benchmarkAPIBindInput, req *http.Request) {
	values := req.URL.Query()
	input.Verbose = values.Get("verbose") == "true"
	input.Limit = parseBenchmarkInt(values.Get("limit"))
}

func buildZincHelloHandler() http.Handler {
	app := New()
	app.Get("/", func(c *Context) error {
		return c.String(benchmarkHelloResponse)
	})
	return app
}

func buildChiHelloHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, benchmarkHelloResponse)
	})
	return r
}

func buildEchoHelloHandler() http.Handler {
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkHelloResponse)
	})
	return e
}

func buildGinHelloHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkHelloResponse)
	})
	return r
}

func buildZincStaticHandler() http.Handler {
	app := New()
	app.Get("/hello", func(c *Context) error {
		return c.String(benchmarkHelloResponse)
	})
	return app
}

func buildChiStaticHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, benchmarkHelloResponse)
	})
	return r
}

func buildEchoStaticHandler() http.Handler {
	e := echo.New()
	e.GET("/hello", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkHelloResponse)
	})
	return e
}

func buildGinStaticHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/hello", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkHelloResponse)
	})
	return r
}

func buildZincParamHandler() http.Handler {
	app := New()
	app.Get("/hello/{name}", func(c *Context) error {
		benchmarkSinkString = c.Param("name")
		return c.String(benchmarkHelloResponse)
	})
	return app
}

func buildZincHandleHTTPHandler() http.Handler {
	app := New()
	app.HandleHTTP("GET /native/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		benchmarkSinkString = r.PathValue("id")
		_, _ = io.WriteString(w, benchmarkOKResponse)
	}))
	return app
}

func buildZincWrappedHTTPHandler() http.Handler {
	app := New()
	app.Get("/native/{id}", Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		benchmarkSinkString = r.PathValue("id")
		_, _ = io.WriteString(w, benchmarkOKResponse)
	})))
	return app
}

func zincHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		benchmarkSinkBool = r.Method == http.MethodGet
		next.ServeHTTP(w, r)
	})
}

func buildZincHandleHTTPWithMiddlewareHandler() http.Handler {
	app := New()
	app.UseHTTP(zincHTTPMiddleware)
	app.HandleHTTP("GET /native/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		benchmarkSinkString = r.PathValue("id")
		_, _ = io.WriteString(w, benchmarkOKResponse)
	}))
	return app
}

func buildChiParamHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request) {
		benchmarkSinkString = chi.URLParam(r, "name")
		writeText(w, benchmarkHelloResponse)
	})
	return r
}

func buildEchoParamHandler() http.Handler {
	e := echo.New()
	e.GET("/hello/:name", func(c *echo.Context) error {
		benchmarkSinkString = c.Param("name")
		return c.String(http.StatusOK, benchmarkHelloResponse)
	})
	return e
}

func buildGinParamHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/hello/:name", func(c *gin.Context) {
		benchmarkSinkString = c.Param("name")
		c.String(http.StatusOK, benchmarkHelloResponse)
	})
	return r
}

func buildZincJSONHandler() http.Handler {
	app := New()
	app.Get("/json", func(c *Context) error {
		return c.JSON(benchmarkJSONData)
	})
	return app
}

func buildChiJSONHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderContentType, "application/json")
		_ = json.NewEncoder(w).Encode(benchmarkJSONData)
	})
	return r
}

func buildEchoJSONHandler() http.Handler {
	e := echo.New()
	e.GET("/json", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, benchmarkJSONData)
	})
	return e
}

func buildGinJSONHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/json", func(c *gin.Context) {
		c.JSON(http.StatusOK, benchmarkJSONData)
	})
	return r
}

func buildZincQueryHandler() http.Handler {
	app := New()
	app.Get("/query", func(c *Context) error {
		benchmarkSinkBool = c.Query("name") != "" && c.Query("age") != "" && c.Query("city") != ""
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiQueryHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/query", func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		benchmarkSinkBool = values.Get("name") != "" && values.Get("age") != "" && values.Get("city") != ""
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoQueryHandler() http.Handler {
	e := echo.New()
	e.GET("/query", func(c *echo.Context) error {
		benchmarkSinkBool = c.QueryParam("name") != "" && c.QueryParam("age") != "" && c.QueryParam("city") != ""
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinQueryHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/query", func(c *gin.Context) {
		benchmarkSinkBool = c.Query("name") != "" && c.Query("age") != "" && c.Query("city") != ""
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func buildZincMiddlewareHandler() http.Handler {
	app := New()
	app.Use(
		zincMiddleware("mw1"),
		zincMiddleware("mw2"),
		zincMiddleware("mw3"),
		zincMiddleware("mw4"),
		zincMiddleware("mw5"),
	)
	app.Get("/middleware", func(c *Context) error {
		_, ok1 := c.Get("mw1")
		_, ok2 := c.Get("mw2")
		_, ok3 := c.Get("mw3")
		_, ok4 := c.Get("mw4")
		_, ok5 := c.Get("mw5")
		benchmarkSinkBool = ok1 && ok2 && ok3 && ok4 && ok5
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiMiddlewareHandler() http.Handler {
	r := chi.NewRouter()
	r.Use(chiMiddleware(middlewareKey1))
	r.Use(chiMiddleware(middlewareKey2))
	r.Use(chiMiddleware(middlewareKey3))
	r.Use(chiMiddleware(middlewareKey4))
	r.Use(chiMiddleware(middlewareKey5))
	r.Get("/middleware", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		benchmarkSinkBool = ctx.Value(middlewareKey1) != nil &&
			ctx.Value(middlewareKey2) != nil &&
			ctx.Value(middlewareKey3) != nil &&
			ctx.Value(middlewareKey4) != nil &&
			ctx.Value(middlewareKey5) != nil
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoMiddlewareHandler() http.Handler {
	e := echo.New()
	e.Use(
		echoMiddleware("mw1"),
		echoMiddleware("mw2"),
		echoMiddleware("mw3"),
		echoMiddleware("mw4"),
		echoMiddleware("mw5"),
	)
	e.GET("/middleware", func(c *echo.Context) error {
		benchmarkSinkBool = c.Get("mw1") != nil &&
			c.Get("mw2") != nil &&
			c.Get("mw3") != nil &&
			c.Get("mw4") != nil &&
			c.Get("mw5") != nil
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinMiddlewareHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.Use(
		ginMiddleware("mw1"),
		ginMiddleware("mw2"),
		ginMiddleware("mw3"),
		ginMiddleware("mw4"),
		ginMiddleware("mw5"),
	)
	r.GET("/middleware", func(c *gin.Context) {
		_, ok1 := c.Get("mw1")
		_, ok2 := c.Get("mw2")
		_, ok3 := c.Get("mw3")
		_, ok4 := c.Get("mw4")
		_, ok5 := c.Get("mw5")
		benchmarkSinkBool = ok1 && ok2 && ok3 && ok4 && ok5
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func buildZincNotFoundHandler() http.Handler {
	app := New()
	app.Get("/found", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiNotFoundHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/found", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoNotFoundHandler() http.Handler {
	e := echo.New()
	e.GET("/found", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinNotFoundHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/found", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func buildZincLargeStaticHandler() http.Handler {
	app := newZincErrorBenchmarkApp()
	for i := 0; i < largeStaticRouteCount; i++ {
		path := largeStaticPath(i)
		app.Get(path, func(c *Context) error {
			return c.String(benchmarkOKResponse)
		})
	}
	return app
}

func buildChiLargeStaticHandler() http.Handler {
	r := newChiErrorBenchmarkRouter()
	for i := 0; i < largeStaticRouteCount; i++ {
		path := largeStaticPath(i)
		r.Get(path, func(w http.ResponseWriter, r *http.Request) {
			writeText(w, benchmarkOKResponse)
		})
	}
	return r
}

func buildEchoLargeStaticHandler() http.Handler {
	e := newEchoErrorBenchmarkApp()
	for i := 0; i < largeStaticRouteCount; i++ {
		path := largeStaticPath(i)
		e.GET(path, func(c *echo.Context) error {
			return c.String(http.StatusOK, benchmarkOKResponse)
		})
	}
	return e
}

func buildGinLargeStaticHandler() http.Handler {
	r := newGinErrorBenchmarkRouter()
	for i := 0; i < largeStaticRouteCount; i++ {
		path := largeStaticPath(i)
		r.GET(path, func(c *gin.Context) {
			c.String(http.StatusOK, benchmarkOKResponse)
		})
	}
	return r
}

func buildZincLargeParamHandler() http.Handler {
	app := newZincErrorBenchmarkApp()
	for i := 0; i < largeParamRouteCount; i++ {
		path := largeParamPatternBrace(i)
		app.Get(path, func(c *Context) error {
			benchmarkSinkString = c.Param("id")
			return c.String(benchmarkOKResponse)
		})
	}
	return app
}

func buildChiLargeParamHandler() http.Handler {
	r := newChiErrorBenchmarkRouter()
	for i := 0; i < largeParamRouteCount; i++ {
		path := largeParamPatternBrace(i)
		r.Get(path, func(w http.ResponseWriter, r *http.Request) {
			benchmarkSinkString = chi.URLParam(r, "id")
			writeText(w, benchmarkOKResponse)
		})
	}
	return r
}

func buildEchoLargeParamHandler() http.Handler {
	e := newEchoErrorBenchmarkApp()
	for i := 0; i < largeParamRouteCount; i++ {
		path := largeParamPatternColon(i)
		e.GET(path, func(c *echo.Context) error {
			benchmarkSinkString = c.Param("id")
			return c.String(http.StatusOK, benchmarkOKResponse)
		})
	}
	return e
}

func buildGinLargeParamHandler() http.Handler {
	r := newGinErrorBenchmarkRouter()
	for i := 0; i < largeParamRouteCount; i++ {
		path := largeParamPatternColon(i)
		r.GET(path, func(c *gin.Context) {
			benchmarkSinkString = c.Param("id")
			c.String(http.StatusOK, benchmarkOKResponse)
		})
	}
	return r
}

func requestContextMiddlewareSatisfied(r *http.Request) bool {
	ctx := r.Context()
	return ctx.Value(middlewareKey1) != nil &&
		ctx.Value(middlewareKey2) != nil &&
		ctx.Value(middlewareKey3) != nil &&
		ctx.Value(middlewareKey4) != nil &&
		ctx.Value(middlewareKey5) != nil
}

func zincMiddlewareSatisfied(c *Context) bool {
	_, ok1 := c.Get("mw1")
	_, ok2 := c.Get("mw2")
	_, ok3 := c.Get("mw3")
	_, ok4 := c.Get("mw4")
	_, ok5 := c.Get("mw5")
	return ok1 && ok2 && ok3 && ok4 && ok5
}

func echoMiddlewareSatisfied(c *echo.Context) bool {
	return c.Get("mw1") != nil &&
		c.Get("mw2") != nil &&
		c.Get("mw3") != nil &&
		c.Get("mw4") != nil &&
		c.Get("mw5") != nil
}

func ginMiddlewareSatisfied(c *gin.Context) bool {
	_, ok1 := c.Get("mw1")
	_, ok2 := c.Get("mw2")
	_, ok3 := c.Get("mw3")
	_, ok4 := c.Get("mw4")
	_, ok5 := c.Get("mw5")
	return ok1 && ok2 && ok3 && ok4 && ok5
}

func buildZincAPIParamQueryJSONHandler() http.Handler {
	app := New()
	app.Get("/teams/{teamID}/users/{userID}", func(c *Context) error {
		var input benchmarkAPIBindInput
		if err := c.Bind().All(&input); err != nil {
			return err
		}
		consumeBenchmarkAPIInput(input)
		return c.JSON(benchmarkAPIResponseFrom(input))
	})
	return app
}

func buildChiAPIParamQueryJSONHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/teams/{teamID}/users/{userID}", func(w http.ResponseWriter, req *http.Request) {
		input := benchmarkAPIBindInput{
			TeamID: parseBenchmarkInt(chi.URLParam(req, "teamID")),
			UserID: parseBenchmarkInt(chi.URLParam(req, "userID")),
		}
		fillBenchmarkAPIQuery(&input, req)
		consumeBenchmarkAPIInput(input)
		w.Header().Set(HeaderContentType, "application/json")
		_ = json.NewEncoder(w).Encode(benchmarkAPIResponseFrom(input))
	})
	return r
}

func buildEchoAPIParamQueryJSONHandler() http.Handler {
	e := echo.New()
	e.GET("/teams/:teamID/users/:userID", func(c *echo.Context) error {
		var input benchmarkAPIBindInput
		if err := c.Bind(&input); err != nil {
			return err
		}
		consumeBenchmarkAPIInput(input)
		return c.JSON(http.StatusOK, benchmarkAPIResponseFrom(input))
	})
	return e
}

func buildGinAPIParamQueryJSONHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/teams/:teamID/users/:userID", func(c *gin.Context) {
		var input benchmarkAPIBindInput
		if err := c.ShouldBindUri(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindQuery(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		consumeBenchmarkAPIInput(input)
		c.JSON(http.StatusOK, benchmarkAPIResponseFrom(input))
	})
	return r
}

func buildZincAPIHappyPathHandler() http.Handler {
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
			return err
		}
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && zincMiddlewareSatisfied(c)
		return c.JSON(benchmarkAPIResponseFrom(input))
	})
	return app
}

func buildChiAPIHappyPathHandler() http.Handler {
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
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && requestContextMiddlewareSatisfied(req)
		w.Header().Set(HeaderContentType, "application/json")
		_ = json.NewEncoder(w).Encode(benchmarkAPIResponseFrom(input))
	})
	return r
}

func buildEchoAPIHappyPathHandler() http.Handler {
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
			return err
		}
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && echoMiddlewareSatisfied(c)
		return c.JSON(http.StatusOK, benchmarkAPIResponseFrom(input))
	})
	return e
}

func buildGinAPIHappyPathHandler() http.Handler {
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
			c.Status(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindQuery(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && ginMiddlewareSatisfied(c)
		c.JSON(http.StatusOK, benchmarkAPIResponseFrom(input))
	})
	return r
}

func buildZincAPIBindJSONHappyPathHandler() http.Handler {
	app := New()
	app.Use(
		zincMiddleware("mw1"),
		zincMiddleware("mw2"),
		zincMiddleware("mw3"),
		zincMiddleware("mw4"),
		zincMiddleware("mw5"),
	)
	app.Post("/teams/{teamID}/users/{userID}", func(c *Context) error {
		var input benchmarkAPIBindInput
		if err := c.Bind().All(&input); err != nil {
			return err
		}
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && zincMiddlewareSatisfied(c)
		return c.JSON(benchmarkAPIResponseFrom(input))
	})
	return app
}

func buildChiAPIBindJSONHappyPathHandler() http.Handler {
	r := chi.NewRouter()
	r.Use(chiMiddleware(middlewareKey1))
	r.Use(chiMiddleware(middlewareKey2))
	r.Use(chiMiddleware(middlewareKey3))
	r.Use(chiMiddleware(middlewareKey4))
	r.Use(chiMiddleware(middlewareKey5))
	r.Post("/teams/{teamID}/users/{userID}", func(w http.ResponseWriter, req *http.Request) {
		input := benchmarkAPIBindInput{
			TeamID: parseBenchmarkInt(chi.URLParam(req, "teamID")),
			UserID: parseBenchmarkInt(chi.URLParam(req, "userID")),
		}
		fillBenchmarkAPIQuery(&input, req)
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && requestContextMiddlewareSatisfied(req)
		w.Header().Set(HeaderContentType, "application/json")
		_ = json.NewEncoder(w).Encode(benchmarkAPIResponseFrom(input))
	})
	return r
}

func buildEchoAPIBindJSONHappyPathHandler() http.Handler {
	e := echo.New()
	e.Use(
		echoMiddleware("mw1"),
		echoMiddleware("mw2"),
		echoMiddleware("mw3"),
		echoMiddleware("mw4"),
		echoMiddleware("mw5"),
	)
	e.POST("/teams/:teamID/users/:userID", func(c *echo.Context) error {
		var input benchmarkAPIBindInput
		// Echo's Bind reads the query only for GET, DELETE and HEAD.
		if err := echo.BindQueryParams(c, &input); err != nil {
			return err
		}
		if err := c.Bind(&input); err != nil {
			return err
		}
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && echoMiddlewareSatisfied(c)
		return c.JSON(http.StatusOK, benchmarkAPIResponseFrom(input))
	})
	return e
}

func buildGinAPIBindJSONHappyPathHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.Use(
		ginMiddleware("mw1"),
		ginMiddleware("mw2"),
		ginMiddleware("mw3"),
		ginMiddleware("mw4"),
		ginMiddleware("mw5"),
	)
	r.POST("/teams/:teamID/users/:userID", func(c *gin.Context) {
		var input benchmarkAPIBindInput
		if err := c.ShouldBindUri(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindQuery(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		consumeBenchmarkAPIInput(input)
		benchmarkSinkBool = benchmarkSinkBool && ginMiddlewareSatisfied(c)
		c.JSON(http.StatusOK, benchmarkAPIResponseFrom(input))
	})
	return r
}

func helloWorldCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincHelloHandler},
		{name: "Chi", build: buildChiHelloHandler},
		{name: "Echo", build: buildEchoHelloHandler},
		{name: "Gin", build: buildGinHelloHandler},
		{name: "BunRouter", build: buildBunRouterHelloHandler},
	}
}

func staticRouteCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincStaticHandler},
		{name: "Chi", build: buildChiStaticHandler},
		{name: "Echo", build: buildEchoStaticHandler},
		{name: "Gin", build: buildGinStaticHandler},
		{name: "BunRouter", build: buildBunRouterStaticHandler},
	}
}

func paramCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincParamHandler},
		{name: "Chi", build: buildChiParamHandler},
		{name: "Echo", build: buildEchoParamHandler},
		{name: "Gin", build: buildGinParamHandler},
		{name: "BunRouter", build: buildBunRouterParamHandler},
	}
}

func jsonCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincJSONHandler},
		{name: "Chi", build: buildChiJSONHandler},
		{name: "Echo", build: buildEchoJSONHandler},
		{name: "Gin", build: buildGinJSONHandler},
	}
}

func queryCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincQueryHandler},
		{name: "Chi", build: buildChiQueryHandler},
		{name: "Echo", build: buildEchoQueryHandler},
		{name: "Gin", build: buildGinQueryHandler},
	}
}

func middlewareCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincMiddlewareHandler},
		{name: "Chi", build: buildChiMiddlewareHandler},
		{name: "Echo", build: buildEchoMiddlewareHandler},
		{name: "Gin", build: buildGinMiddlewareHandler},
	}
}

func notFoundCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincNotFoundHandler},
		{name: "Chi", build: buildChiNotFoundHandler},
		{name: "Echo", build: buildEchoNotFoundHandler},
		{name: "Gin", build: buildGinNotFoundHandler},
	}
}

func largeStaticCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincLargeStaticHandler},
		{name: "Chi", build: buildChiLargeStaticHandler},
		{name: "Echo", build: buildEchoLargeStaticHandler},
		{name: "Gin", build: buildGinLargeStaticHandler},
		{name: "BunRouter", build: buildBunRouterLargeStaticHandler},
	}
}

func largeParamCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincLargeParamHandler},
		{name: "Chi", build: buildChiLargeParamHandler},
		{name: "Echo", build: buildEchoLargeParamHandler},
		{name: "Gin", build: buildGinLargeParamHandler},
		{name: "BunRouter", build: buildBunRouterLargeParamHandler},
	}
}

func apiParamQueryJSONCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincAPIParamQueryJSONHandler},
		{name: "Chi", build: buildChiAPIParamQueryJSONHandler},
		{name: "Echo", build: buildEchoAPIParamQueryJSONHandler},
		{name: "Gin", build: buildGinAPIParamQueryJSONHandler},
	}
}

func apiHappyPathCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincAPIHappyPathHandler},
		{name: "Chi", build: buildChiAPIHappyPathHandler},
		{name: "Echo", build: buildEchoAPIHappyPathHandler},
		{name: "Gin", build: buildGinAPIHappyPathHandler},
	}
}

func apiBindJSONHappyPathCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincAPIBindJSONHappyPathHandler},
		{name: "Chi", build: buildChiAPIBindJSONHappyPathHandler},
		{name: "Echo", build: buildEchoAPIBindJSONHappyPathHandler},
		{name: "Gin", build: buildGinAPIBindJSONHappyPathHandler},
	}
}

func BenchmarkHelloWorld(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/", helloWorldCases())
}

func BenchmarkStaticRoute(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/hello", staticRouteCases())
}

// BenchmarkCacheBestCase keeps three one-URL requests from the 0.4 suite.
// Repeating one path is the best case for any per-path cache, which only Zinc
// has, so these are labelled as such and read next to their pooled versions.
func BenchmarkCacheBestCase(b *testing.B) {
	b.Run("RouterParam", func(b *testing.B) {
		runServeHTTPBenchmarks(b, http.MethodGet, "/hello/world", paramCases())
	})
	github := scenarioNamed("GitHubAPI203")
	b.Run("GitHubAPI203Param", func(b *testing.B) {
		runServeHTTPRequestSetBenchmarks(b, scenarioCases(github), []*http.Request{github.paramRequest})
	})
	b.Run("LargeRouteSetNotFound", func(b *testing.B) {
		runServeHTTPBenchmarks(b, http.MethodGet, "/static/missing", largeStaticCases())
	})
}

func BenchmarkRouterParam(b *testing.B) {
	runServeHTTPRequestSetBenchmarks(b, paramCases(), poolRequests(http.MethodGet, poolSize, 1, func(i int) string {
		return "/hello/" + poolValue("name", i)
	}))
}

func BenchmarkHTTPHandlerRoute(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/native/42", []benchmarkCase{
		{name: "HandleHTTP", build: buildZincHandleHTTPHandler},
		{name: "GetWrap", build: buildZincWrappedHTTPHandler},
	})
}

func BenchmarkHTTPMiddleware(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/native/42", []benchmarkCase{
		{name: "None", build: buildZincHandleHTTPHandler},
		{name: "UseHTTP", build: buildZincHandleHTTPWithMiddlewareHandler},
	})
}

func BenchmarkJSONResponse(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/json", jsonCases())
}

func BenchmarkQueryParams(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/query?name=john&age=25&city=london", queryCases())
}

func BenchmarkMiddlewareChain(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/middleware", middlewareCases())
}

// BenchmarkNotFound keeps each framework's default 404 response; the routing
// scenarios' misses use one normalised response instead.
func BenchmarkNotFound(b *testing.B) {
	runServeHTTPRequestSetBenchmarks(b, notFoundCases(), poolRequests(http.MethodGet, poolSize, 2, poolMissingPaths("", poolSize)))
}

func BenchmarkLargeRouteSetStatic(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, largeStaticPath(largeStaticRouteCount-1), largeStaticCases())
}

func BenchmarkLargeRouteSetStaticMixed(b *testing.B) {
	runServeHTTPRequestSetBenchmarks(b, largeStaticCases(), buildRequests(http.MethodGet, mixedLargeStaticTargets(coldPathRequestCount)))
}

func BenchmarkLargeRouteSetNotFound(b *testing.B) {
	runServeHTTPRequestSetBenchmarks(b, largeStaticCases(), poolRequests(http.MethodGet, poolSize, 3, poolMissingPaths("/static", poolSize)))
}

func BenchmarkLargeRouteSetMethodMismatch(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodPost, largeStaticPath(largeStaticRouteCount-1), largeStaticCases())
}

func BenchmarkLargeRouteSetParam(b *testing.B) {
	last := strconv.Itoa(largeParamRouteCount - 1)
	runServeHTTPRequestSetBenchmarks(b, largeParamCases(), poolRequests(http.MethodGet, poolSize, 4, func(i int) string {
		return "/teams/" + last + "/users/" + poolID(i)
	}))
}

func BenchmarkLargeRouteSetParamMixed(b *testing.B) {
	runServeHTTPRequestSetBenchmarks(b, largeParamCases(), poolRequests(http.MethodGet, poolSize, 5, func(i int) string {
		return "/teams/" + strconv.Itoa(i%largeParamRouteCount) + "/users/" + poolID(i)
	}))
}

func BenchmarkRouteRegistrationStatic(b *testing.B) {
	runRegistrationBenchmarks(b, largeStaticCases())
}

func BenchmarkRouteRegistrationParam(b *testing.B) {
	runRegistrationBenchmarks(b, largeParamCases())
}

func BenchmarkAPIParamQueryJSON(b *testing.B) {
	runServeHTTPRequestSetBenchmarks(b, apiParamQueryJSONCases(), poolRequests(http.MethodGet, poolSize, 6, func(i int) string {
		return poolTeamUser(i) + "?verbose=true&limit=25"
	}))
}

func BenchmarkAPIHappyPath(b *testing.B) {
	runServeHTTPRequestSetBenchmarks(b, apiHappyPathCases(), poolRequests(http.MethodGet, poolSize, 7, func(i int) string {
		return poolTeamUser(i) + "?verbose=true&limit=25"
	}))
}

func BenchmarkAPIBindJSONHappyPath(b *testing.B) {
	runPreparedRequestSetBenchmarks(b, apiBindJSONHappyPathCases(), poolPrepared(http.MethodPost, poolSize, 8, func(i int) string {
		return poolTeamUser(i) + "?verbose=true&limit=25"
	}, benchmarkAPIBindBody, benchmarkAPIBodyHeaders()))
}

func TestRunBenchmarks(t *testing.T) {
	t.Skip(`
Suite v2, from benchmarks/:
    go test -run=^$ -bench . -benchmem             every scenario
    go test -run=^$ -bench '^BenchmarkParallel'    the parallel slice
    go test -tags rps -run=^$ -bench '^BenchmarkRequestsPerSecond$' -count=3

Record runs with zincbench (see README.md) rather than reading raw output.

How the suite stays honest:
- Every framework in a scenario receives the same requests, built before
  timing. Parameter, miss and wildcard scenarios cycle through a pool of
  10,000 distinct paths (pools_test.go): ten times Zinc's route cache, the
  only per-path cache in the suite. CacheBestCase keeps three one-URL
  requests, labelled as the cache's best case.
- Before timing, every scenario proves its frameworks agree on status, body,
  media type, Allow and the parameters read (proof_test.go).
- Frameworks are scored against Gin and Echo on every scenario, and against
  BunRouter and Chi on the routing scenarios BunRouter runs. Routing
  scenarios use one normalised 404/405 response per framework; NotFound and
  StaticFileNotFound keep each framework's default.
- Gin's binding validator is off, so every framework does bind-only work.
- The RPS benchmark needs the rps build tag and a real loopback listener;
  compare reqs/s, not ns/op.
`)
}
