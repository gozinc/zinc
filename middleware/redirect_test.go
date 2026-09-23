// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRedirectMiddlewareHelpers(t *testing.T) {
	app := zinc.New()
	app.Use(Redirect("/old", "/new", http.StatusTemporaryRedirect))
	req := httptest.NewRequest(http.MethodGet, "/old?x=1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/new?x=1" {
		t.Fatalf("location=%q", got)
	}

	app = zinc.New()
	app.Use(RedirectWithRules(map[string]string{"/docs/*": "/new-docs/*"}))
	req = httptest.NewRequest(http.MethodGet, "/docs/intro", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/new-docs/intro" {
		t.Fatalf("location=%q", got)
	}
}

func TestRedirectSkipperAndFallthrough(t *testing.T) {
	app := zinc.New()
	app.Use(RedirectWithConfig(RedirectConfig{
		Rules:   map[string]string{"/old": "/new"},
		Skipper: func(*zinc.Context) bool { return true },
	}))
	app.Get("/old", func(c *zinc.Context) error {
		return c.String("skipped")
	})
	app.Get("/other", func(c *zinc.Context) error {
		return c.String("next")
	})

	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "skipped" {
		t.Fatalf("skipped body=%q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/other", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "next" {
		t.Fatalf("fallthrough body=%q", rec.Body.String())
	}
}

func mustNoErrRedirect(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
