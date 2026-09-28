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
		for _, placement := range []string{"use", "prefix", "group"} {
			logs.Reset()
			app := zinc.New()
			switch placement {
			case "use":
				app.Use(mw())
			case "prefix":
				app.UsePrefix("/api", mw())
			}
			api := app.Group("/api")
			if placement == "group" {
				api.Use(mw())
			}
			api.Get("/users", func(c *zinc.Context) error { return c.String("ok") })
			for range 3 {
				app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/users", nil))
			}

			warnings := strings.Count(logs.String(), "level=WARN")
			if placement == "group" {
				if warnings != 1 || !strings.Contains(logs.String(), "middleware="+name) || !strings.Contains(logs.String(), "route=/api/users") {
					t.Fatalf("%s on a group: want one warning, got %q", name, logs.String())
				}
			} else if warnings != 0 {
				t.Fatalf("%s with %s: unexpected warning %q", name, placement, logs.String())
			}
		}
	}
}
