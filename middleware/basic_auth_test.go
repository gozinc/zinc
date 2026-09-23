// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestBasicAuthStaticSuccess(t *testing.T) {
	app := zinc.New()
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Validator: BasicAuthStatic("joe", "secret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		identity := MustBasicAuthCurrent(c)
		if identity.Username != "joe" {
			t.Fatalf("username=%q", identity.Username)
		}
		if identity.Source != BasicAuthSourceAuthorizationHeader {
			t.Fatalf("source=%q", identity.Source)
		}
		if got := MustBasicAuthUsername(c); got != "joe" {
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
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Validator: BasicAuthStatic("joe", "secret"),
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
	if rec.Body.String() != http.StatusText(http.StatusUnauthorized) {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != `Basic realm="admin"` {
		t.Fatalf("challenge=%q", got)
	}
}

func TestBasicAuthMultipleAuthorizationHeaders(t *testing.T) {
	app := zinc.New()
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Validator: BasicAuthStatic("joe", "secret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String(MustBasicAuthUsername(c))
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
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Extractor: BasicAuthFromFirst(
			BasicAuthFromHeaderPrefix("X-Admin-Auth", "Basic "),
			BasicAuthFromAuthorizationHeader(),
		),
		Validator: BasicAuthStatic("ann", "s3cret"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		identity := MustBasicAuthCurrent(c)
		if identity.Source != BasicAuthSourceHeader {
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
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Validator: BasicAuthStatic("joe", "secret"),
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
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Validator: BasicAuthStatic("joe", "secret"),
		SuccessHandler: func(c *zinc.Context) error {
			c.Set("authed", MustBasicAuthUsername(c))
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
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Validator: BasicAuthStatic("joe", "secret"),
		ErrorHandler: func(c *zinc.Context, err error) error {
			if !errors.Is(err, ErrBasicAuthCredentialsInvalid) {
				t.Fatalf("error=%v", err)
			}
			return zinc.NewError(http.StatusTeapot).WithMessage("brew")
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
	if rec.Body.String() != "brew" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBasicAuthValidatorErrorPassThrough(t *testing.T) {
	app := zinc.New()
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Validator: func(*zinc.Context, BasicAuthCredentials) (bool, error) {
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

func TestBasicAuthSkipper(t *testing.T) {
	app := zinc.New()
	app.Use(BasicAuthWithConfig(BasicAuthConfig{
		Skipper: func(*zinc.Context) bool { return true },
		Validator: BasicAuthStaticPairs(
			BasicAuthPair{Username: "joe", Password: "secret"},
			BasicAuthPair{Username: "ann", Password: "s3cret"},
		),
	}))
	app.Get("/public", func(c *zinc.Context) error {
		if _, ok := BasicAuthCurrent(c); ok {
			t.Fatal("identity should not be present")
		}
		return c.String("public")
	})

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "public" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func mustNoErrBasicAuth(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
