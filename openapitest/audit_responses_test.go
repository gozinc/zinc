// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/0mjs/zinc"
)

type problem struct {
	Code string `json:"code"`
}

// createdItem sends its location and a cookie as headers, not in the body.
type createdItem struct {
	Location string       `header:"Location" json:"-"`
	Session  *http.Cookie `header:"Set-Cookie" json:"-"`
	Count    int          `header:"X-Count" json:"-"`
	ID       int          `json:"id"`
}

// account has a field for each spec-only role, and names its component.
type account struct {
	ID       string `json:"id" openapi:"readonly"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty" openapi:"writeonly"`
	Nick     string `json:"nick,omitempty" openapi:"deprecated"`
}

func (account) OpenAPIName() string { return "Account" }

type echoed struct {
	Region string `header:"X-Region" json:"region"`
}

func responseScenarios() []scenario {
	dir, _ := os.MkdirTemp("", "zinc-audit-files")
	_ = os.WriteFile(filepath.Join(dir, "report.csv"), []byte("a,b\n1,2\n"), 0o644)

	return []scenario{
		{id: "R01", area: "Responses", title: "Typed handler with Status(201)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Post("/items", zinc.Typed(func(*zinc.Context, struct{}) (item, error) { return item{ID: 1}, nil })).Status(http.StatusCreated)
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "POST", target: "/items", status: 201}},
			expect: func(f *findings, s spec) {
				if s.response("POST", "/items", 201) == nil || s.response("POST", "/items", 200) != nil {
					f.add("want 201 and no 200")
				}
			}},
		{id: "R02", area: "Responses", title: "NoContent gives 204",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Delete("/items/{id}", zinc.Typed(func(*zinc.Context, pathInt) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "DELETE", target: "/items/1", status: 204}},
			expect: func(f *findings, s spec) {
				if r := s.response("DELETE", "/items/{id}", 204); r == nil || r["content"] != nil {
					f.add("204 should have no content: %v", r)
				}
			}},
		{id: "R03", area: "Responses", title: "Slice output: empty and nil",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/empty", zinc.Typed(func(*zinc.Context, struct{}) ([]item, error) { return []item{}, nil }))
				app.Get("/nil", zinc.Typed(func(*zinc.Context, struct{}) ([]item, error) { return nil, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/empty"), get("/nil")},
			note:   "A nil slice is sent as null."},
		{id: "R04", area: "Responses", title: "Nil pointer output",
			build:  serveValue[*item]("/v", nil),
			probes: []probe{get("/v")}},
		{id: "R05", area: "Responses", title: "Errors(404, 409) through the error handler",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/items/{id}", zinc.Typed(func(_ *zinc.Context, in pathInt) (item, error) {
					switch in.ID {
					case 404:
						return item{}, zinc.NotFound("no item")
					case 409:
						return item{}, zinc.NewError(http.StatusConflict, "busy")
					}
					return item{ID: int(in.ID)}, nil
				})).Errors(http.StatusNotFound, http.StatusConflict)
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/items/1"), {method: "GET", target: "/items/404", status: 404}, {method: "GET", target: "/items/409", status: 409}}},
		{id: "R06", area: "Responses", title: "Response(status, T) written by the handler",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Post("/items", zinc.Typed(func(c *zinc.Context, _ struct{}) (item, error) {
					return item{}, c.Status(http.StatusConflict).JSON(problem{Code: "taken"})
				})).Response(http.StatusConflict, problem{})
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "POST", target: "/items", status: 409}}},
		{id: "R07", area: "Responses", title: "Custom error handler writing text",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New(zinc.Config{ErrorHandler: zinc.TextErrors})
				app.Get("/boom", zinc.Typed(func(*zinc.Context, struct{}) (item, error) { return item{}, errors.New("db down") }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/boom", status: 500}},
			note:   "Zinc can't know a custom handler's body; the spec lists the status alone."},
		{id: "R08", area: "Responses", title: "Plain handler without Output sending JSON",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/v", func(c *zinc.Context) error { return c.JSON(item{ID: 1}) })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/v")}},
		{id: "R09", area: "Responses", title: "Plain handler sending text (c.String)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/hello", func(c *zinc.Context) error { return c.String("hello") })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/hello")}},
		{id: "R10", area: "Responses", title: "HTML page",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/page", func(c *zinc.Context) error { return c.HTML("<h1>hi</h1>") })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/page")}},
		{id: "R11", area: "Responses", title: "File download",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/report", func(c *zinc.Context) error { return c.Attachment(filepath.Join(dir, "report.csv"), "report.csv") })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/report")}},
		{id: "R12", area: "Responses", title: "Redirect",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/old", func(c *zinc.Context) error { return c.Redirect("/new") })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/old", status: http.StatusFound}}},
		{id: "R13", area: "Responses", title: "Streaming (server-sent events)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/events", func(c *zinc.Context) error {
					return c.Stream("text/event-stream", strings.NewReader("data: hi\n\n"))
				})
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/events")}},
		{id: "R14", area: "Responses", title: "Automatic HEAD for a GET route",
			build:  serveValue("/v", item{ID: 1}),
			probes: []probe{{method: "HEAD", target: "/v", status: 200}},
			note:   "OpenAPI lists HEAD only when it's declared; Zinc answers it automatically."},
		{id: "R15", area: "Responses", title: "Automatic OPTIONS",
			build:  serveValue("/v", item{ID: 1}),
			probes: []probe{{method: "OPTIONS", target: "/v", status: 204}}},
		{id: "R16", area: "Responses", title: "Wrong method gives 405",
			build:  serveValue("/v", item{ID: 1}),
			probes: []probe{{method: "DELETE", target: "/v", status: 405}}},
		{id: "R17", area: "Responses", title: "HTTPError with Details",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/x", zinc.Typed(func(*zinc.Context, struct{}) (item, error) {
					return item{}, zinc.NewError(http.StatusConflict, "stale").WithDetail("version", 3)
				})).Errors(http.StatusConflict)
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/x", status: 409}}},
		{id: "R18", area: "Responses", title: "Validation failure with fields (422)",
			build:  typedPost[newPet]("/pets", validated()),
			probes: []probe{post("/pets", `{"name":""}`, 422)}},
		{id: "R19", area: "Responses", title: "Status error not declared with Errors",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/x", zinc.Typed(func(*zinc.Context, struct{}) (item, error) { return item{}, zinc.ErrForbidden }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/x", status: 403}},
			note:   "The spec can only know statuses the route declares."},
		{id: "R20", area: "Responses", title: "Auth middleware answering 401",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				api := app.Group("/api", func(c *zinc.Context) error {
					if c.Header("Authorization") == "" {
						return zinc.ErrUnauthorized
					}
					return c.Next()
				}).Security("bearer")
				api.Get("/me", zinc.Typed(func(*zinc.Context, struct{}) (item, error) { return item{ID: 1}, nil }))
				return app, zinc.OpenAPIConfig{SecuritySchemes: map[string]zinc.OpenAPISecurityScheme{"bearer": {Type: "http", Scheme: "bearer"}}}
			},
			probes: []probe{{method: "GET", target: "/api/me", status: 401}, {method: "GET", target: "/api/me", header: http.Header{"Authorization": {"Bearer x"}}, status: 200}},
			expect: func(f *findings, s spec) {
				if s.response("GET", "/api/me", 401) == nil {
					f.add("a route with security usually documents 401; Zinc adds none")
				}
			}},
		{id: "R21", area: "Responses", title: "Typed handler sending CSV (Bytes and Produces)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/csv", zinc.Typed(func(c *zinc.Context, _ struct{}) (zinc.Bytes, error) {
					return zinc.Bytes{Type: "text/csv", Data: []byte("a,b\n")}, nil
				})).Produces(http.StatusOK, "text/csv")
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/csv")},
			expect: func(f *findings, s spec) {
				if mediaSchema(s.response("GET", "/csv", 200), "text/csv") == nil {
					f.add("text/csv isn't documented with a string schema: %v", s.response("GET", "/csv", 200))
				}
			}},
		{id: "R24", area: "Responses", title: "Text output",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/hello", zinc.Typed(func(*zinc.Context, struct{}) (zinc.Text, error) { return "hello", nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/hello")},
			expect: func(f *findings, s spec) {
				if !typeIs(mediaSchema(s.response("GET", "/hello", 200), "text/plain"), "string") {
					f.add("Text should be a text/plain string")
				}
			}},
		{id: "R25", area: "Responses", title: "HTML output",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/page", zinc.Typed(func(*zinc.Context, struct{}) (zinc.HTML, error) { return "<h1>hi</h1>", nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/page")}},
		{id: "R26", area: "Responses", title: "File output as a download",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/report", zinc.Typed(func(*zinc.Context, struct{}) (zinc.File, error) {
					return zinc.File{Path: filepath.Join(dir, "report.csv"), Name: "report.csv"}, nil
				})).Produces(http.StatusOK, "text/csv")
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/report")}},
		{id: "R27", area: "Responses", title: "Stream output as server-sent events",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/events", zinc.Typed(func(*zinc.Context, struct{}) (zinc.Stream, error) {
					return zinc.Stream{Type: "text/event-stream", Reader: strings.NewReader("data: hi\n\n")}, nil
				})).Produces(http.StatusOK, "text/event-stream")
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/events")}},
		{id: "R28", area: "Responses", title: "Redirect output with a 301",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/old", zinc.Typed(func(*zinc.Context, struct{}) (zinc.Redirect, error) { return "/new", nil })).Status(http.StatusMovedPermanently)
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/old", status: http.StatusMovedPermanently}},
			expect: func(f *findings, s spec) {
				if r := s.response("GET", "/old", 301); r == nil || r["headers"] == nil {
					f.add("a 301 with a Location header should be documented: %v", s.op("GET", "/old")["responses"])
				}
			}},
		{id: "R29", area: "Responses", title: "Plain handler with Produces",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/export", func(c *zinc.Context) error { return c.Data("text/csv", []byte("a,b\n")) }).Produces(http.StatusOK, "text/csv")
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/export")},
			expect: func(f *findings, s spec) {
				if s.response("GET", "/export", 200) == nil || s.op("GET", "/export")["responses"].(map[string]any)["default"] != nil {
					f.add("Produces should document a 200, with no default fallback")
				}
			}},
		{id: "R30", area: "Responses", title: "Typed handler writing a body its output type doesn't describe",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/csv", zinc.Typed(func(c *zinc.Context, _ struct{}) (item, error) {
					return item{}, c.Data("text/csv", []byte("a,b\n"))
				}))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/csv")},
			note:   "The handler bypasses its output type; no spec can know. Return zinc.Bytes with Produces instead (R21)."},
		{id: "R31", area: "Responses", title: "Response headers from output fields (Location, Set-Cookie)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Post("/items", zinc.Typed(func(*zinc.Context, struct{}) (createdItem, error) {
					return createdItem{Location: "/items/1", Session: &http.Cookie{Name: "s", Value: "1"}, Count: 1, ID: 1}, nil
				})).Status(http.StatusCreated)
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "POST", target: "/items", status: 201}},
			expect: func(f *findings, s spec) {
				headers, _ := s.response("POST", "/items", 201)["headers"].(map[string]any)
				for _, h := range []string{"Location", "Set-Cookie", "X-Count"} {
					if headers[h] == nil {
						f.add("the %s header isn't documented: %v", h, headers)
					}
				}
				if prop(s.responseSchema("POST", "/items", 201), "Location") != nil {
					f.add("a header field is also in the body schema")
				}
			}},
		{id: "R32", area: "Responses", title: "Header-tagged field of an echoed input stays in the body",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/echo", zinc.Typed(func(_ *zinc.Context, in echoed) (echoed, error) { return in, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/echo", header: http.Header{"X-Region": {"eu"}}, status: 200}},
			expect: func(f *findings, s spec) {
				if prop(s.responseSchema("GET", "/echo", 200), "region") == nil {
					f.add("the echoed field left the body schema")
				}
			}},
		{id: "R33", area: "Responses", title: "ProblemErrors: RFC 9457 bodies, documented",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New(zinc.Config{ErrorHandler: zinc.ProblemErrors})
				app.Get("/items/{id}", zinc.Typed(func(_ *zinc.Context, in struct {
					ID int `path:"id"`
				}) (item, error) {
					if in.ID != 1 {
						return item{}, zinc.NewError(http.StatusNotFound, "no such item").WithDetail("id", in.ID)
					}
					return item{ID: 1}, nil
				})).Errors(http.StatusNotFound).
					Example(http.StatusNotFound, "unknown id", zinc.NewError(http.StatusNotFound, "no such item"))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/items/1"), {method: "GET", target: "/items/2", status: 404}, {method: "GET", target: "/items/x", status: 400}},
			expect: func(f *findings, s spec) {
				if mediaSchema(s.response("GET", "/items/{id}", 404), "application/problem+json") == nil {
					f.add("the 404 isn't documented as application/problem+json")
				}
			}},
		{id: "R34", area: "Responses", title: "Named examples for success, error and request",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Post("/pets", zinc.Typed(func(_ *zinc.Context, in newPet) (newPet, error) { return in, nil })).
					Status(http.StatusCreated).
					RequestExample("a cat", newPet{Name: "Tom", Kind: "cat"}).
					Example(http.StatusCreated, "a cat", newPet{Name: "Tom", Kind: "cat", Tags: []string{}}).
					Errors(http.StatusConflict).
					Example(http.StatusConflict, "taken", zinc.NewError(http.StatusConflict, "name taken"))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{post("/pets", `{"name":"Tom","kind":"cat"}`, 201)},
			expect: func(f *findings, s spec) {
				for _, status := range []int{201, 409} {
					media, _ := s.response("POST", "/pets", status)["content"].(map[string]any)["application/json"].(map[string]any)
					if media["examples"] == nil {
						f.add("no examples for %d", status)
					}
				}
			}},
		{id: "R35", area: "Responses", title: "Spec-only field roles and a chosen component name",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/me", zinc.Typed(func(*zinc.Context, struct{}) (account, error) { return account{ID: "1", Email: "a@b.c"}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/me")},
			expect: func(f *findings, s spec) {
				c := s.component("Account")
				if c == nil {
					f.add("OpenAPIName didn't name the component: %v", s.components())
					return
				}
				for field, keyword := range map[string]string{"id": "readOnly", "password": "writeOnly", "nick": "deprecated"} {
					if prop(c, field)[keyword] != true {
						f.add("%s isn't marked %s", field, keyword)
					}
				}
			}},
		{id: "R22", area: "Responses", title: "Output declared on a plain handler, sent as JSON",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/v", func(c *zinc.Context) error { return c.JSON(item{ID: 1}) }).Output(item{})
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/v")}},
		{id: "R23", area: "Responses", title: "Panic recovered as 500",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/panic", zinc.Typed(func(*zinc.Context, struct{}) (item, error) { panic("boom") }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/panic", status: 500}},
			note:   "Without the recover middleware, net/http closes the connection; with it, Zinc sends a 500."},
	}
}
