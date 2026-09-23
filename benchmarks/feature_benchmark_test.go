// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/0mjs/zinc"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v5"
)

const (
	benchmarkStaticRoot   = "testdata/benchmark_static"
	benchmarkStaticTarget = "/assets/hello.txt"
)

type benchmarkHeaderQueryJSONInput struct {
	TeamID  int      `path:"teamID" uri:"teamID" param:"teamID"`
	UserID  int      `path:"userID" uri:"userID" param:"userID"`
	Verbose bool     `query:"verbose" form:"verbose"`
	Limit   int      `query:"limit" form:"limit"`
	Name    string   `json:"name"`
	Roles   []string `json:"roles"`
	TraceID string   `header:"X-Trace-ID"`
	Tenant  string   `header:"X-Tenant-ID"`
}

type benchmarkMultipartResult struct {
	Title      string
	Count      int
	TagCount   int
	Filename   string
	FileSize   int64
	HasComment bool
}

type benchmarkLargeJSONItem struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Active   bool     `json:"active"`
	Tags     []string `json:"tags"`
	Projects []string `json:"projects"`
}

type benchmarkLargeJSONEnvelope struct {
	OK      bool                     `json:"ok"`
	TeamID  int                      `json:"team_id"`
	Cursor  string                   `json:"cursor"`
	Message string                   `json:"message"`
	Items   []benchmarkLargeJSONItem `json:"items"`
}

type benchmarkValidationInput struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

var benchmarkHeaderQueryJSONBody = []byte(`{"name":"alice","roles":["admin","editor"]}`)
var benchmarkInvalidJSONBody = []byte(`{"name":"alice","roles":["admin","editor"}`)
var benchmarkValidationFailureBody = []byte(`{"name":"","roles":[]}`)
var benchmarkLargeJSONData = buildBenchmarkLargeJSONEnvelope()
var benchmarkLargeJSONBody = mustMarshalBenchmarkJSON(benchmarkLargeJSONData)
var benchmarkMultipartBody, benchmarkMultipartContentType = buildBenchmarkMultipartBody()

func buildBenchmarkLargeJSONEnvelope() benchmarkLargeJSONEnvelope {
	items := make([]benchmarkLargeJSONItem, 128)
	for i := range items {
		items[i] = benchmarkLargeJSONItem{
			ID:       i + 1,
			Name:     "user-" + strconv.Itoa(i+1),
			Email:    "user-" + strconv.Itoa(i+1) + "@example.com",
			Active:   i%2 == 0,
			Tags:     []string{"alpha", "beta", "tier-" + strconv.Itoa(i%4)},
			Projects: []string{"api", "web", "ops"},
		}
	}

	return benchmarkLargeJSONEnvelope{
		OK:      true,
		TeamID:  42,
		Cursor:  "cursor_1234567890",
		Message: strings.Repeat("payload-", 8),
		Items:   items,
	}
}

func mustMarshalBenchmarkJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func buildBenchmarkMultipartBody() ([]byte, string) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	mustNoErr(writer.WriteField("title", "Quarterly report"))
	mustNoErr(writer.WriteField("count", "25"))
	mustNoErr(writer.WriteField("comment", "uploaded from benchmark"))
	mustNoErr(writer.WriteField("tags", "alpha"))
	mustNoErr(writer.WriteField("tags", "beta"))

	part, err := writer.CreateFormFile("file", "report.txt")
	mustNoErr(err)
	_, err = io.WriteString(part, strings.Repeat("zinc-benchmark-file-", 32))
	mustNoErr(err)
	mustNoErr(writer.Close())

	return buf.Bytes(), writer.FormDataContentType()
}

func benchmarkHeaderQueryTarget() string {
	return "/teams/42/users/7?verbose=true&limit=25"
}

func benchmarkHeaderQueryHeaders() http.Header {
	header := benchmarkAPIBodyHeaders()
	header.Set("X-Trace-ID", "trace-benchmark-123")
	header.Set("X-Tenant-ID", "tenant-acme")
	return header
}

func benchmarkMultipartHeaders() http.Header {
	header := make(http.Header, 1)
	header.Set(HeaderContentType, benchmarkMultipartContentType)
	return header
}

func benchmarkMultipartTarget() string {
	return "/upload"
}

