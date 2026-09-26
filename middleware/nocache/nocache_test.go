// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package nocache

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestNoCache(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Get("/", func(c *zinc.Context) error { return c.String("ok") })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for header, want := range map[string]string{
		zinc.HeaderCacheControl: "no-cache, no-store, max-age=0, must-revalidate",
		zinc.HeaderPragma:       "no-cache",
		zinc.HeaderExpires:      "0",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s=%q, want %q", header, got, want)
		}
	}
}
