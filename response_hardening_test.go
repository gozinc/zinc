package zinc_test

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFailedJSONDoesNotCommitSelectedStatus(t *testing.T) {
	for _, status := range []int{200, 201, 422} {
		app := zinc.New()
		app.Get("/", func(c *zinc.Context) error { return c.Status(status).JSON(make(chan int)) })
		w := hardeningRequest(app, "GET", "/")
		if w.Code != 500 || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("selected=%d got=%d headers=%v", status, w.Code, w.Header())
		}
	}
}

type failingStream struct{}

func (failingStream) Read([]byte) (int, error) { return 0, errors.New("stream failed before output") }

func TestFailedStreamLeavesErrorResponseAvailable(t *testing.T) {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error { return c.Status(201).Stream("application/octet-stream", failingStream{}) })
	if w := hardeningRequest(app, "GET", "/"); w.Code != 500 {
		t.Fatalf("status %d", w.Code)
	}
}

func TestEmptyStreamCommitsSelectedStatus(t *testing.T) {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error { return c.Status(201).Stream("text/plain", strings.NewReader("")) })
	if w := hardeningRequest(app, "GET", "/"); w.Code != 201 {
		t.Fatalf("status %d", w.Code)
	}
}

func TestGzipFlushKeepsIdentityForLaterWrites(t *testing.T) {
	app := zinc.New()
	app.Use(middleware.GzipWithConfig(middleware.GzipConfig{MinLength: 32}))
	app.Get("/", func(c *zinc.Context) error {
		_, _ = io.WriteString(c.Writer(), "first")
		if err := http.NewResponseController(c.Writer()).Flush(); err != nil {
			return err
		}
		_, err := io.WriteString(c.Writer(), strings.Repeat("x", 128))
		return err
	})
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "" || w.Body.String() != "first"+strings.Repeat("x", 128) {
		t.Fatalf("mixed representation: %v %q", w.Header(), w.Body.String())
	}
	identity := hardeningRequest(app, "GET", "/")
	if !strings.Contains(identity.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatal("identity response lacks Vary")
	}
}

func TestRecoverPreservesAbortHandlerAndRestoresWriter(t *testing.T) {
	app := zinc.New()
	app.Use(middleware.Recover(), middleware.Gzip())
	app.Get("/", func(c *zinc.Context) error { panic(http.ErrAbortHandler) })
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	func() {
		defer func() {
			if value := recover(); value != http.ErrAbortHandler {
				t.Errorf("abort changed: %v", value)
			}
		}()
		app.ServeHTTP(httptest.NewRecorder(), r)
	}()
	app.Get("/ok", func(c *zinc.Context) error { return c.String("ok") })
	if w := hardeningRequest(app, "GET", "/ok"); w.Body.String() != "ok" {
		t.Fatalf("pooled state after abort: %q", w.Body.String())
	}
}

func TestMiddlewareDoesNotInventWriterCapabilities(t *testing.T) {
	app := zinc.New()
	app.Use(middleware.Gzip())
	app.Get("/", func(c *zinc.Context) error {
		if _, ok := c.Writer().(http.Flusher); ok {
			t.Error("invented flushing")
		}
		if _, ok := c.Writer().(http.Hijacker); ok {
			t.Error("invented hijacking")
		}
		if _, ok := c.Writer().(http.Pusher); ok {
			t.Error("invented HTTP/2 push")
		}
		return c.String("ok")
	})
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	// Deliberately hide the recorder's Flusher capability.
	app.ServeHTTP(struct{ http.ResponseWriter }{httptest.NewRecorder()}, r)
}
func TestFailedResponseDoesNotBecomeSuccess(t *testing.T) {
	for _, kind := range []string{"JSON", "FileFS"} {
		t.Run(kind, func(t *testing.T) {
			app := zinc.New()
			app.Get("/", func(c *zinc.Context) error {
				if kind == "JSON" {
					return c.JSON(make(chan int))
				}
				return c.FileFS("missing", fstest.MapFS{})
			})
			w := hardeningRequest(app, "GET", "/")
			if w.Code < 400 {
				t.Fatalf("failed %s produced %d %q", kind, w.Code, w.Body.String())
			}
		})
	}
}

func TestRawWriterCommitIsRespected(t *testing.T) {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error {
		_, _ = io.WriteString(c.Writer(), "partial")
		return errors.New("internal")
	})
	w := hardeningRequest(app, "GET", "/")
	if w.Body.String() != "partial" {
		t.Fatalf("error handler appended to committed response: %q", w.Body.String())
	}
}

