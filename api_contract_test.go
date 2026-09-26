// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

var (
	_ http.Handler = (*zinc.App)(nil)

	_ zinc.HandlerFunc    = func(*zinc.Context) error { return nil }
	_ zinc.HTTPMiddleware = func(next http.Handler) http.Handler { return next }

	_ func(*zinc.App, string, ...zinc.HandlerFunc) zinc.Route         = (*zinc.App).Get
	_ func(*zinc.App, string, ...zinc.HandlerFunc) zinc.Route         = (*zinc.App).Post
	_ func(*zinc.App, string, string, ...zinc.HandlerFunc) zinc.Route = (*zinc.App).Add
	_ func(*zinc.App, zinc.RouteSpec) error                           = (*zinc.App).TryHandle
	_ func(*zinc.App, string, http.Handler) zinc.Route                = (*zinc.App).HandleHTTP
	_ func(*zinc.App, ...zinc.HTTPMiddleware)                         = (*zinc.App).UseHTTP
	_ func(zinc.Route, string) zinc.Route                             = zinc.Route.Name
	_ func(zinc.HTTPMiddleware) zinc.Middleware                       = zinc.FromHTTP

	_ func(*zinc.Group, string, ...zinc.HandlerFunc) zinc.Route         = (*zinc.Group).Get
	_ func(*zinc.Group, string, ...zinc.HandlerFunc) zinc.Route         = (*zinc.Group).Post
	_ func(*zinc.Group, string, string, ...zinc.HandlerFunc) zinc.Route = (*zinc.Group).Add
	_ func(*zinc.Group, zinc.RouteSpec) error                           = (*zinc.Group).TryHandle
	_ func(*zinc.Group, string, http.Handler) zinc.Route                = (*zinc.Group).HandleHTTP
	_ func(*zinc.Group, ...zinc.HTTPMiddleware) *zinc.Group             = (*zinc.Group).UseHTTP
)

func TestPublicAPIContract(t *testing.T) {
	app := zinc.New()
	app.UseHTTP(func(next http.Handler) http.Handler { return next })

	app.Get("/users/{id}", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{
			"context": c.Param("id"),
		})
	})
	app.HandleHTTP("GET /native/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("id")))
	}))

	api := app.Group("/api")
	api.Add(http.MethodGet, "/status", func(c *zinc.Context) error { return c.String("ready") }).Name("api.status")
	if err := api.TryHandle(zinc.RouteSpec{
		Name:    "api.dynamic",
		Method:  http.MethodPost,
		Path:    "/dynamic/{name}",
		Handler: func(c *zinc.Context) error { return c.String(c.Param("name")) },
	}); err != nil {
		t.Fatalf("TryHandle: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || strings.TrimSpace(resp.Body.String()) != `{"context":"42"}` {
		t.Fatalf("status=%d body=%q", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/native/84", nil)
	resp = httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || resp.Body.String() != "84" {
		t.Fatalf("native status=%d body=%q", resp.Code, resp.Body.String())
	}
}
