// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	. "github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/cors"
	"github.com/0mjs/zinc/middleware/logger"
	zrecover "github.com/0mjs/zinc/middleware/recover"
	"github.com/0mjs/zinc/middleware/requestid"
	gincors "github.com/gin-contrib/cors"
	ginrequestid "github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/labstack/echo/v5"
	echomw "github.com/labstack/echo/v5/middleware"
)

// APIProductionStack is an API route behind the middleware a production app
// runs in front of every request: a request ID, an access log line, panic
// recovery and CORS, in that order. Each framework uses its own idiomatic
// middleware for each job (Gin's request ID and CORS come from gin-contrib,
// its official add-ons), writing its default log line to the same writer.

const stackOrigin = "https://app.example.com"

// stackLog counts the log lines every framework writes, and discards them.
type stackLog struct{ lines atomic.Int64 }

func (l *stackLog) Write(p []byte) (int, error) {
	l.lines.Add(int64(bytes.Count(p, []byte{'\n'})))
	return len(p), nil
}

var stackLogSink = &stackLog{}

func stackCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincStackHandler},
		{name: "Echo", build: buildEchoStackHandler},
		{name: "Gin", build: buildGinStackHandler},
	}
}

func buildZincStackHandler() http.Handler {
	app := New()
	app.Use(
		requestid.New(),
		logger.New(logger.Config{Logger: slog.New(slog.NewTextHandler(stackLogSink, nil))}),
		zrecover.New(),
		cors.New(cors.Config{AllowOrigins: []string{stackOrigin}}),
	)
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

func buildEchoStackHandler() http.Handler {
	e := echo.NewWithConfig(echo.Config{Logger: slog.New(slog.NewTextHandler(stackLogSink, nil))})
	e.Use(
		echomw.RequestID(),
		echomw.RequestLogger(),
		echomw.Recover(),
		echomw.CORS(stackOrigin),
	)
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

func buildGinStackHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.Use(
		ginrequestid.New(),
		gin.LoggerWithWriter(stackLogSink),
		gin.Recovery(),
		gincors.New(gincors.Config{AllowOrigins: []string{stackOrigin}, AllowMethods: []string{http.MethodGet}}),
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
		c.JSON(http.StatusOK, benchmarkAPIResponseFrom(input))
	})
	return r
}

func stackRequests() []*http.Request {
	reqs := poolRequests(http.MethodGet, poolSize, 60, func(i int) string {
		return poolTeamUser(i) + "?verbose=true&limit=25"
	})
	for _, r := range reqs {
		r.Header.Set("Origin", stackOrigin)
	}
	return reqs
}

// proveStack checks what proveCases can't see: every framework set a request
// ID, answered CORS for the request's origin, and wrote one log line.
func proveStack(t testing.TB, name string, handler http.Handler, req *http.Request) {
	t.Helper()
	before := stackLogSink.lines.Load()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status %d", name, rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatalf("%s: no X-Request-Id on the response", name)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != stackOrigin {
		t.Fatalf("%s: Access-Control-Allow-Origin %q, want %q", name, got, stackOrigin)
	}
	if got := stackLogSink.lines.Load() - before; got != 1 {
		t.Fatalf("%s: wrote %d log lines, want 1", name, got)
	}
}

func BenchmarkAPIProductionStack(b *testing.B) {
	requests := stackRequests()
	for _, bc := range stackCases() {
		for _, r := range requests[:proofRequests] {
			proveStack(b, bc.name, bc.build(), r)
		}
	}
	runServeHTTPRequestSetBenchmarks(b, stackCases(), requests)
}

// TestAPIProductionStackProof runs the scenario's proofs without timing, so
// go test catches a framework that skips a middleware's work.
func TestAPIProductionStackProof(t *testing.T) {
	requests := stackRequests()
	for _, bc := range stackCases() {
		for _, r := range requests[:proofRequests] {
			proveStack(t, bc.name, bc.build(), r)
		}
	}
}
