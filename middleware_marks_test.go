// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc_test

import (
	"net/http"
	"testing"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/cors"
	"github.com/0mjs/zinc/middleware/redirect"
	"github.com/0mjs/zinc/middleware/rewrite"
)

// What Zinc knows about a middleware belongs to that middleware. Two
// closures made by the same function, such as two Skip wrappers, never share
// each other's marks.

func never(*zinc.Context) bool { return false }

func passThrough(c *zinc.Context) error { return c.Next() }

func newRewrite() zinc.Middleware {
	return rewrite.New(rewrite.Config{Rules: map[string]string{"/old": "/new"}})
}

func mustNotPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if v := recover(); v != nil {
			t.Errorf("%s: unexpected panic: %v", what, v)
		}
	}()
	f()
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected a registration panic", what)
		}
	}()
	f()
}

func TestMarksSkipPreroutingStaysWithItsInstance(t *testing.T) {
	app := zinc.New()
	marked := zinc.Skip(never, newRewrite())
	app.Use(marked)
	plain := zinc.Skip(never, passThrough)

	mustNotPanic(t, "Group with Skip(plain)", func() { app.Group("/a", plain) })
	mustNotPanic(t, "Group.Use with Skip(plain)", func() { app.Group("/b").Use(plain) })
	mustPanic(t, "Group with Skip(rewrite)", func() { app.Group("/c", marked) })
	mustPanic(t, "Group.Use with Skip(redirect)", func() {
		app.Group("/d").Use(zinc.Skip(never, redirect.New(redirect.Config{Rules: map[string]string{"/x": "/y"}})))
	})

	// The rewrite still runs before routing from app.Use.
	app.Get("/new", func(c *zinc.Context) error { return c.String("new") })
	if w := serve(app, "GET", "/old", ""); w.Code != http.StatusOK || w.Body.String() != "new" {
		t.Fatalf("rewrite in Skip: %d %q", w.Code, w.Body)
	}
}

func TestMarksRegistrationOrder(t *testing.T) {
	app := zinc.New()
	// A plain Skip registered before any marked one exists, and another
	// made after: neither picks up a mark made in between.
	early := zinc.Skip(never, passThrough)
	mustNotPanic(t, "early plain Skip", func() { app.Group("/early", early) })
	marked := zinc.Skip(never, newRewrite())
	late := zinc.Skip(never, passThrough)
	mustNotPanic(t, "early plain Skip again", func() { app.Group("/early2", early) })
	mustNotPanic(t, "late plain Skip", func() { app.Group("/late", late) })
	mustPanic(t, "marked Skip", func() { app.Group("/marked", marked) })
}

func TestMarksSkipPreflightStaysWithItsInstance(t *testing.T) {
	authRan := false
	auth := func(c *zinc.Context) error { authRan = true; return zinc.ErrUnauthorized }
	app := zinc.New()
	// Auth outside CORS: on an automatic OPTIONS only CORS runs.
	api := app.Group("/api",
		zinc.Skip(never, auth),
		zinc.Skip(never, cors.New(cors.Config{AllowOrigins: []string{"https://app.example.com"}})))
	api.Post("/pets", func(c *zinc.Context) error { return c.NoContent() })

	w := serve(app, "OPTIONS", "/api/pets", "", "Origin", "https://app.example.com", "Access-Control-Request-Method", "POST")
	if authRan {
		t.Error("Skip-wrapped auth ran on an automatic OPTIONS request")
	}
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("preflight: %d %v", w.Code, w.Header())
	}
	// The real request still runs auth.
	if w := serve(app, "POST", "/api/pets", "", "Origin", "https://app.example.com"); w.Code != http.StatusUnauthorized || !authRan {
		t.Fatalf("real request: %d, auth ran %v", w.Code, authRan)
	}
}

func TestMarksNestedSkip(t *testing.T) {
	app := zinc.New()
	mustPanic(t, "Skip(Skip(rewrite))", func() { app.Group("/a", zinc.Skip(never, zinc.Skip(never, newRewrite()))) })
	mustNotPanic(t, "Skip(Skip(plain))", func() { app.Group("/b", zinc.Skip(never, zinc.Skip(never, passThrough))) })
	mustNotPanic(t, "Skip(plain) beside a nested marked one", func() { app.Group("/c", zinc.Skip(never, passThrough)) })

	authRan := false
	auth := func(c *zinc.Context) error { authRan = true; return zinc.ErrUnauthorized }
	app.Group("/api",
		zinc.Skip(never, zinc.Skip(never, auth)),
		zinc.Skip(never, zinc.Skip(never, cors.New()))).
		Put("/pets", func(c *zinc.Context) error { return c.NoContent() })
	w := serve(app, "OPTIONS", "/api/pets", "", "Origin", "https://app.example.com", "Access-Control-Request-Method", "PUT")
	if authRan {
		t.Error("nested Skip-wrapped auth ran on an automatic OPTIONS request")
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("nested Skip-wrapped CORS didn't answer the preflight: %d %v", w.Code, w.Header())
	}
}

func TestMarksAcrossApps(t *testing.T) {
	first := zinc.New()
	first.Use(zinc.Skip(never, newRewrite()), zinc.Skip(never, cors.New()))

	second := zinc.New()
	mustNotPanic(t, "Skip(plain) on another app's group", func() { second.Group("/a", zinc.Skip(never, passThrough)) })
	mustPanic(t, "Skip(rewrite) on another app's group", func() { second.Group("/b", zinc.Skip(never, newRewrite())) })

	authRan := false
	second.Group("/api", zinc.Skip(never, func(c *zinc.Context) error { authRan = true; return zinc.ErrUnauthorized })).
		Post("/pets", func(c *zinc.Context) error { return c.NoContent() })
	w := serve(second, "OPTIONS", "/api/pets", "", "Origin", "https://app.example.com", "Access-Control-Request-Method", "POST")
	if authRan || w.Code != http.StatusNoContent {
		t.Fatalf("automatic OPTIONS on the second app: %d, auth ran %v", w.Code, authRan)
	}
}
