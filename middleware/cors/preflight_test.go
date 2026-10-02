// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package cors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/cors"
)

func preflight(app *zinc.App, path, method string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodOptions, path, nil)
	r.Header.Set("Origin", "https://app.example.com")
	if method != "" {
		r.Header.Set("Access-Control-Request-Method", method)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

// CORS on a group answers its routes' automatic preflight requests; other
// group middleware, such as auth, doesn't run for them.
func TestGroupCORSAnswersPreflight(t *testing.T) {
	authRan := false
	auth := func(c *zinc.Context) error { authRan = true; return zinc.ErrUnauthorized }
	app := zinc.New()
	api := app.Group("/api", cors.New(cors.Config{AllowOrigins: []string{"https://app.example.com"}}), auth)
	api.Post("/pets", func(c *zinc.Context) error { return c.NoContent() })
	v2 := api.Group("/v2", zinc.Skip(func(*zinc.Context) bool { return false }, cors.New()))
	v2.Put("/pets/{id}", func(c *zinc.Context) error { return c.NoContent() })
	app.Group("/plain").Post("/x", func(c *zinc.Context) error { return c.NoContent() })

	w := preflight(app, "/api/pets", "POST")
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("group preflight: %d %v", w.Code, w.Header())
	}
	if authRan {
		t.Fatal("auth middleware ran on a preflight request")
	}
	// Nested groups, and CORS wrapped in Skip.
	if w := preflight(app, "/api/v2/pets/7", "PUT"); w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("nested preflight: %d %v", w.Code, w.Header())
	}
	// No CORS on the route, or a method the path doesn't have: the plain
	// automatic OPTIONS answer.
	for _, tt := range [][2]string{{"/plain/x", "POST"}, {"/api/pets", "DELETE"}, {"/api/pets", ""}} {
		w := preflight(app, tt[0], tt[1])
		if w.Code != http.StatusNoContent || w.Header().Get("Allow") == "" || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%v: %d %v", tt, w.Code, w.Header())
		}
	}
	// The real request still runs the whole chain.
	r := httptest.NewRequest(http.MethodPost, "/api/pets", nil)
	r.Header.Set("Origin", "https://app.example.com")
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || !authRan {
		t.Fatalf("real request: %d, auth ran %v", w.Code, authRan)
	}
}
