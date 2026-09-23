// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRewriteUpdatesRequestPath(t *testing.T) {
	app := zinc.New()
	app.Use(Rewrite("/old", "/new"))
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
	app.Use(Rewrite("/v1/*", "/api/*"))
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

func TestRedirectPreservesQuery(t *testing.T) {
	app := zinc.New()
	app.Use(Redirect("/old", "/new", http.StatusPermanentRedirect))
	app.Get("/new", func(c *zinc.Context) error {
		return c.String("new")
	})

	req := httptest.NewRequest(http.MethodGet, "/old?x=1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/new?x=1" {
		t.Fatalf("location=%q", got)
	}
}

func TestTrailingSlashRemovesSlashBeforeRouting(t *testing.T) {
	app := zinc.New()
	app.Use(TrailingSlash())
	app.Get("/users", func(c *zinc.Context) error {
		return c.String(c.Path())
	})

	req := httptest.NewRequest(http.MethodGet, "/users/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "/users" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestTrailingSlashCanRedirect(t *testing.T) {
	app := zinc.New()
	app.Use(TrailingSlashWithConfig(TrailingSlashConfig{
		Redirect:   true,
		StatusCode: http.StatusTemporaryRedirect,
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/?page=1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/users?page=1" {
		t.Fatalf("location=%q", got)
	}
}

func TestMethodOverrideFromHeader(t *testing.T) {
	app := zinc.New()
	app.Use(MethodOverride())
	app.Put("/resource", func(c *zinc.Context) error {
		return c.String(c.Method() + ":" + c.GetHeader(HeaderXOriginalMethod))
	})

	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	req.Header.Set(HeaderXHTTPMethodOverride, http.MethodPut)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "PUT:POST" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestMethodOverrideRejectsUnknownMethod(t *testing.T) {
	app := zinc.New()
	app.Use(MethodOverride())
	app.Post("/resource", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	req.Header.Set(HeaderXHTTPMethodOverride, http.MethodGet)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSecureSetsDefaultHeaders(t *testing.T) {
	app := zinc.New()
	app.Use(SecureWithConfig(SecureConfig{
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

func mustNoErrRoutingHelpers(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
