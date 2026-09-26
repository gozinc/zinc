// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestCredentialedOrigin(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		AllowOrigins:     []string{"https://app.example.com"},
		AllowCredentials: true,
	}))
	app.Get("/ok", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set(zinc.HeaderOrigin, "https://app.example.com")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get(zinc.HeaderAccessControlAllowOrigin); got != "https://app.example.com" {
		t.Fatalf("allow-origin=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderAccessControlAllowCredentials); got != "true" {
		t.Fatalf("allow-credentials=%q", got)
	}
}
