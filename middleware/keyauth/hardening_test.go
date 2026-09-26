// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package keyauth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestKeyAuthExtractorsAndFirstFallback(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Extractor: FromFirst(
			FromHeader("X-Missing"),
			FromHeaderPrefix("X-API-Key", "Token "),
			FromCookie("api_key"),
		),
		Validator: StaticKeys("secret", "backup"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		state := MustGet(c)
		return c.String(state.Key + ":" + string(state.Source))
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("X-API-Key", "Token backup")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "backup:"+string(SourceHeader) {
		t.Fatalf("body=%q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: "api_key", Value: "secret"})
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "secret:"+string(SourceCookie) {
		t.Fatalf("body=%q", rec.Body.String())
	}

}

func TestKeyAuthCustomErrorHandlerCanReturnHTTPError(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Validator: Static("secret"),
		ErrorHandler: func(_ *zinc.Context, err error) error {
			if errors.Is(err, ErrKeyMissing) {
				return zinc.ErrForbidden
			}
			return err
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}
