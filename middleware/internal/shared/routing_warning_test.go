// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package shared_test

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/redirect"
	"github.com/0mjs/zinc/middleware/rewrite"
)

func TestRoutingWarningOnlyAfterRouting(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	middlewares := map[string]func() zinc.Middleware{
		"redirect": func() zinc.Middleware {
			return redirect.New(redirect.Config{Rules: map[string]string{"/api/old": "/api/new"}})
		},
		"rewrite": func() zinc.Middleware {
			return rewrite.New(rewrite.Config{Rules: map[string]string{"/api/v1/*": "/api/v2/*"}})
		},
	}
	for name, mw := range middlewares {
		// Before routing: no warning.
		for _, placement := range []string{"use", "prefix"} {
			logs.Reset()
			app := zinc.New()
			if placement == "use" {
				app.Use(mw())
			} else {
				app.UsePrefix("/api", mw())
			}
			app.Group("/api").Get("/users", func(c *zinc.Context) error { return c.String("ok") })
			for range 3 {
				app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/users", nil))
			}
			if strings.Contains(logs.String(), "level=WARN") {
				t.Fatalf("%s with %s: unexpected warning %q", name, placement, logs.String())
			}
		}

		// On a group, in any of the three ways to give it middleware:
		// registration panics.
		want := name + ` middleware on group "/api" would run after routing`
		for placement, register := range map[string]func(app *zinc.App){
			"Group.Use":   func(app *zinc.App) { app.Group("/api").Use(mw()) },
			"App.Group":   func(app *zinc.App) { app.Group("/api", mw()) },
			"child Group": func(app *zinc.App) { app.Group("/").Group("/api", mw()) },
			"App.Route":   func(app *zinc.App) { app.Route("/api", nil, mw()) },
		} {
			func() {
				defer func() {
					got, _ := recover().(string)
					if !strings.Contains(got, want) || !strings.Contains(got, `app.UsePrefix("/api", ...)`) {
						t.Fatalf("%s via %s: panic %q, want it to contain %q", name, placement, got, want)
					}
				}()
				register(zinc.New())
			}()
		}

		// On one route it works for that path, so it only warns, once.
		logs.Reset()
		app := zinc.New()
		app.Get("/api/users", mw(), func(c *zinc.Context) error { return c.String("ok") })
		for range 3 {
			app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/users", nil))
		}
		if warnings := strings.Count(logs.String(), "level=WARN"); warnings != 1 || !strings.Contains(logs.String(), "middleware="+name) {
			t.Fatalf("%s on a route: want one warning, got %q", name, logs.String())
		}
	}
}
