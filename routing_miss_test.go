// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

// Middleware sees a routing miss as ErrNotFound or ErrMethodNotAllowed
// whichever error handler is configured, and the client gets the same bytes.
func TestRoutingMissReachesMiddlewareAsError(t *testing.T) {
	type result struct {
		seen          error
		code          int
		body, ct, all string
		allowValues   int
	}
	run := func(custom bool, method, path string, extra zinc.Middleware) result {
		cfg := zinc.Config{}
		if custom {
			cfg.ErrorHandler = func(c *zinc.Context, err error) { zinc.DefaultErrorHandler(c, err) }
		}
		app := zinc.New(cfg)
		var seen error
		app.Use(func(c *zinc.Context) error {
			seen = c.Next()
			return seen
		})
		if extra != nil {
			app.Use(extra)
		}
		app.Get("/users", func(c *zinc.Context) error { return c.String("ok") })
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return result{seen, w.Code, w.Body.String(), w.Header().Get("Content-Type"), w.Header().Get("Allow"), len(w.Header().Values("Allow"))}
	}

	for _, tt := range []struct {
		method, path string
		want         error
	}{
		{"GET", "/nope", zinc.ErrNotFound},
		{"DELETE", "/users", zinc.ErrMethodNotAllowed},
		{"HEAD", "/nope", zinc.ErrNotFound},
	} {
		def, cus := run(false, tt.method, tt.path, nil), run(true, tt.method, tt.path, nil)
		if !errors.Is(def.seen, tt.want) || !errors.Is(cus.seen, tt.want) {
			t.Fatalf("%s %s: middleware saw %v (default) and %v (custom), want %v", tt.method, tt.path, def.seen, cus.seen, tt.want)
		}
		if def != (result{def.seen, cus.code, cus.body, cus.ct, cus.all, cus.allowValues}) {
			t.Fatalf("%s %s: default %+v, custom %+v", tt.method, tt.path, def, cus)
		}
		if tt.method == "HEAD" && def.body != "" {
			t.Fatalf("HEAD miss wrote a body: %q", def.body)
		}
	}
	if got := run(false, "DELETE", "/users", nil); got.all != "GET, HEAD, OPTIONS" || got.ct != "application/json; charset=utf-8" {
		t.Fatalf("405 headers: %+v", got)
	}

	// A middleware that adds to Allow before the response is written keeps
	// both values, and the default JSON Content-Type is not disturbed.
	appendAllow := func(c *zinc.Context) error {
		err := c.Next()
		c.AppendHeader("Allow", "PURGE")
		c.HandleError(err)
		return err
	}
	if got := run(false, "DELETE", "/users", appendAllow); got.code != 405 || got.ct != "application/json; charset=utf-8" || got.allowValues != 2 {
		t.Fatalf("405 after Allow append: %+v", got)
	}

	// A middleware can answer the miss itself.
	html404 := func(c *zinc.Context) error {
		err := c.Next()
		if errors.Is(err, zinc.ErrNotFound) {
			return c.Status(404).HTML("<h1>Not here</h1>")
		}
		return err
	}
	for _, custom := range []bool{false, true} {
		got := run(custom, "GET", "/nope", html404)
		if got.code != 404 || got.body != "<h1>Not here</h1>" || got.ct != "text/html; charset=utf-8" || got.seen != nil {
			t.Fatalf("custom=%v own 404: %+v", custom, got)
		}
	}
}
