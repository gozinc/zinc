// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package requestid

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRequestIDGeneratesAndPublishesID(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Generate: Static("req-generated"),
	}))
	app.Get("/", func(c *zinc.Context) error {
		if c.Header(zinc.HeaderXRequestID) != "req-generated" {
			t.Fatalf("request header id=%q", c.Header(zinc.HeaderXRequestID))
		}
		return c.String(Get(c))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "req-generated" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderXRequestID); got != "req-generated" {
		t.Fatalf("response request id=%q", got)
	}
}

func TestRequestIDPreservesIncomingID(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Generate: Static("unused"),
	}))
	app.Get("/", func(c *zinc.Context) error {
		return c.String(Get(c))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderXRequestID, "req-incoming")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "req-incoming" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderXRequestID); got != "req-incoming" {
		t.Fatalf("response request id=%q", got)
	}
}

func TestRequestIDGeneratorError(t *testing.T) {
	app := zinc.New()
	generatorErr := errors.New("no entropy")
	app.Use(New(Config{
		Generate: func(*zinc.Context) (string, error) {
			return "", generatorErr
		},
	}))
	app.Get("/", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestGetWithoutMiddleware(t *testing.T) {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error { return c.String(Get(c)) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderXRequestID, "from-header")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "from-header" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if Get(nil) != "" {
		t.Fatal("Get(nil) should be empty")
	}
}
