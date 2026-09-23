// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestKeyAuthBearerHeader(t *testing.T) {
	app := zinc.New()
	app.Use(KeyAuth(KeyAuthStatic("secret")))
	app.Get("/private", func(c *zinc.Context) error {
		state := MustKeyAuthCurrent(c)
		return c.String(string(state.Source))
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer secret")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(KeyAuthSourceAuthorizationHeader) {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestKeyAuthQueryExtractor(t *testing.T) {
	app := zinc.New()
	app.Use(KeyAuthWithConfig(KeyAuthConfig{
		Extractor: KeyAuthFromQuery("api_key"),
		Validator: KeyAuthStatic("secret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private?api_key=secret", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestKeyAuthRejectsMissingKey(t *testing.T) {
	app := zinc.New()
	app.Use(KeyAuth(KeyAuthStatic("secret")))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != "Bearer" {
		t.Fatalf("www-authenticate=%q", got)
	}
}

func mustNoErrKeyAuth(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
