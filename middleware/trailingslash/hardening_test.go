// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package trailingslash

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestTrailingSlashAddAndPathHelpers(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Add: true}))
	app.Get("/users/", func(c *zinc.Context) error {
		return c.String(c.Path())
	})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "/users/" {
		t.Fatalf("body=%q", rec.Body.String())
	}

	if got := normalizeTrailingSlashPath("", false); got != "/" {
		t.Fatalf("empty path=%q", got)
	}
	if got := normalizeTrailingSlashPath("/", true); got != "/" {
		t.Fatalf("root path=%q", got)
	}
}
