// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package cors

import (
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestCORSPartialAndCredentialPolicy(t *testing.T) {
	for _, cfg := range []Config{{}, {AllowCredentials: true}} {
		app := zinc.New()
		app.Use(New(cfg))
		app.Get("/", func(c *zinc.Context) error { return c.Send("ok") })
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		want := "*"
		if cfg.AllowCredentials {
			want = ""
		}
		if w.Header().Get("Access-Control-Allow-Origin") != want {
			t.Fatal(w.Header())
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("wildcard credentials must be rejected")
		}
	}()
	New(Config{AllowOrigins: []string{"*"}, AllowCredentials: true})
}
