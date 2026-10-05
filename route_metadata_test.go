// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// seen records what standard middleware and handlers read off the request.
type seen struct{ log []string }

func (s *seen) record(who string, r *http.Request, params ...string) {
	parts := []string{who, r.Pattern}
	for _, p := range params {
		parts = append(parts, p+"="+r.PathValue(p))
	}
	s.log = append(s.log, strings.Join(parts, "|"))
}

func (s *seen) middleware(who string, params ...string) HTTPMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.record(who, r, params...)
			next.ServeHTTP(w, r)
		})
	}
}

func (s *seen) handler(params ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.record("handler", r, params...)
		w.WriteHeader(http.StatusNoContent)
	})
}

func (s *seen) want(t *testing.T, want ...string) {
	t.Helper()
	if strings.Join(s.log, "\n") != strings.Join(want, "\n") {
		t.Fatalf("seen:\n%s\nwant:\n%s", strings.Join(s.log, "\n"), strings.Join(want, "\n"))
	}
}

func TestNativeMiddlewareSeesMatchedRoute(t *testing.T) {
	s := &seen{}
	app := New()
	app.UseHTTP(s.middleware("app-http", "id"))
	app.Use(FromHTTP(s.middleware("app-use", "id")))
	g := app.Group("/items", FromHTTP(s.middleware("group", "id")))
	g.HandleHTTP("GET /{id}", s.handler("id"))
	app.Get("/routes/{id}", FromHTTP(s.middleware("route", "id")), Wrap(s.handler("id")))

	if resp := performRequest(t, app, http.MethodGet, "/items/42", nil, nil); resp.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%q", resp.Code, resp.Body.String())
	}
	s.want(t,
		"app-http||id=",
		"app-use||id=",
		"group|GET /items/{id}|id=42",
		"handler|GET /items/{id}|id=42",
	)

	s.log = nil
	performRequest(t, app, http.MethodGet, "/routes/7", nil, nil)
	s.want(t,
		"app-http||id=",
		"app-use||id=",
		"route|GET /routes/{id}|id=7",
		"handler|GET /routes/{id}|id=7",
	)

	// A GET route answers HEAD; http.ServeMux reports the GET pattern too.
	s.log = nil
	performRequest(t, app, http.MethodHead, "/routes/8", nil, nil)
	s.want(t,
		"app-http||id=",
		"app-use||id=",
		"route|GET /routes/{id}|id=8",
		"handler|GET /routes/{id}|id=8",
	)
}

func TestNativeMiddlewareRouteSurvivesReplacedRequest(t *testing.T) {
	type key struct{}
	s := &seen{}
	replace := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key{}, "v")))
		})
	}
	app := New()
	g := app.Group("/a", FromHTTP(replace), FromHTTP(s.middleware("after-replace", "x", "y")))
	g.HandleHTTP("GET /{x}/b/{y}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(key{}) != "v" {
			t.Error("handler lost the replaced context")
		}
		s.record("handler", r, "x", "y")
	}))
	// A Zinc middleware between replaces the request with a new URL.
	h := app.Group("/c", FromHTTP(replace), func(c *Context) error {
		r := c.Request().Clone(c.Context())
		c.SetRequest(r)
		return c.Next()
	}, FromHTTP(s.middleware("after-clone", "rest")))
	h.HandleHTTP("GET /{rest...}", s.handler("rest"))

	performRequest(t, app, http.MethodGet, "/a/1/b/2", nil, nil)
	s.want(t,
		"after-replace|GET /a/{x}/b/{y}|x=1|y=2",
		"handler|GET /a/{x}/b/{y}|x=1|y=2",
	)

	s.log = nil
	performRequest(t, app, http.MethodGet, "/c/deep/er/path", nil, nil)
	s.want(t,
		"after-clone|GET /c/{rest...}|rest=deep/er/path",
		"handler|GET /c/{rest...}|rest=deep/er/path",
	)
}

func TestNativeMiddlewareCatchAllAndManyParams(t *testing.T) {
	s := &seen{}
	app := New()
	mw := FromHTTP(s.middleware("mw", "org", "repo", "path"))
	app.Get("/{org}/{repo}/blob/{path...}", mw, Wrap(s.handler("org", "repo", "path")))

	performRequest(t, app, http.MethodGet, "/zinc/core/blob/docs/a.md", nil, nil)
	s.want(t,
		"mw|GET /{org}/{repo}/blob/{path...}|org=zinc|repo=core|path=docs/a.md",
		"handler|GET /{org}/{repo}/blob/{path...}|org=zinc|repo=core|path=docs/a.md",
	)
}

func TestPlainZincHandlerGetsNoNativeRoute(t *testing.T) {
	app := New()
	app.Get("/users/{id}", func(c *Context) error {
		r := c.Request()
		if r.Pattern != "" || r.PathValue("id") != "" {
			return fmt.Errorf("pattern=%q id=%q", r.Pattern, r.PathValue("id"))
		}
		return c.String(c.Param("id"))
	})
	if resp := performRequest(t, app, http.MethodGet, "/users/3", nil, nil); resp.Body.String() != "3" {
		t.Fatalf("status=%d body=%q", resp.Code, resp.Body.String())
	}
}
