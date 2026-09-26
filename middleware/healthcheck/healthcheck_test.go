// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package healthcheck

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestHealthcheck(t *testing.T) {
	healthy := true
	app := zinc.New()
	app.Use(New(Config{Check: func(*zinc.Context) error {
		if healthy {
			return nil
		}
		return errors.New("database unreachable")
	}}))
	app.Get("/healthz", func(c *zinc.Context) error { return c.String("handler") })
	app.Post("/healthz", func(c *zinc.Context) error { return c.String("handler") })

	serve := func(method string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(method, DefaultPath, nil))
		return rec
	}
	if rec := serve(http.MethodGet); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("healthy: %d %q", rec.Code, rec.Body.String())
	}
	if rec := serve(http.MethodPost); rec.Body.String() != "handler" {
		t.Fatalf("POST reached %q, want the handler", rec.Body.String())
	}
	healthy = false
	if rec := serve(http.MethodGet); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy: %d %q", rec.Code, rec.Body.String())
	}
}

func TestHealthcheckPath(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Path: "/ping"}))
	app.Get("/healthz", func(c *zinc.Context) error { return c.String("handler") })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/ping", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("/ping: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Body.String() != "handler" {
		t.Fatalf("/healthz: %q", rec.Body.String())
	}
}
