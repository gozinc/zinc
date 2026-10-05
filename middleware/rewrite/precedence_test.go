// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package rewrite

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func newPrecedenceApp(rules map[string]string) *zinc.App {
	app := zinc.New()
	app.Use(New(Config{Rules: rules}))
	app.Get("/{path...}", func(c *zinc.Context) error {
		return c.String(c.Path())
	})
	return app
}

func rewritten(app *zinc.App, path string) string {
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Body.String()
}

// Overlapping rules must pick the same winner on every request and every
// construction: the longer literal prefix is the more specific rule.
func TestRewriteOverlappingRulesAreDeterministic(t *testing.T) {
	for build := 0; build < 50; build++ {
		app := newPrecedenceApp(map[string]string{
			"/*":     "/global/*",
			"/api/*": "/specific/*",
		})
		for i := 0; i < 1000; i++ {
			if got := rewritten(app, "/api/pets"); got != "/specific/pets" {
				t.Fatalf("build %d request %d: path=%q want /specific/pets", build, i, got)
			}
		}
		if got := rewritten(app, "/other"); got != "/global/other" {
			t.Fatalf("build %d: fallback path=%q", build, got)
		}
	}
}

func TestRewriteExactBeatsWildcard(t *testing.T) {
	for build := 0; build < 50; build++ {
		app := newPrecedenceApp(map[string]string{
			"/*":           "/global/*",
			"/api/*":       "/specific/*",
			"/api/pets":    "/exact",
			"/api/pets/*":  "/pets/*",
			"/api/pets/v*": "/versioned/*",
		})
		for _, tc := range []struct{ in, want string }{
			{"/api/pets", "/exact"},
			{"/api/pets/v2", "/versioned/2"},
			{"/api/pets/1", "/pets/1"},
			{"/api/other", "/specific/other"},
			{"/x", "/global/x"},
		} {
			for i := 0; i < 20; i++ {
				if got := rewritten(app, tc.in); got != tc.want {
					t.Fatalf("build %d: %s → %q want %q", build, tc.in, got, tc.want)
				}
			}
		}
	}
}

// BenchmarkRewriteTenRules routes a request through a rewrite with ten
// rules, most of them prefixes, so rule matching dominates the middleware.
func BenchmarkRewriteTenRules(b *testing.B) {
	app := zinc.New()
	app.Use(New(Config{Rules: map[string]string{
		"/":             "/home",
		"/old":          "/new",
		"/login":        "/signin",
		"/*":            "/global/*",
		"/v1/*":         "/api/v1/*",
		"/v2/*":         "/api/v2/*",
		"/docs/*":       "/manual/*",
		"/api/*":        "/specific/*",
		"/api/pets/*":   "/pets/*",
		"/static/img/*": "/assets/img/*",
	}}))
	app.Get("/{path...}", func(c *zinc.Context) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/api/pets/42", nil)
	rec := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// The rewrite changes the request's path, so put it back.
		req.URL.Path = "/api/pets/42"
		app.ServeHTTP(rec, req)
	}
}