func benchmarkLargeJSONTarget() string {
	return "/bulk"
}

func benchmarkUnauthorizedTarget() string {
	return "/api/v1/secure/data"
}

func consumeBenchmarkHeaderQueryInput(input benchmarkHeaderQueryJSONInput) {
	benchmarkSinkString = input.TraceID + "|" + input.Tenant + "|" + input.Name
	benchmarkSinkBool = input.Verbose && input.TraceID != "" && input.Tenant != ""
	benchmarkSinkInt = input.TeamID + input.UserID + input.Limit + len(input.Roles)
}

func validateBenchmarkInput(input benchmarkValidationInput) bool {
	return input.Name != "" && len(input.Roles) > 0
}

func consumeBenchmarkLargeJSONEnvelope(input benchmarkLargeJSONEnvelope) {
	benchmarkSinkInt = input.TeamID + len(input.Items)
	benchmarkSinkString = input.Cursor
	benchmarkSinkBool = input.OK && len(input.Items) > 0
}

func consumeBenchmarkMultipart(result benchmarkMultipartResult) {
	benchmarkSinkInt = result.Count + result.TagCount + int(result.FileSize)
	benchmarkSinkString = result.Title + "|" + result.Filename
	benchmarkSinkBool = result.HasComment
}

func extractBenchmarkMultipart(req *http.Request) (benchmarkMultipartResult, error) {
	file, header, err := req.FormFile("file")
	if err != nil {
		return benchmarkMultipartResult{}, err
	}
	defer file.Close()

	_, err = io.Copy(io.Discard, file)
	if err != nil {
		return benchmarkMultipartResult{}, err
	}

	return benchmarkMultipartResult{
		Title:      req.FormValue("title"),
		Count:      parseBenchmarkInt(req.FormValue("count")),
		TagCount:   len(req.MultipartForm.Value["tags"]),
		Filename:   header.Filename,
		FileSize:   header.Size,
		HasComment: req.FormValue("comment") != "",
	}, nil
}

func buildZincAPIBindHeaderQueryJSONHandler() http.Handler {
	app := New()
	app.Post("/teams/{teamID}/users/{userID}", func(c *Context) error {
		var input benchmarkHeaderQueryJSONInput
		if err := c.Bind().Path(&input); err != nil {
			return err
		}
		if err := c.Bind().Query(&input); err != nil {
			return err
		}
		if err := c.Bind().Header(&input); err != nil {
			return err
		}
		if err := c.Bind().JSON(&input); err != nil {
			return err
		}
		consumeBenchmarkHeaderQueryInput(input)
		return c.JSON(benchmarkAPIResponse{
			OK:        true,
			TeamID:    input.TeamID,
			UserID:    input.UserID,
			Limit:     input.Limit,
			RoleCount: len(input.Roles),
			Name:      input.Name,
		})
	})
	return app
}

