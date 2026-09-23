// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
	jwtgo "github.com/golang-jwt/jwt/v5"
)

func TestMiddlewareSurfaceBasicAuth(t *testing.T) {
	app := zinc.New()
	app.Use(BasicAuth(BasicAuthStatic("joe", "secret")))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String(MustBasicAuthUsername(c))
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

func TestMiddlewareSurfaceCORSWithOptions(t *testing.T) {
	app := zinc.New()
	app.Use(CORSWithOptions(
		CORSAllowOrigins("https://app.example.com"),
		CORSAllowCredentials(true),
	))
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

func TestMiddlewareSurfaceBodyDumpAndBodyLimit(t *testing.T) {
	app := zinc.New()

	var observed BodyDumpSnapshot
	app.Use(BodyDump(func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
		observed = snapshot
	}))
	app.Use(BodyLimit(4))
	app.Post("/echo", func(c *zinc.Context) error {
		body, err := c.BodyString()
		if err != nil {
			return err
		}
		return c.String(body)
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("ping"))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if string(observed.RequestBody) != "ping" {
		t.Fatalf("request body=%q", string(observed.RequestBody))
	}

	req = httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("too-large"))
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddlewareSurfaceContextTimeoutAndCSRF(t *testing.T) {
	app := zinc.New()
	app.Use(ContextTimeout(100 * time.Millisecond))
	app.Use(CSRF())
	app.Get("/form", func(c *zinc.Context) error {
		if MustContextTimeoutCurrent(c).Timeout != 100*time.Millisecond {
			t.Fatalf("timeout=%s", MustContextTimeoutCurrent(c).Timeout)
		}
		return c.String(MustCSRFToken(c))
	})

	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() == "" {
		t.Fatal("csrf token should be present")
	}
	if got := rec.Header().Get(zinc.HeaderSetCookie); !strings.Contains(got, "_csrf=") {
		t.Fatalf("set-cookie=%q", got)
	}
}

func TestMiddlewareSurfaceJWT(t *testing.T) {
	tokenString := signSurfaceJWT(t, []byte("secret"))

	app := zinc.New()
	app.Use(JWT(func(*zinc.Context, *jwtgo.Token) (any, error) {
		return []byte("secret"), nil
	}))
	app.Get("/private", func(c *zinc.Context) error {
		claims := MustJWTClaims[jwtgo.MapClaims](c)
		return c.String(claims["sub"].(string))
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "joe" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func signSurfaceJWT(t *testing.T, key []byte) string {
	t.Helper()

	token := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"sub": "joe",
	})
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func mustNoErrSurface(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
