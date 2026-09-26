// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package compress

import (
	"net/http"
	"net/http/httptest"
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
