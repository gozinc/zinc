// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package secure

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestSecureSetsDefaultHeaders(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'",
	}))
	app.Get("/", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get(zinc.HeaderXContentTypeOptions); got != "nosniff" {
		t.Fatalf("x-content-type-options=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderXFrameOptions); got != "SAMEORIGIN" {
		t.Fatalf("x-frame-options=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderStrictTransportSecurity); got != "max-age=31536000; includeSubDomains" {
		t.Fatalf("hsts=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderContentSecurityPolicy); got != "default-src 'self'" {
		t.Fatalf("csp=%q", got)
	}
}
