// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package trailingslash

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestTrailingSlashRemovesSlashBeforeRouting(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Get("/users", func(c *zinc.Context) error {
		return c.String(c.Path())
	})

	req := httptest.NewRequest(http.MethodGet, "/users/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "/users" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestTrailingSlashCanRedirect(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Redirect:   true,
		StatusCode: http.StatusTemporaryRedirect,
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/?page=1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/users?page=1" {
		t.Fatalf("location=%q", got)
	}
}