func TestMiddlewareHandlesErrorOnce(t *testing.T) {
	calls := 0
	cfg := zinc.DefaultConfig
	cfg.ErrorHandler = func(c *zinc.Context, err error) { calls++; _ = c.Status(500).String("failure") }
	app := zinc.NewWithConfig(cfg)
	app.Use(middleware.Prometheus(middleware.NewPrometheusMetrics()), middleware.Gzip())
	app.Get("/", func(c *zinc.Context) error { return errors.New("internal") })
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	app.ServeHTTP(httptest.NewRecorder(), r)
	if calls != 1 {
		t.Fatalf("configured error handler invoked %d times", calls)
	}
}

func TestGzipMinLengthFlushSendsPendingBytes(t *testing.T) {
	app := zinc.New()
	app.Use(middleware.GzipWithConfig(middleware.GzipConfig{MinLength: 1024}))
	w := httptest.NewRecorder()
	app.Get("/", func(c *zinc.Context) error {
		_, _ = io.WriteString(c.Writer(), "hello")
		if err := http.NewResponseController(c.Writer()).Flush(); err != nil {
			return err
		}
		if w.Body.Len() == 0 {
			t.Error("Flush did not send buffered bytes")
		}
		return nil
	})
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	app.ServeHTTP(w, r)
}

func TestInformationalHeadersDoNotCommitFinalStatus(t *testing.T) {
	w := &headerRecorder{header: make(http.Header)}
	rw := zinc.WrapResponseWriter(w)
	rw.WriteHeader(103)
	rw.WriteHeader(201)
	if fmt.Sprint(w.codes) != "[103 201]" || rw.Status() != 201 {
		t.Fatalf("codes=%v final=%d", w.codes, rw.Status())
	}
}

func TestAcceptSpecificExclusionOverridesWildcard(t *testing.T) {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error { return c.String(c.Accepts("text/html")) })
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "text/html;q=0, */*;q=1")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Body.String() != "" {
		t.Fatalf("explicitly excluded type accepted: %q", w.Body.String())
	}
}

func TestResponseHeadersAreRequestOwned(t *testing.T) {
	app := zinc.New()
	firstRequest := true
	var shared []string
	var original string
	app.Use(func(c *zinc.Context) error {
		err := c.Next()
		if firstRequest {
			firstRequest = false
			shared = c.Writer().Header()["Content-Type"]
			original = shared[0]
			shared[0] = "application/x-audit"
		}
		return err
	})
	defer func() {
		if shared != nil {
			shared[0] = original
		}
	}()
	app.Get("/", func(c *zinc.Context) error { return c.String("ok") })
	hardeningRequest(app, "GET", "/")
	second := hardeningRequest(app, "GET", "/")
	if second.Header().Get("Content-Type") != original {
		t.Fatalf("response headers share mutable backing storage: %q", second.Header().Get("Content-Type"))
	}
}

func TestDefaultMethodMismatchHeadersAreIndependent(t *testing.T) {
	app := zinc.New()
	app.Get("/resource", func(c *zinc.Context) error { return c.String("ok") })
	first := hardeningRequest(app, http.MethodPost, "/resource")
	if first.Code != http.StatusMethodNotAllowed {
		t.Fatalf("first status = %d", first.Code)
	}
	allow := first.Header()["Allow"]
	contentType := first.Header()["Content-Type"]
	if len(allow) == 0 || len(contentType) == 0 {
		t.Fatalf("missing headers: %#v", first.Header())
	}
	originalAllow, originalContentType := allow[0], contentType[0]
	allow[0] = "tampered"
	allow = append(allow, "another-method")
	if contentType[0] != originalContentType {
		t.Fatalf("Allow mutation changed Content-Type: %q", contentType[0])
	}
	second := hardeningRequest(app, http.MethodPost, "/resource")
	if second.Header().Get("Allow") != originalAllow || second.Header().Get("Content-Type") != originalContentType {
		t.Fatalf("headers leaked into next response: %#v", second.Header())
	}
}

type headerRecorder struct {
	header http.Header
	codes  []int
	bytes.Buffer
}

func (w *headerRecorder) Header() http.Header  { return w.header }
func (w *headerRecorder) WriteHeader(code int) { w.codes = append(w.codes, code) }
