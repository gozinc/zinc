// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRequestIDDefaultGeneratorAndFallbackValue(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Get("/", func(c *zinc.Context) error {
		id := Get(c)
		if len(id) != 32 {
			t.Fatalf("request id length=%d id=%q", len(id), id)
		}
		return c.String(id)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderXRequestID); got != rec.Body.String() {
		t.Fatalf("header=%q body=%q", got, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderXRequestID, "fallback")
	app = zinc.New()
	app.Get("/", func(c *zinc.Context) error {
		if got := Get(c); got != "fallback" {
			t.Fatalf("fallback request id=%q", got)
		}
		return c.String("ok")
	})
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
}
