// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package rewrite

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRewriteUpdatesRequestPath(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Rules: map[string]string{"/old": "/new"}}))
	app.Get("/new", func(c *zinc.Context) error {
		return c.String(c.Path())
	})

	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "/new" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestRewriteWildcardRule(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Rules: map[string]string{"/v1/*": "/api/*"}}))
	app.Get("/api/users", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}
