// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/bodylimit"
	"github.com/0mjs/zinc/middleware/csrf"
	"github.com/0mjs/zinc/middleware/limiter"
	"github.com/0mjs/zinc/middleware/timeout"
)

// Each Doc describes what its middleware answers, checked against the
// middleware itself.
func TestMiddlewareDocsMatchBehavior(t *testing.T) {
	send := func(app *zinc.App, method, target, body string) int {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w.Code
	}
	for _, tt := range []struct {
		name   string
		use    zinc.Middleware
		doc    zinc.MiddlewareDoc
		method string
		body   string
		route  zinc.HandlerFunc
	}{
		{"csrf", csrf.New(), csrf.Doc(), http.MethodPost, "", func(c *zinc.Context) error { return c.NoContent() }},
		{"bodylimit", bodylimit.New(bodylimit.Config{Limit: 4}), bodylimit.Doc(), http.MethodPost, "too long", func(c *zinc.Context) error {
			_, err := c.BodyBytes()
			if err != nil {
				return err
			}
			return c.NoContent()
		}},
		{"timeout", timeout.New(timeout.Config{Timeout: time.Millisecond}), timeout.Doc(), http.MethodGet, "", func(c *zinc.Context) error {
			<-c.Context().Done()
			return c.Context().Err()
		}},
		{"limiter", limiter.New(limiter.Config{Rate: 1, Capacity: 1}), limiter.Doc(), http.MethodGet, "", func(c *zinc.Context) error { return c.NoContent() }},
	} {
		app := zinc.New()
		app.Use(tt.use)
		app.Document(tt.doc)
		app.Add(tt.method, "/", tt.route)
		var got int
		for range 3 {
			if got = send(app, tt.method, "/", tt.body); got >= 400 {
				break
			}
		}
		spec, err := app.OpenAPISpec(zinc.OpenAPIConfig{Title: "T", Version: "1"})
		if err != nil {
			t.Fatal(err)
		}
		if got < 400 || !strings.Contains(string(spec), `"`+http.StatusText(got)+`"`) {
			t.Errorf("%s answered %d, which its Doc doesn't describe", tt.name, got)
		}
	}
	if doc := csrf.Doc(); doc.Security["csrf"].Name != zinc.HeaderXCSRFToken || doc.Security["csrf"].In != "header" || len(doc.Headers) != 0 {
		t.Errorf("csrf.Doc: %+v", doc)
	}
}
