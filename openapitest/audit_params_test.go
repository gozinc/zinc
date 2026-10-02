// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"net/http"
	"time"

	"github.com/0mjs/zinc"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// playground adapts go-playground/validator, as most Zinc apps would.
type playground struct{ v *validator.Validate }

func (p playground) Validate(v any) error { return p.v.Struct(v) }

// RuleSet declares the rules the audit's types use, all of which
// go-playground/validator enforces, so the spec claims them.
func (playground) RuleSet() []string { return append(zinc.BuiltinRules(), "dive") }

func validated() zinc.Config { return zinc.Config{Validator: playground{validator.New()}} }

type pathInt struct {
	ID int64 `path:"id"`
}

type pathMixed struct {
	Small uint8   `path:"small"`
	Ratio float64 `path:"ratio"`
	On    bool    `path:"on"`
}

type pathUUID struct {
	ID uuid.UUID `path:"id"`
}

type queryInt struct {
	Page int `query:"page"`
}

type queryInts struct {
	IDs []int `query:"id"`
}

type queryPointer struct {
	Limit *int `query:"limit"`
}

type queryKinds struct {
	On    bool      `query:"on"`
	Ratio float64   `query:"ratio"`
	Since time.Time `query:"since"`
}

type queryRequired struct {
	Q string `query:"q" validate:"required"`
}

type headerInt struct {
	Version int `header:"x-api-version"`
}

type headerRequired struct {
	Tenant string `header:"X-Tenant" validate:"required"`
}

type queryDefault struct {
	Limit int `query:"limit" default:"20"`
}

type cookieInput struct {
	Session string `cookie:"session" validate:"required"`
}

type pathAndQuery struct {
	ID int `path:"id" query:"id"`
}

type embeddedParams struct {
	Page int `query:"page"`
}

type withEmbeddedParams struct {
	embeddedParams
	Name string `query:"name"`
}

type unnamedQuery struct {
	SortBy string `query:",omitempty"`
}

type documentedQuery struct {
	Page int `query:"page" doc:"Page number, from 1." example:"2"`
}

func typedGet[In any](path string, cfg zinc.Config) func() (*zinc.App, zinc.OpenAPIConfig) {
	return func() (*zinc.App, zinc.OpenAPIConfig) {
		app := zinc.New(cfg)
		app.Get(path, zinc.Typed(func(_ *zinc.Context, in In) (In, error) { return in, nil }))
		return app, zinc.OpenAPIConfig{}
	}
}

