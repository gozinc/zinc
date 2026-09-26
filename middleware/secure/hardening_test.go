// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package secure

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestSecureDefaultsAndHSTSOptions(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Get("/", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get(zinc.HeaderStrictTransportSecurity); got != "" {
		t.Fatalf("hsts over http=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderXXSSProtection); got != "0" {
		t.Fatalf("xss=%q", got)
	}

	app = zinc.New()
	app.Use(New(Config{
		HSTSMaxAge:            60,
		HSTSExcludeSubdomains: true,
	}))
	app.Get("/", func(c *zinc.Context) error {
		return c.String("ok")
	})
	req = httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if got := rec.Header().Get(zinc.HeaderStrictTransportSecurity); got != "max-age=60" {
		t.Fatalf("hsts=%q", got)
	}
}