func buildChiAPIBindHeaderQueryJSONHandler() http.Handler {
	r := chi.NewRouter()
	r.Post("/teams/{teamID}/users/{userID}", func(w http.ResponseWriter, req *http.Request) {
		input := benchmarkHeaderQueryJSONInput{
			TeamID:  parseBenchmarkInt(chi.URLParam(req, "teamID")),
			UserID:  parseBenchmarkInt(chi.URLParam(req, "userID")),
			Verbose: req.URL.Query().Get("verbose") == "true",
			Limit:   parseBenchmarkInt(req.URL.Query().Get("limit")),
			TraceID: req.Header.Get("X-Trace-ID"),
			Tenant:  req.Header.Get("X-Tenant-ID"),
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		consumeBenchmarkHeaderQueryInput(input)
		w.Header().Set(HeaderContentType, "application/json")
		_ = json.NewEncoder(w).Encode(benchmarkAPIResponseFrom(benchmarkAPIBindInput{
			TeamID:  input.TeamID,
			UserID:  input.UserID,
			Verbose: input.Verbose,
			Limit:   input.Limit,
			Name:    input.Name,
			Roles:   input.Roles,
		}))
	})
	return r
}

func buildEchoAPIBindHeaderQueryJSONHandler() http.Handler {
	e := echo.New()
	e.POST("/teams/:teamID/users/:userID", func(c *echo.Context) error {
		var input benchmarkHeaderQueryJSONInput
		if err := echo.BindHeaders(c, &input); err != nil {
			return err
		}
		if err := c.Bind(&input); err != nil {
			return err
		}
		consumeBenchmarkHeaderQueryInput(input)
		return c.JSON(http.StatusOK, benchmarkAPIResponse{
			OK:        true,
			TeamID:    input.TeamID,
			UserID:    input.UserID,
			Limit:     input.Limit,
			RoleCount: len(input.Roles),
			Name:      input.Name,
		})
	})
	return e
}

func buildGinAPIBindHeaderQueryJSONHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.POST("/teams/:teamID/users/:userID", func(c *gin.Context) {
		var input benchmarkHeaderQueryJSONInput
		if err := c.ShouldBindUri(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindQuery(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindHeader(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		consumeBenchmarkHeaderQueryInput(input)
		c.JSON(http.StatusOK, benchmarkAPIResponse{
			OK:        true,
			TeamID:    input.TeamID,
			UserID:    input.UserID,
			Limit:     input.Limit,
			RoleCount: len(input.Roles),
			Name:      input.Name,
		})
	})
	return r
}

func buildZincAPIBindInvalidJSONHandler() http.Handler {
	app := New()
	app.Post("/payload", func(c *Context) error {
		var input benchmarkValidationInput
		if err := c.Bind().JSON(&input); err != nil {
			return c.Status(http.StatusBadRequest).NoContent()
		}
		benchmarkSinkBool = false
		return c.NoContent()
	})
	return app
}

func buildChiAPIBindInvalidJSONHandler() http.Handler {
	r := chi.NewRouter()
	r.Post("/payload", func(w http.ResponseWriter, req *http.Request) {
		var input benchmarkValidationInput
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		benchmarkSinkBool = false
		w.WriteHeader(http.StatusNoContent)
	})
	return r
}

func buildEchoAPIBindInvalidJSONHandler() http.Handler {
	e := echo.New()
	e.POST("/payload", func(c *echo.Context) error {
		var input benchmarkValidationInput
		if err := c.Bind(&input); err != nil {
			return c.NoContent(http.StatusBadRequest)
		}
		benchmarkSinkBool = false
		return c.NoContent(http.StatusNoContent)
	})
	return e
}

func buildGinAPIBindInvalidJSONHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.POST("/payload", func(c *gin.Context) {
		var input benchmarkValidationInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		benchmarkSinkBool = false
		c.Status(http.StatusNoContent)
	})
	return r
}

func buildZincAPIBindValidationFailureHandler() http.Handler {
	app := New()
	app.Post("/validate", func(c *Context) error {
		var input benchmarkValidationInput
		if err := c.Bind().JSON(&input); err != nil {
			return c.Status(http.StatusBadRequest).NoContent()
		}
		if !validateBenchmarkInput(input) {
			return c.Status(http.StatusUnprocessableEntity).NoContent()
		}
		return c.NoContent()
	})
	return app
}

func buildChiAPIBindValidationFailureHandler() http.Handler {
	r := chi.NewRouter()
	r.Post("/validate", func(w http.ResponseWriter, req *http.Request) {
		var input benchmarkValidationInput
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !validateBenchmarkInput(input) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return r
}

func buildEchoAPIBindValidationFailureHandler() http.Handler {
	e := echo.New()
	e.POST("/validate", func(c *echo.Context) error {
		var input benchmarkValidationInput
		if err := c.Bind(&input); err != nil {
			return c.NoContent(http.StatusBadRequest)
		}
		if !validateBenchmarkInput(input) {
			return c.NoContent(http.StatusUnprocessableEntity)
		}
		return c.NoContent(http.StatusNoContent)
	})
	return e
}

func buildGinAPIBindValidationFailureHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.POST("/validate", func(c *gin.Context) {
		var input benchmarkValidationInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if !validateBenchmarkInput(input) {
			c.Status(http.StatusUnprocessableEntity)
			return
		}
		c.Status(http.StatusNoContent)
	})
	return r
}

func buildZincAPIBindMultipartHappyPathHandler() http.Handler {
	app := New()
	app.Post("/upload", func(c *Context) error {
		result, err := extractBenchmarkMultipart(c.Request())
		if err != nil {
			return err
		}
		consumeBenchmarkMultipart(result)
		return c.NoContent()
	})
	return app
}

func buildChiAPIBindMultipartHappyPathHandler() http.Handler {
	r := chi.NewRouter()
	r.Post("/upload", func(w http.ResponseWriter, req *http.Request) {
		result, err := extractBenchmarkMultipart(req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		consumeBenchmarkMultipart(result)
		w.WriteHeader(http.StatusNoContent)
	})
	return r
}

func buildEchoAPIBindMultipartHappyPathHandler() http.Handler {
	e := echo.New()
	e.POST("/upload", func(c *echo.Context) error {
		result, err := extractBenchmarkMultipart(c.Request())
		if err != nil {
			return c.NoContent(http.StatusBadRequest)
		}
		consumeBenchmarkMultipart(result)
		return c.NoContent(http.StatusNoContent)
	})
	return e
}

func buildGinAPIBindMultipartHappyPathHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.POST("/upload", func(c *gin.Context) {
		result, err := extractBenchmarkMultipart(c.Request)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		consumeBenchmarkMultipart(result)
		c.Status(http.StatusNoContent)
	})
	return r
}

func buildZincLargeJSONResponseHandler() http.Handler {
	app := New()
	app.Get("/bulk", func(c *Context) error {
		return c.JSON(benchmarkLargeJSONData)
	})
	return app
}

func buildChiLargeJSONResponseHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/bulk", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set(HeaderContentType, "application/json")
		_ = json.NewEncoder(w).Encode(benchmarkLargeJSONData)
	})
	return r
}