func paramScenarios() []scenario {
	return []scenario{
		{id: "P01", area: "Parameters", title: "Integer path parameter",
			build:  typedGet[pathInt]("/items/{id}", zinc.Config{}),
			probes: []probe{get("/items/5"), {method: "GET", target: "/items/x", status: 400}},
			expect: func(f *findings, s spec) {
				p := s.param("GET", "/items/{id}", "path", "id")
				if p == nil || p["required"] != true || !typeIs(p["schema"].(map[string]any), "integer") {
					f.add("id should be a required integer path parameter: %v", p)
				}
			}},
		{id: "P02", area: "Parameters", title: "uint8, float64 and bool path parameters",
			build:  typedGet[pathMixed]("/m/{small}/{ratio}/{on}", zinc.Config{}),
			probes: []probe{get("/m/7/1.5/true"), {method: "GET", target: "/m/300/1/true", status: 400}},
			expect: func(f *findings, s spec) {
				for name, want := range map[string]string{"small": "integer", "ratio": "number", "on": "boolean"} {
					p := s.param("GET", "/m/{small}/{ratio}/{on}", "path", name)
					if p == nil || !typeIs(p["schema"].(map[string]any), want) {
						f.add("%s should be %s: %v", name, want, p)
					}
				}
				if sch, _ := s.param("GET", "/m/{small}/{ratio}/{on}", "path", "small")["schema"].(map[string]any); sch["maximum"] == nil {
					f.add("uint8 has no maximum 255 (300 is rejected with 400)")
				}
			}},
		{id: "P03", area: "Parameters", title: "Path parameter a plain handler doesn't bind",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/users/{name}", func(c *zinc.Context) error { return c.JSON(map[string]string{"name": c.Param("name")}) })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/users/ada")},
			expect: func(f *findings, s spec) {
				if p := s.param("GET", "/users/{name}", "path", "name"); p == nil || !typeIs(p["schema"].(map[string]any), "string") {
					f.add("name should be a string path parameter")
				}
			},
			note: "The handler sends JSON without .Output, so the default response covers its body."},
		{id: "P04", area: "Parameters", title: "uuid.UUID path parameter",
			build:  typedGet[pathUUID]("/things/{id}", zinc.Config{}),
			probes: []probe{get("/things/7f1c0a52-7e4c-4d3b-9c2c-6c4f0f6d2a11"), {method: "GET", target: "/things/nope", status: 400}},
			expect: func(f *findings, s spec) {
				sch, _ := s.param("GET", "/things/{id}", "path", "id")["schema"].(map[string]any)
				if sch["format"] != "uuid" {
					f.add("a uuid path parameter isn't documented with format uuid: %v", sch)
				}
			}},
		{id: "P05", area: "Parameters", title: "Wildcard path {rest...}",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/files/{path...}", func(c *zinc.Context) error { return c.NoContent() })
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/files/a/b/c.txt", status: 204}},
			expect: func(f *findings, s spec) {
				if !s.hasPath("/files/{path}") {
					f.add("wildcard path missing")
				}
			},
			note: "OpenAPI path parameters can't contain '/' unless percent-encoded; a wildcard can."},
		{id: "P06", area: "Parameters", title: "Optional integer query parameter",
			build:  typedGet[queryInt]("/q", zinc.Config{}),
			probes: []probe{get("/q"), get("/q?page=3"), {method: "GET", target: "/q?page=x", status: 400}},
			expect: func(f *findings, s spec) {
				p := s.param("GET", "/q", "query", "page")
				if p == nil || p["required"] == true {
					f.add("page should be an optional query parameter: %v", p)
				}
			}},
		{id: "P07", area: "Parameters", title: "Repeated query parameter ([]int)",
			build:  typedGet[queryInts]("/q", zinc.Config{}),
			probes: []probe{get("/q?id=1&id=2")},
			expect: func(f *findings, s spec) {
				p := s.param("GET", "/q", "query", "id")
				sch, _ := p["schema"].(map[string]any)
				if !typeIs(sch, "array") || !typeIs(sch["items"].(map[string]any), "integer") {
					f.add("id should be an array of integers: %v", p)
				}
			}},
		{id: "P08", area: "Parameters", title: "Pointer query parameter",
			build:  typedGet[queryPointer]("/q", zinc.Config{}),
			probes: []probe{get("/q"), get("/q?limit=5")},
			expect: func(f *findings, s spec) {
				sch, _ := s.param("GET", "/q", "query", "limit")["schema"].(map[string]any)
				if typeIs(sch, "null") {
					f.add("a query parameter is absent or a value; null isn't a query value: %v", sch)
				}
			}},
		{id: "P09", area: "Parameters", title: "bool, float and time.Time query parameters",
			build:  typedGet[queryKinds]("/q", zinc.Config{}),
			probes: []probe{get("/q?on=true&ratio=0.5&since=2026-01-02T03:04:05Z")},
			expect: func(f *findings, s spec) {
				sch, _ := s.param("GET", "/q", "query", "since")["schema"].(map[string]any)
				if sch["format"] != "date-time" {
					f.add("time.Time query parameter isn't date-time: %v", sch)
				}
			}},
		{id: "P10", area: "Parameters", title: "Required query parameter (validator)",
			build:  typedGet[queryRequired]("/q", validated()),
			probes: []probe{get("/q?q=x"), {method: "GET", target: "/q", status: 422}},
			expect: func(f *findings, s spec) {
				if p := s.param("GET", "/q", "query", "q"); p == nil || p["required"] != true {
					f.add("q should be required: %v", p)
				}
			}},
		{id: "P11", area: "Parameters", title: "Integer header",
			build:  typedGet[headerInt]("/h", zinc.Config{}),
			probes: []probe{{method: "GET", target: "/h", header: http.Header{"X-Api-Version": {"2"}}, status: 200}},
			expect: func(f *findings, s spec) {
				if p := s.param("GET", "/h", "header", "X-Api-Version"); p == nil || !typeIs(p["schema"].(map[string]any), "integer") {
					f.add("X-Api-Version should be an integer header: %v", s.op("GET", "/h")["parameters"])
				}
			}},
		{id: "P12", area: "Parameters", title: "Required header (validator)",
			build:  typedGet[headerRequired]("/h", validated()),
			probes: []probe{{method: "GET", target: "/h", header: http.Header{"X-Tenant": {"acme"}}, status: 200}, {method: "GET", target: "/h", status: 422}},
			expect: func(f *findings, s spec) {
				if p := s.param("GET", "/h", "header", "X-Tenant"); p == nil || p["required"] != true {
					f.add("X-Tenant should be required")
				}
			}},
		{id: "P13", area: "Parameters", title: "Cookie read in the handler",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New(validated())
				app.Get("/me", zinc.Typed(func(_ *zinc.Context, in cookieInput) (map[string]string, error) {
					return map[string]string{"session": in.Session}, nil
				}))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "GET", target: "/me", header: http.Header{"Cookie": {"session=abc"}}, status: 200}, {method: "GET", target: "/me", status: 422}},
			expect: func(f *findings, s spec) {
				if p := s.param("GET", "/me", "cookie", "session"); p == nil || p["required"] != true {
					f.add("session should be a required cookie parameter: %v", p)
				}
			}},
		{id: "P14", area: "Parameters", title: "Query parameter with a default",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/q", zinc.Typed(func(_ *zinc.Context, in queryDefault) (queryDefault, error) {
					if in.Limit != 20 {
						panic("the default didn't bind")
					}
					return in, nil
				}))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/q")},
			expect: func(f *findings, s spec) {
				sch, _ := s.param("GET", "/q", "query", "limit")["schema"].(map[string]any)
				if sch["default"] == nil {
					f.add("no way to document a query parameter's default")
				}
			}},
		{id: "P15", area: "Parameters", title: "One field bound from path and query",
			build:  typedGet[pathAndQuery]("/a/{id}", zinc.Config{}),
			probes: []probe{get("/a/1?id=2")},
			expect: func(f *findings, s spec) {
				if s.param("GET", "/a/{id}", "path", "id") == nil {
					f.add("path id missing")
				}
				f.add("info: parameters: %v", s.op("GET", "/a/{id}")["parameters"])
			}},
		{id: "P16", area: "Parameters", title: "Parameter tags in an embedded struct",
			build:  typedGet[withEmbeddedParams]("/q", zinc.Config{}),
			probes: []probe{get("/q?page=2&name=x")},
			expect: func(f *findings, s spec) {
				if s.param("GET", "/q", "query", "page") == nil {
					f.add("page, promoted from an embedded struct, binds but isn't a parameter in the spec")
				}
				if sch := s.requestSchema("GET", "/q", "application/json"); prop(sch, "page") != nil {
					f.add("page is a parameter, so it isn't a body field")
				}
			},
			note: "Binding promotes tagged fields from embedded structs, as Go does."},
		{id: "P17", area: "Parameters", title: "Query tag with options and no name",
			build:  typedGet[unnamedQuery]("/q", zinc.Config{}),
			probes: []probe{get("/q?sortby=name")},
			expect: func(f *findings, s spec) {
				if s.param("GET", "/q", "query", "sortby") == nil {
					f.add("an unnamed query tag binds the lower-cased field name, sortby: %v", s.op("GET", "/q")["parameters"])
				}
			}},
		{id: "P18", area: "Parameters", title: "doc and example tags on a parameter",
			build:  typedGet[documentedQuery]("/q", zinc.Config{}),
			probes: []probe{get("/q?page=2")},
			expect: func(f *findings, s spec) {
				p := s.param("GET", "/q", "query", "page")
				sch, _ := p["schema"].(map[string]any)
				if p["description"] == nil && sch["description"] == nil {
					f.add("doc tag lost")
				}
				f.add("info: parameter %v", p)
			}},
	}
}
