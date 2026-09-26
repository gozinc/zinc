// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package redirect

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRedirectPreservesQuery(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Rules: map[string]string{"/old": "/new"}, StatusCode: http.StatusPermanentRedirect}))
	app.Get("/new", func(c *zinc.Context) error {
		return c.String("new")
	})

	req := httptest.NewRequest(http.MethodGet, "/old?x=1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/new?x=1" {
		t.Fatalf("location=%q", got)
	}
}
