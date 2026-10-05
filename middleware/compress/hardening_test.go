// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package compress

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestGzipAcceptQValuesAndExistingVary(t *testing.T) {
	if requestAcceptsGzip("gzip;q=0") {
		t.Fatal("gzip q=0 should not be accepted")
	}
	if !requestAcceptsGzip("br;q=1, gzip;q=0.5") {
		t.Fatal("gzip q=0.5 should be accepted")
	}

	app := zinc.New()
	app.Use(New())
	app.Get("/", func(c *zinc.Context) error {
		c.Vary(zinc.HeaderAcceptEncoding)
		return c.String("hello")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderAcceptEncoding, "gzip;q=1")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if values := rec.Header().Values(zinc.HeaderVary); len(values) != 1 || values[0] != zinc.HeaderAcceptEncoding {
		t.Fatalf("vary=%v", values)
	}
	if got := gunzipResponse(t, rec.Body.Bytes()); got != "hello" {
		t.Fatalf("body=%q", got)
	}
}

func TestGzipHeadSkipsBody(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Head("/head", func(c *zinc.Context) error {
		return c.String("no body")
	})

	req := httptest.NewRequest(http.MethodHead, "/head", nil)
	req.Header.Set(zinc.HeaderAcceptEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "" {
		t.Fatalf("content-encoding=%q", got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

// A gzip body is a different representation from the identity body, so a
// strong ETag must not be shared between them (RFC 9110 section 8.8.3).
func TestGzipWeakensStrongETag(t *testing.T) {
	body := strings.Repeat("x", 100)
	for _, tc := range []struct{ set, want string }{
		{`"x"`, `W/"x"`},
		{`W/"x"`, `W/"x"`},
	} {
		app := zinc.New()
		app.Use(New())
		app.Get("/", func(c *zinc.Context) error {
			c.SetHeader(zinc.HeaderETag, tc.set)
			return c.String(body)
		})

		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, gzipRequest())

		if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "gzip" {
			t.Fatalf("content-encoding=%q", got)
		}
		if got := rec.Header().Get(zinc.HeaderETag); got != tc.want {
			t.Fatalf("etag %s: got %q, want %q", tc.set, got, tc.want)
		}
		if got := gunzipResponse(t, rec.Body.Bytes()); got != body {
			t.Fatalf("body=%q", got)
		}
	}
}

// An uncompressed response keeps its strong ETag.
func TestGzipKeepsETagWhenNotCompressing(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{MinLength: 1024}))
	app.Get("/", func(c *zinc.Context) error {
		c.SetHeader(zinc.HeaderETag, `"x"`)
		return c.String("short")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, gzipRequest())

	if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "" {
		t.Fatalf("content-encoding=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderETag); got != `"x"` {
		t.Fatalf("etag=%q", got)
	}
}

// Content-Range gives offsets in the identity body, so partial responses
// go out uncompressed.
func TestGzipSkipsPartialResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		rng    string
		body   string
	}{
		{"206", http.StatusPartialContent, "bytes 2-4/10", "234"},
		{"416 with Content-Range", http.StatusRequestedRangeNotSatisfiable, "bytes */10", strings.Repeat("u", 100)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := zinc.New()
			app.Use(New())
			app.Get("/", func(c *zinc.Context) error {
				c.SetHeader(zinc.HeaderContentRange, tc.rng)
				return c.Status(tc.status).String(tc.body)
			})

			req := gzipRequest()
			req.Header.Set("Range", "bytes=2-4")
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("status=%d", rec.Code)
			}
			if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "" {
				t.Fatalf("content-encoding=%q", got)
			}
			if got := rec.Header().Get(zinc.HeaderContentRange); got != tc.rng {
				t.Fatalf("content-range=%q", got)
			}
			if rec.Body.String() != tc.body {
				t.Fatalf("body=%q", rec.Body.String())
			}
		})
	}
}
