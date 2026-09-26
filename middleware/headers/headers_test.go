// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package headers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestHeaders(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Set: map[string]string{"x-app": "zinc"},
		Routes: []Route{{
			Header:     "X-Mode",
			Value:      "blocked",
			Middleware: func(c *zinc.Context) error { return zinc.ErrForbidden },
		}},
	}))
	app.Get("/", func(c *zinc.Context) error { return c.String("ok") })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("X-App") != "zinc" {
		t.Fatalf("status=%d X-App=%q", rec.Code, rec.Header().Get("X-App"))
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Mode", "blocked")
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("routed status=%d", rec.Code)
	}
}

func TestHeadersRejectsIncompleteRoute(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a Route without Middleware did not panic")
		}
	}()
	New(Config{Routes: []Route{{Header: "X-Mode"}}})
}