func buildEchoLargeJSONResponseHandler() http.Handler {
	e := echo.New()
	e.GET("/bulk", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, benchmarkLargeJSONData)
	})
	return e
}

func buildGinLargeJSONResponseHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/bulk", func(c *gin.Context) {
		c.JSON(http.StatusOK, benchmarkLargeJSONData)
	})
	return r
}

func buildZincLargeJSONBindHandler() http.Handler {
	app := New()
	app.Post("/bulk", func(c *Context) error {
		var input benchmarkLargeJSONEnvelope
		if err := c.Bind().JSON(&input); err != nil {
			return err
		}
		consumeBenchmarkLargeJSONEnvelope(input)
		return c.NoContent()
	})
	return app
}

func buildChiLargeJSONBindHandler() http.Handler {
	r := chi.NewRouter()
	r.Post("/bulk", func(w http.ResponseWriter, req *http.Request) {
		var input benchmarkLargeJSONEnvelope
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		consumeBenchmarkLargeJSONEnvelope(input)
		w.WriteHeader(http.StatusNoContent)
	})
	return r
}

func buildEchoLargeJSONBindHandler() http.Handler {
	e := echo.New()
	e.POST("/bulk", func(c *echo.Context) error {
		var input benchmarkLargeJSONEnvelope
		if err := c.Bind(&input); err != nil {
			return c.NoContent(http.StatusBadRequest)
		}
		consumeBenchmarkLargeJSONEnvelope(input)
		return c.NoContent(http.StatusNoContent)
	})
	return e
}

func buildGinLargeJSONBindHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.POST("/bulk", func(c *gin.Context) {
		var input benchmarkLargeJSONEnvelope
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		consumeBenchmarkLargeJSONEnvelope(input)
		c.Status(http.StatusNoContent)
	})
	return r
}

func buildZincStaticFileHandler() http.Handler {
	app := New()
	mustNoErr(app.Static("/assets", benchmarkStaticRoot))
	return app
}

func buildChiStaticFileHandler() http.Handler {
	r := chi.NewRouter()
	fs := http.StripPrefix("/assets/", http.FileServer(http.Dir(benchmarkStaticRoot)))
	r.Handle("/assets/*", fs)
	return r
}

func buildEchoStaticFileHandler() http.Handler {
	e := echo.New()
	e.Static("/assets", benchmarkStaticRoot)
	return e
}

func buildGinStaticFileHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.Static("/assets", benchmarkStaticRoot)
	return r
}

