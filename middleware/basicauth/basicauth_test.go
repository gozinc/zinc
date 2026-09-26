// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package basicauth

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc/middleware/internal/testutil"

	"github.com/0mjs/zinc"
)

func TestBasicAuthStaticSuccess(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: Static("joe", "secret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		identity := MustGet(c)
		if identity.Username != "joe" {
			t.Fatalf("username=%q", identity.Username)
		}
		if identity.Source != SourceAuthorizationHeader {
			t.Fatalf("source=%q", identity.Source)
		}
		if got := MustGet(c).Username; got != "joe" {
			t.Fatalf("must username=%q", got)
		}
		return c.String(identity.Username)
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.SetBasicAuth("joe", "secret")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "joe" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBasicAuthMissingCredentials(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: Static("joe", "secret"),
		Realm:     "admin",
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != testutil.JSONErrorBody(http.StatusUnauthorized, "Unauthorized") {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != `Basic realm="admin"` {
		t.Fatalf("challenge=%q", got)
	}
}

func TestBasicAuthMultipleAuthorizationHeaders(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: Static("joe", "secret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String(MustGet(c).Username)
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Add(zinc.HeaderAuthorization, "Bearer abc")
	req.Header.Add(zinc.HeaderAuthorization, "Basic "+base64.StdEncoding.EncodeToString([]byte("joe:secret")))
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "joe" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBasicAuthFromFirst(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Extractor: FromFirst(
			FromHeaderPrefix("X-Admin-Auth", "Basic "),
			FromAuthorizationHeader(),
		),
		Validator: Static("ann", "s3cret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		identity := MustGet(c)
		if identity.Source != SourceHeader {
			t.Fatalf("source=%q", identity.Source)
		}
		return c.String(identity.Username)
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("X-Admin-Auth", "Basic "+base64.StdEncoding.EncodeToString([]byte("ann:s3cret")))
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "ann" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBasicAuthMalformedHeader(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: Static("joe", "secret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Basic !!!")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != `Basic realm="Restricted"` {
		t.Fatalf("challenge=%q", got)
	}
}

func TestBasicAuthSuccessHandler(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: Static("joe", "secret"),
		SuccessHandler: func(c *zinc.Context) error {
			c.Set("authed", MustGet(c).Username)
			return c.Next()
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		value, _ := c.Get("authed")
		return c.String(value.(string))
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.SetBasicAuth("joe", "secret")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "joe" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBasicAuthCustomErrorHandler(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: Static("joe", "secret"),
		ErrorHandler: func(c *zinc.Context, err error) error {
			if !errors.Is(err, ErrCredentialsInvalid) {
				t.Fatalf("error=%v", err)
			}
			return zinc.NewError(http.StatusTeapot, "brew")
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.SetBasicAuth("joe", "wrong")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != testutil.JSONErrorBody(http.StatusTeapot, "brew") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBasicAuthValidatorErrorPassThrough(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: func(*zinc.Context, Credentials) (bool, error) {
			return false, errors.New("lookup failed")
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.SetBasicAuth("joe", "secret")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != "" {
		t.Fatalf("challenge=%q", got)
	}
}
