// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package pprof

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestPprof(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Prefix: "/debug/pprof"}))
	app.Get("/other", func(c *zinc.Context) error {
		return c.String("next")
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("pprof status=%d body len=%d", rec.Code, rec.Body.Len())
	}

	req = httptest.NewRequest(http.MethodGet, "/other", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "next" {
		t.Fatalf("fallthrough body=%q", rec.Body.String())
	}
}

func TestPprofDefaultAndPrefixNormalization(t *testing.T) {
	app := zinc.New()
	app.Use(New())

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("default cmdline status=%d", rec.Code)
	}

	app = zinc.New()
	app.Use(New(Config{Prefix: "custom/pprof/"}))
	app.Get("/next", func(c *zinc.Context) error {
		return c.String("next")
	})

	req = httptest.NewRequest(http.MethodGet, "/custom/pprof/symbol", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("symbol status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/custom/pprof/heap", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("heap status=%d body len=%d", rec.Code, rec.Body.Len())
	}

	req = httptest.NewRequest(http.MethodGet, "/next", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "next" {
		t.Fatalf("fallthrough body=%q", rec.Body.String())
	}
}
