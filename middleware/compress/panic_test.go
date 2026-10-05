// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
	zrecover "github.com/0mjs/zinc/middleware/recover"
)

// panicApp builds an app whose handler writes partial and then panics with
// value. recoverOutside puts recovery outside compression; otherwise
// compression is outside recovery. The writerCheck middleware runs outermost
// and records whether the compression writer was left installed.
func panicApp(t *testing.T, minLength int, recoverOutside bool, value any, leaked *bool) *zinc.App {
	t.Helper()
	app := zinc.New()
	app.Use(func(c *zinc.Context) error {
		defer func() {
			*leaked = writerIsGzip(c.Writer())
		}()
		return c.Next()
	})
	if recoverOutside {
		app.Use(zrecover.New(zrecover.Config{DisableStack: true}), New(Config{MinLength: minLength}))
	} else {
		app.Use(New(Config{MinLength: minLength}), zrecover.New(zrecover.Config{DisableStack: true}))
	}
	app.Get("/", func(c *zinc.Context) error {
		if _, err := c.Writer().Write([]byte("partial")); err != nil {
			return err
		}
		panic(value)
	})
	return app
}

func writerIsGzip(w http.ResponseWriter) bool {
	for w != nil {
		if _, ok := w.(*gzipResponseWriter); ok {
			return true
		}
		unwrap, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = unwrap.Unwrap()
	}
	return false
}

func gzipRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderAcceptEncoding, "gzip")
	return req
}

// An ordinary panic completes what the handler wrote, in either middleware
// order: a gzip stream gets its trailer and decompresses cleanly, and a body
// still buffered under MinLength is sent as identity. The status the handler
// committed stays.
func TestCompressPanicCompletesResponse(t *testing.T) {
	for _, order := range []struct {
		name           string
		recoverOutside bool
	}{
		{"recover outside compress", true},
		{"compress outside recover", false},
	} {
		t.Run(order.name+"/compressing", func(t *testing.T) {
			var leaked bool
			app := panicApp(t, 0, order.recoverOutside, "boom", &leaked)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, gzipRequest())

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d", rec.Code)
			}
			if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "gzip" {
				t.Fatalf("content-encoding=%q", got)
			}
			if got := gunzipResponse(t, rec.Body.Bytes()); got != "partial" {
				t.Fatalf("body=%q", got)
			}
			if leaked {
				t.Fatal("compression writer left installed after panic")
			}
		})
		t.Run(order.name+"/buffering", func(t *testing.T) {
			var leaked bool
			app := panicApp(t, 1024, order.recoverOutside, "boom", &leaked)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, gzipRequest())

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d", rec.Code)
			}
			if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "" {
				t.Fatalf("content-encoding=%q", got)
			}
			if got := rec.Body.String(); got != "partial" {
				t.Fatalf("body=%q", got)
			}
			if leaked {
				t.Fatal("compression writer left installed after panic")
			}
		})
	}
}

// A panic before any write leaves the response uncommitted, so outer
// recovery writes its 500 uncompressed through the restored writer.
func TestCompressPanicBeforeWriteLeavesErrorToRecovery(t *testing.T) {
	app := zinc.New()
	app.Use(zrecover.New(zrecover.Config{DisableStack: true}), New())
	app.Get("/", func(*zinc.Context) error { panic("boom") })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, gzipRequest())

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "" {
		t.Fatalf("content-encoding=%q", got)
	}
}

// http.ErrAbortHandler propagates unchanged, and compression neither
// completes the gzip stream nor flushes a buffered body.
func TestCompressAbortHandlerPropagates(t *testing.T) {
	for _, tc := range []struct {
		name           string
		minLength      int
		recoverOutside bool
	}{
		{"compressing/recover outside", 0, true},
		{"compressing/compress outside", 0, false},
		{"buffering/recover outside", 1024, true},
		{"buffering/compress outside", 1024, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var leaked bool
			app := panicApp(t, tc.minLength, tc.recoverOutside, http.ErrAbortHandler, &leaked)
			rec := httptest.NewRecorder()

			func() {
				defer func() {
					if value := recover(); value != http.ErrAbortHandler {
						t.Fatalf("recovered %#v, want http.ErrAbortHandler", value)
					}
				}()
				app.ServeHTTP(rec, gzipRequest())
			}()

			if leaked {
				t.Fatal("compression writer left installed after abort")
			}
			if tc.minLength > 0 {
				if rec.Body.Len() != 0 {
					t.Fatalf("buffered body flushed on abort: %q", rec.Body.String())
				}
				return
			}
			reader, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
			if err != nil {
				// No header bytes reached the client yet: also not completed.
				return
			}
			if _, err := io.ReadAll(reader); err == nil {
				t.Fatal("gzip stream completed on abort")
			}
		})
	}
}
