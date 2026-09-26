// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package methodoverride

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestMethodOverrideFromHeader(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Put("/resource", func(c *zinc.Context) error {
		return c.String(c.Method() + ":" + c.Header(HeaderXOriginalMethod))
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
	app.Use(New())
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