func buildZincNestedGroupMiddlewareAPIHandler() http.Handler {
	app := New()
	api := app.Group("/api", zincMiddleware("mw1"), zincMiddleware("mw2"))
	v1 := api.Group("/v1", zincMiddleware("mw3"))
	admin := v1.Group("/admin", zincMiddleware("mw4"), zincMiddleware("mw5"))
	admin.Get("/teams/{teamID}/users/{userID}", func(c *Context) error {
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

func buildChiNestedGroupMiddlewareAPIHandler() http.Handler {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Use(chiMiddleware(middlewareKey1))
		r.Use(chiMiddleware(middlewareKey2))
		r.Route("/v1", func(r chi.Router) {
			r.Use(chiMiddleware(middlewareKey3))
			r.Route("/admin", func(r chi.Router) {
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
			})
		})
	})
	return r
}

func buildEchoNestedGroupMiddlewareAPIHandler() http.Handler {
	e := echo.New()
	api := e.Group("/api", echoMiddleware("mw1"), echoMiddleware("mw2"))
	v1 := api.Group("/v1", echoMiddleware("mw3"))
	admin := v1.Group("/admin", echoMiddleware("mw4"), echoMiddleware("mw5"))
	admin.GET("/teams/:teamID/users/:userID", func(c *echo.Context) error {
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

func buildGinNestedGroupMiddlewareAPIHandler() http.Handler {
	r := newGinBenchmarkRouter()
	api := r.Group("/api", ginMiddleware("mw1"), ginMiddleware("mw2"))
	v1 := api.Group("/v1", ginMiddleware("mw3"))
	admin := v1.Group("/admin", ginMiddleware("mw4"), ginMiddleware("mw5"))
	admin.GET("/teams/:teamID/users/:userID", func(c *gin.Context) {
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

func buildZincUnauthorizedRejectHandler() http.Handler {
	app := New()
	app.Use(func(c *Context) error {
		if c.GetHeader("Authorization") == "" {
			benchmarkSinkBool = true
			return c.Status(http.StatusUnauthorized).NoContent()
		}
		return c.Next()
	})
	app.Get("/api/v1/secure/data", func(c *Context) error {
		benchmarkSinkInt++
		return c.NoContent()
	})
	return app
}

func buildChiUnauthorizedRejectHandler() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Header.Get("Authorization") == "" {
				benchmarkSinkBool = true
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Get("/api/v1/secure/data", func(w http.ResponseWriter, req *http.Request) {
		benchmarkSinkInt++
		w.WriteHeader(http.StatusNoContent)
	})
	return r
}

func buildEchoUnauthorizedRejectHandler() http.Handler {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if c.Request().Header.Get("Authorization") == "" {
				benchmarkSinkBool = true
				return c.NoContent(http.StatusUnauthorized)
			}
			return next(c)
		}
	})
	e.GET("/api/v1/secure/data", func(c *echo.Context) error {
		benchmarkSinkInt++
		return c.NoContent(http.StatusNoContent)
	})
	return e
}

func buildGinUnauthorizedRejectHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			benchmarkSinkBool = true
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	r.GET("/api/v1/secure/data", func(c *gin.Context) {
		benchmarkSinkInt++
		c.Status(http.StatusNoContent)
	})
	return r
}

func apiBindHeaderQueryJSONCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincAPIBindHeaderQueryJSONHandler},
		{name: "Chi", build: buildChiAPIBindHeaderQueryJSONHandler},
		{name: "Echo", build: buildEchoAPIBindHeaderQueryJSONHandler},
		{name: "Gin", build: buildGinAPIBindHeaderQueryJSONHandler},
	}
}

func apiBindInvalidJSONCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincAPIBindInvalidJSONHandler},
		{name: "Chi", build: buildChiAPIBindInvalidJSONHandler},
		{name: "Echo", build: buildEchoAPIBindInvalidJSONHandler},
		{name: "Gin", build: buildGinAPIBindInvalidJSONHandler},
	}
}

func apiBindValidationFailureCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincAPIBindValidationFailureHandler},
		{name: "Chi", build: buildChiAPIBindValidationFailureHandler},
		{name: "Echo", build: buildEchoAPIBindValidationFailureHandler},
		{name: "Gin", build: buildGinAPIBindValidationFailureHandler},
	}
}

func apiBindMultipartHappyPathCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincAPIBindMultipartHappyPathHandler},
		{name: "Chi", build: buildChiAPIBindMultipartHappyPathHandler},
		{name: "Echo", build: buildEchoAPIBindMultipartHappyPathHandler},
		{name: "Gin", build: buildGinAPIBindMultipartHappyPathHandler},
	}
}

func largeJSONResponseCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincLargeJSONResponseHandler},
		{name: "Chi", build: buildChiLargeJSONResponseHandler},
		{name: "Echo", build: buildEchoLargeJSONResponseHandler},
		{name: "Gin", build: buildGinLargeJSONResponseHandler},
	}
}

func largeJSONBindCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincLargeJSONBindHandler},
		{name: "Chi", build: buildChiLargeJSONBindHandler},
		{name: "Echo", build: buildEchoLargeJSONBindHandler},
		{name: "Gin", build: buildGinLargeJSONBindHandler},
	}
}

func staticFileCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincStaticFileHandler},
		{name: "Chi", build: buildChiStaticFileHandler},
		{name: "Echo", build: buildEchoStaticFileHandler},
		{name: "Gin", build: buildGinStaticFileHandler},
	}
}

func nestedGroupMiddlewareAPICases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincNestedGroupMiddlewareAPIHandler},
		{name: "Chi", build: buildChiNestedGroupMiddlewareAPIHandler},
		{name: "Echo", build: buildEchoNestedGroupMiddlewareAPIHandler},
		{name: "Gin", build: buildGinNestedGroupMiddlewareAPIHandler},
	}
}

func unauthorizedRejectCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincUnauthorizedRejectHandler},
		{name: "Chi", build: buildChiUnauthorizedRejectHandler},
		{name: "Echo", build: buildEchoUnauthorizedRejectHandler},
		{name: "Gin", build: buildGinUnauthorizedRejectHandler},
	}
}

func BenchmarkAPIBindHeaderQueryJSON(b *testing.B) {
	runPreparedRequestBenchmarks(
		b,
		apiBindHeaderQueryJSONCases(),
		newPreparedBenchmarkRequest(http.MethodPost, benchmarkHeaderQueryTarget(), benchmarkHeaderQueryJSONBody, benchmarkHeaderQueryHeaders()),
	)
}

func BenchmarkAPIBindInvalidJSON(b *testing.B) {
	runPreparedRequestBenchmarks(
		b,
		apiBindInvalidJSONCases(),
		newPreparedBenchmarkRequest(http.MethodPost, "/payload", benchmarkInvalidJSONBody, benchmarkAPIBodyHeaders()),
	)
}

func BenchmarkAPIBindValidationFailure(b *testing.B) {
	runPreparedRequestBenchmarks(
		b,
		apiBindValidationFailureCases(),
		newPreparedBenchmarkRequest(http.MethodPost, "/validate", benchmarkValidationFailureBody, benchmarkAPIBodyHeaders()),
	)
}

func BenchmarkAPIBindMultipartHappyPath(b *testing.B) {
	runPreparedRequestBenchmarks(
		b,
		apiBindMultipartHappyPathCases(),
		newPreparedBenchmarkRequest(http.MethodPost, benchmarkMultipartTarget(), benchmarkMultipartBody, benchmarkMultipartHeaders()),
	)
}

func BenchmarkLargeJSONResponse(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, benchmarkLargeJSONTarget(), largeJSONResponseCases())
}

func BenchmarkLargeJSONBind(b *testing.B) {
	runPreparedRequestBenchmarks(
		b,
		largeJSONBindCases(),
		newPreparedBenchmarkRequest(http.MethodPost, benchmarkLargeJSONTarget(), benchmarkLargeJSONBody, benchmarkAPIBodyHeaders()),
	)
}

func BenchmarkStaticFileHit(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, benchmarkStaticTarget, staticFileCases())
}

func BenchmarkStaticFileNotFound(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/assets/missing.txt", staticFileCases())
}

func BenchmarkNestedGroupMiddlewareAPI(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, "/api/v1/admin/teams/42/users/7?verbose=true&limit=25", nestedGroupMiddlewareAPICases())
}

func BenchmarkAPIUnauthorizedReject(b *testing.B) {
	runServeHTTPBenchmarks(b, http.MethodGet, benchmarkUnauthorizedTarget(), unauthorizedRejectCases())
}

func TestBenchmarkStaticFixtureExists(t *testing.T) {
	if _, err := http.Dir(benchmarkStaticRoot).Open(filepath.Base(benchmarkStaticTarget)); err != nil {
		t.Fatalf("static fixture missing: %v", err)
	}
}
