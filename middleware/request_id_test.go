// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRequestIDGeneratesAndPublishesID(t *testing.T) {
	app := zinc.New()
	app.Use(RequestIDWithConfig(RequestIDConfig{
		Generator: StaticRequestID("req-generated"),
	}))
	app.Get("/", func(c *zinc.Context) error {
		state := MustRequestIDCurrent(c)
		if state.ID != "req-generated" {
			t.Fatalf("state id=%q", state.ID)
		}
		if !state.Generated {
			t.Fatal("state should mark generated id")
		}
		if c.RequestID() != "req-generated" {
			t.Fatalf("context request id=%q", c.RequestID())
		}
		return c.String(RequestIDValue(c))
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
	app.Use(RequestIDWithConfig(RequestIDConfig{
		Generator: StaticRequestID("unused"),
	}))
	app.Get("/", func(c *zinc.Context) error {
		state := MustRequestIDCurrent(c)
		if state.Generated {
			t.Fatal("state should not mark incoming id as generated")
		}
		return c.String(state.ID)
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
	app.Use(RequestIDWithConfig(RequestIDConfig{
		Generator: func(*zinc.Context) (string, error) {
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

func TestRequestIDSkipper(t *testing.T) {
	app := zinc.New()
	app.Use(RequestIDWithConfig(RequestIDConfig{
		Skipper:   func(*zinc.Context) bool { return true },
		Generator: StaticRequestID("req-generated"),
	}))
	app.Get("/", func(c *zinc.Context) error {
		if _, ok := RequestIDCurrent(c); ok {
			t.Fatal("request id state should not be set")
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "ok" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderXRequestID); got != "" {
		t.Fatalf("response request id=%q", got)
	}
}

func mustNoErrRequestID(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
