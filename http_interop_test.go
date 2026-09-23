// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHandleHTTPPreservesStandardContracts(t *testing.T) {
	app := New()
	request := httptest.NewRequest(http.MethodGet, "/native/42", nil)
	writer := newHTTPInteropWriter()
	hijackErr := errors.New("hijack reached underlying writer")
	writer.hijackErr = hijackErr
	app.UseHTTP(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r != request || w != writer {
				t.Fatal("standard middleware did not receive the original HTTP values")
			}
			next.ServeHTTP(w, r)
		})
	})

	app.HandleHTTP("GET /native/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r != request {
			t.Fatal("standard handler did not receive the original request")
		}
		if w != writer {
			t.Fatal("standard handler did not receive the original response writer")
		}
		if got := r.PathValue("id"); got != "42" {
			t.Fatalf("path value=%q", got)
		}

		w.(http.Flusher).Flush()
		if _, _, err := w.(http.Hijacker).Hijack(); !errors.Is(err, hijackErr) {
			t.Fatalf("hijack err=%v", err)
		}
		if _, err := w.(io.ReaderFrom).ReadFrom(bytes.NewBufferString("native")); err != nil {
			t.Fatalf("read from: %v", err)
		}
		if err := w.(http.Pusher).Push("/asset.js", nil); err != nil {
			t.Fatalf("push: %v", err)
		}
		if got := w.(interface{ Unwrap() http.ResponseWriter }).Unwrap(); got != writer {
			t.Fatal("unwrap did not reach the original writer")
		}
	}))

	app.ServeHTTP(writer, request)

	if !writer.flushed || !writer.readFrom || writer.pushed != "/asset.js" {
		t.Fatalf("interfaces flush=%v readFrom=%v pushed=%q", writer.flushed, writer.readFrom, writer.pushed)
	}
	if got := writer.body.String(); got != "native" {
		t.Fatalf("body=%q", got)
	}
}

func TestUseHTTPOrderingAndGroupHandleHTTP(t *testing.T) {
	app := New()
	var order []string

	standardMiddleware := func(name string) HTTPMiddleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+":before")
				next.ServeHTTP(w, r)
				order = append(order, name+":after")
			})
		}
	}
	app.UseHTTP(standardMiddleware("http-1"), standardMiddleware("http-2"))
	app.Use(func(c *Context) error {
		order = append(order, "zinc-global")
		return c.Next()
	})

	api := app.Group("/api", func(c *Context) error {
		order = append(order, "zinc-group")
		return c.Next()
	})
	api.HandleHTTP("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler:"+r.PathValue("id"))
		_, _ = io.WriteString(w, "ok")
	}))

	resp := performRequest(t, app, http.MethodGet, "/api/users/7", nil, nil)
	if resp.Body.String() != "ok" {
		t.Fatalf("body=%q", resp.Body.String())
	}
	want := []string{
		"http-1:before",
		"http-2:before",
		"zinc-global",
		"zinc-group",
		"handler:7",
		"http-2:after",
		"http-1:after",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order=%v want=%v", order, want)
	}

	route, ok := app.FindRoute(http.MethodGet, "/api/users/7")
	if !ok || route.Path != "/api/users/{id}" {
		t.Fatalf("route=%+v ok=%v", route, ok)
	}
}

func TestUseHTTPInitializesEachMiddlewareOnceAcrossCalls(t *testing.T) {
	app := New()
	var initialized []string
	var order []string

	middleware := func(name string) HTTPMiddleware {
		return func(next http.Handler) http.Handler {
			initialized = append(initialized, name)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+":before")
				next.ServeHTTP(w, r)
				order = append(order, name+":after")
			})
		}
	}

	app.UseHTTP(middleware("first"))
	app.UseHTTP(middleware("second"))
	app.Get("/", func(c *Context) error {
		order = append(order, "handler")
		return c.String("ok")
	})

	if !reflect.DeepEqual(initialized, []string{"first", "second"}) {
		t.Fatalf("initialized=%v", initialized)
	}
	performRequest(t, app, MethodGet, "/", nil, nil)
	want := []string{"first:before", "second:before", "handler", "second:after", "first:after"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order=%v want=%v", order, want)
	}
}

func TestHandleHTTPValidation(t *testing.T) {
	app := New()
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	for _, pattern := range []string{"", "GET", "/users", "GET users", "GET /users extra"} {
		pattern := pattern
		mustPanic(t, "invalid HTTP route pattern", func() { app.HandleHTTP(pattern, handler) })
	}
	mustPanic(t, "HTTP handler is nil", func() { app.HandleHTTP("GET /users", nil) })

	group := app.Group("/api")
	mustPanic(t, "invalid HTTP route pattern", func() { group.HandleHTTP("GET users", handler) })
	mustPanic(t, "HTTP handler is nil", func() { group.HandleHTTP("GET /users", nil) })
}

func TestUseHTTPRejectsInvalidMiddleware(t *testing.T) {
	t.Run("nil middleware", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		New().UseHTTP(nil)
	})

	t.Run("nil wrapped handler", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		New().UseHTTP(func(http.Handler) http.Handler { return nil })
	})
}

type httpInteropWriter struct {
	header    http.Header
	body      bytes.Buffer
	status    int
	flushed   bool
	readFrom  bool
	pushed    string
	hijackErr error
}

func newHTTPInteropWriter() *httpInteropWriter {
	return &httpInteropWriter{header: make(http.Header)}
}

func (w *httpInteropWriter) Header() http.Header { return w.header }

func (w *httpInteropWriter) WriteHeader(code int) { w.status = code }

func (w *httpInteropWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func (w *httpInteropWriter) Flush() { w.flushed = true }

func (w *httpInteropWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, w.hijackErr
}

func (w *httpInteropWriter) ReadFrom(r io.Reader) (int64, error) {
	w.readFrom = true
	return w.body.ReadFrom(r)
}

func (w *httpInteropWriter) Push(target string, _ *http.PushOptions) error {
	w.pushed = target
	return nil
}

func (w *httpInteropWriter) Unwrap() http.ResponseWriter { return w }
