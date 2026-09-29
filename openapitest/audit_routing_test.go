// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"fmt"
	"io/fs"
	"net/http"
	"testing/fstest"
	"time"

	"github.com/0mjs/zinc"
)

func noContent(c *zinc.Context) error { return c.NoContent() }

func routingScenarios() []scenario {
	return []scenario{
		{id: "X01", area: "Routing", title: "Nested groups",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				v1 := app.Group("/api").Group("/v1")
				v1.Get("/items/{id}", zinc.Typed(func(_ *zinc.Context, in pathInt) (item, error) { return item{ID: int(in.ID)}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/api/v1/items/3")}},
		{id: "X02", area: "Routing", title: "Match with several methods",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Match([]string{"GET", "POST"}, "/both", noContent)
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/both", status: 204}, {method: "POST", target: "/both", status: 204}},
			expect: func(f *findings, s spec) {
				if s.op("GET", "/both") == nil || s.op("POST", "/both") == nil {
					f.add("both methods should be operations")
				}
			},
			note: "A plain handler without Output: its 204 is covered by the default response."},
		{id: "X03", area: "Routing", title: "Custom method (PURGE)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Add("PURGE", "/cache", noContent)
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				if s.hasPath("/cache") {
					f.add("OpenAPI 3.1 has no field for PURGE; it should be left out")
				}
			},
			note: "OpenAPI 3.2 adds additionalOperations for custom methods."},
		{id: "X04", area: "Routing", title: "Mounted http.Handler",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Mount("/legacy", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				f.add("info: paths %v", s.raw["paths"])
			}},
		{id: "X05", area: "Routing", title: "HandleHTTP with a method pattern",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.HandleHTTP("GET /std/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"id":%q}`, r.PathValue("id"))
				}))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/std/7")},
			expect: func(f *findings, s spec) {
				if s.op("GET", "/std/{id}") == nil {
					f.add("HandleHTTP route missing: %v", s.raw["paths"])
				}
			}},
		{id: "X06", area: "Routing", title: "Static files",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				var files fs.FS = fstest.MapFS{"a.txt": {Data: []byte("a"), ModTime: time.Unix(0, 0)}}
				app.StaticFS("/assets", files)
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				f.add("info: paths %v", s.raw["paths"])
			}},
		{id: "X07", area: "Routing", title: "Route with a trailing slash",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/items/", zinc.Typed(func(*zinc.Context, struct{}) ([]item, error) { return []item{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/items/")},
			expect: func(f *findings, s spec) { f.add("info: paths %v", s.raw["paths"]) }},
		{id: "X08", area: "Routing", title: "One path, several methods",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/items/{id}", zinc.Typed(func(_ *zinc.Context, in pathInt) (item, error) { return item{}, nil }))
				app.Put("/items/{id}", zinc.Typed(func(_ *zinc.Context, in pathInt) (item, error) { return item{}, nil }))
				app.Delete("/items/{id}", zinc.Typed(func(_ *zinc.Context, in pathInt) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/items/1"), {method: "PUT", target: "/items/1", status: 200}, {method: "DELETE", target: "/items/1", status: 204}}},
		{id: "X09", area: "Routing", title: "Case-insensitive routing",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/Reports/Daily", zinc.Typed(func(*zinc.Context, struct{}) (item, error) { return item{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/reports/daily")},
			note:   "OpenAPI paths are case-sensitive; Zinc's matching isn't, by default."},
		{id: "X10", area: "Routing", title: "RouteNotFound handler",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.RouteNotFound("/api/{rest...}", func(c *zinc.Context) error { return zinc.NotFound("unknown API route") })
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				if s.hasPath("/api/{rest}") {
					f.add("a not-found handler isn't an operation")
				}
			}},
		{id: "X11", area: "Routing", title: "1,000 routes",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				for i := range 1000 {
					app.Get(fmt.Sprintf("/r%d/{id}", i), zinc.Typed(func(_ *zinc.Context, in pathInt) (item, error) { return item{}, nil }))
				}
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/r999/1")},
			expect: func(f *findings, s spec) {
				if paths, _ := s.raw["paths"].(map[string]any); len(paths) != 1000 {
					f.add("want 1000 paths, got %d", len(paths))
				}
			}},
		{id: "X12", area: "Routing", title: "Unicode path parameter name",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/café/{naïve}", func(c *zinc.Context) error { return c.NoContent() })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/caf%C3%A9/x", status: 204}}},
		{id: "X13", area: "Routing", title: "Root route",
			build:  serveValue("/", item{ID: 1}),
			probes: []probe{get("/")}},
		{id: "X14", area: "Routing", title: "Literal ':' in a segment (/v1/users:batch)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Post("/v1/users:batch", zinc.Typed(func(*zinc.Context, struct{}) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "POST", target: "/v1/users:batch", status: 204}}},
		{id: "X16", area: "Routing", title: "A parameter and a catch-all on the same path",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/files/{path}", noContent).Name("single")
				app.Get("/files/{path...}", noContent).Name("catchall")
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				f.add("two routes became one operation, %v, without an error", s.op("GET", "/files/{path}")["operationId"])
			},
			note: "OpenAPI can't tell these apart, so building the spec should fail with a clear error."},
		{id: "X17", area: "Routing", title: "One path shape with different parameter names",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/pets/{id}", noContent)
				app.Post("/pets/{name}", noContent)
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				if s.hasPath("/pets/{id}") && s.hasPath("/pets/{name}") {
					f.add("two path templates differ only in parameter names, which OpenAPI forbids")
				}
			}},
		{id: "X15", area: "Routing", title: "Route middleware before a typed handler",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				mw := func(c *zinc.Context) error { return c.Next() }
				app.Get("/items/{id}", mw, mw, zinc.Typed(func(_ *zinc.Context, in pathInt) (item, error) { return item{ID: int(in.ID)}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/items/2")},
			expect: func(f *findings, s spec) {
				if p := s.param("GET", "/items/{id}", "path", "id"); p == nil || !typeIs(p["schema"].(map[string]any), "integer") {
					f.add("typed types lost behind route middleware")
				}
			}},
	}
}
