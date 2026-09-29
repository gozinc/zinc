// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/apidocs"
)

func secured(schemes map[string]zinc.OpenAPISecurityScheme, def []string, register func(app *zinc.App)) func() (*zinc.App, zinc.OpenAPIConfig) {
	return func() (*zinc.App, zinc.OpenAPIConfig) {
		app := zinc.New()
		register(app)
		return app, zinc.OpenAPIConfig{SecuritySchemes: schemes, Security: def}
	}
}

func securityScenarios() []scenario {
	bearer := map[string]zinc.OpenAPISecurityScheme{"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"}}
	return []scenario{
		{id: "S01", area: "Security", title: "Bearer for every route",
			build: secured(bearer, []string{"bearer"}, func(app *zinc.App) { app.Get("/me", noContent) }),
			expect: func(f *findings, s spec) {
				if asList(s.raw["security"]) == nil {
					f.add("top-level security missing")
				}
			}},
		{id: "S02", area: "Security", title: "API key on a group, one public route",
			build: secured(map[string]zinc.OpenAPISecurityScheme{"key": {Type: "apiKey", In: "header", Name: "X-API-Key"}}, nil, func(app *zinc.App) {
				api := app.Group("/api").Security("key")
				api.Get("/private", noContent)
				api.Get("/health", noContent).Security()
			}),
			expect: func(f *findings, s spec) {
				if sec, ok := s.op("GET", "/api/health")["security"].([]any); !ok || len(sec) != 0 {
					f.add("health should be public (security: [])")
				}
			}},
		{id: "S03", area: "Security", title: "HTTP basic",
			build: secured(map[string]zinc.OpenAPISecurityScheme{"basic": {Type: "http", Scheme: "basic"}}, []string{"basic"}, func(app *zinc.App) { app.Get("/me", noContent) })},
		{id: "S04", area: "Security", title: "OpenID Connect",
			build: secured(map[string]zinc.OpenAPISecurityScheme{"oidc": {Type: "openIdConnect", OpenIDConnectURL: "https://id.example.com/.well-known/openid-configuration"}}, []string{"oidc"}, func(app *zinc.App) { app.Get("/me", noContent) })},
		{id: "S05", area: "Security", title: "Mutual TLS",
			build: secured(map[string]zinc.OpenAPISecurityScheme{"mtls": {Type: "mutualTLS"}}, []string{"mtls"}, func(app *zinc.App) { app.Get("/me", noContent) })},
		{id: "S06", area: "Security", title: "OAuth2",
			build: secured(map[string]zinc.OpenAPISecurityScheme{"oauth": {Type: "oauth2", Flows: &zinc.OpenAPIOAuthFlows{
				AuthorizationCode: &zinc.OpenAPIOAuthFlow{AuthorizationURL: "https://id.example.com/authorize", TokenURL: "https://id.example.com/token", Scopes: map[string]string{"pets:read": "Read pets"}},
				ClientCredentials: &zinc.OpenAPIOAuthFlow{TokenURL: "https://id.example.com/token"},
			}}}, []string{"oauth"}, func(app *zinc.App) { app.Get("/me", noContent) }),
			expect: func(f *findings, s spec) {
				if _, err := zinc.New().OpenAPISpec(zinc.OpenAPIConfig{SecuritySchemes: map[string]zinc.OpenAPISecurityScheme{"o": {Type: "oauth2"}}}); err == nil {
					f.add("an oauth2 scheme without flows should be an error, not an invalid spec")
				}
			}},
		{id: "S07", area: "Security", title: "Two schemes on one route",
			build: secured(map[string]zinc.OpenAPISecurityScheme{"key": {Type: "apiKey", In: "header", Name: "X-API-Key"}, "bearer": {Type: "http", Scheme: "bearer"}}, nil, func(app *zinc.App) {
				app.Get("/either", noContent).Security("key", "bearer")
				app.Get("/both", noContent).SecurityAll("key", "bearer")
			}),
			expect: func(f *findings, s spec) {
				if either := asList(s.op("GET", "/either")["security"]); len(either) != 2 {
					f.add("Security(key, bearer) should be two alternatives: %v", either)
				}
				if both := asList(s.op("GET", "/both")["security"]); len(both) != 1 || len(both[0].(map[string]any)) != 2 {
					f.add("SecurityAll(key, bearer) should be one requirement with both: %v", both)
				}
			}},
		{id: "S08", area: "Security", title: "Unknown scheme name",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/x", noContent).Security("nope")
				return app, zinc.OpenAPIConfig{}
			},
			note: "OpenAPISpec returns an error, by design."},
		{id: "S09", area: "Security", title: "apiKey in a query parameter and a cookie",
			build: secured(map[string]zinc.OpenAPISecurityScheme{
				"q": {Type: "apiKey", In: "query", Name: "api_key"},
				"c": {Type: "apiKey", In: "cookie", Name: "session"},
			}, []string{"q"}, func(app *zinc.App) { app.Get("/x", noContent).Security("c") })},
	}
}

func metadataScenarios() []scenario {
	return []scenario{
		{id: "M01", area: "Metadata", title: "Name becomes operationId",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/a", noContent).Name("users.list")
				app.Get("/b", noContent).Name("users.show")
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				if s.op("GET", "/a")["operationId"] != "users.list" {
					f.add("operationId missing")
				}
			}},
		{id: "M02", area: "Metadata", title: "Summary, description, deprecated",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/a", noContent).Summary("One line").Description("Multi\n\n**line** markdown").Deprecated()
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				op := s.op("GET", "/a")
				if op["summary"] != "One line" || op["deprecated"] != true || !strings.Contains(op["description"].(string), "**line**") {
					f.add("metadata lost: %v", op)
				}
			}},
		{id: "M03", area: "Metadata", title: "Group and route tags, and tag descriptions",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				g := app.Group("/pets").Tags("pets")
				g.Get("", noContent).Tags("read")
				return app, zinc.OpenAPIConfig{Tags: []zinc.OpenAPITag{{Name: "pets", Description: "Everything about pets"}}}
			},
			expect: func(f *findings, s spec) {
				tags := asList(s.op("GET", "/pets")["tags"])
				if len(tags) != 2 || tags[0] != "pets" {
					f.add("tags should be [pets read]: %v", tags)
				}
				if tags := asList(s.raw["tags"]); len(tags) != 2 || tags[0].(map[string]any)["description"] != "Everything about pets" {
					f.add("top-level tags should describe pets, then list read: %v", s.raw["tags"])
				}
			}},
		{id: "M04", area: "Metadata", title: "Hidden route and hidden group",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/internal", noContent).Hidden()
				admin := app.Group("/admin").Hidden()
				admin.Get("/a", noContent)
				admin.Get("/b", noContent)
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				if s.hasPath("/internal") {
					f.add("hidden route listed")
				}
				if s.hasPath("/admin/a") || s.hasPath("/admin/b") {
					f.add("hidden group listed")
				}
			}},
		{id: "M05", area: "Metadata", title: "Info: description, contact, license, terms, servers",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/a", noContent)
				return app, zinc.OpenAPIConfig{Title: "Shop", Version: "2.1.0", Description: "An API.",
					Contact: &zinc.OpenAPIContact{Email: "api@example.com"}, License: &zinc.OpenAPILicense{Name: "MIT", Identifier: "MIT"},
					TermsOfService: "https://example.com/terms", ExternalDocs: &zinc.OpenAPIExternalDocs{URL: "https://example.com/docs"},
					Servers: []zinc.OpenAPIServer{{URL: "https://api.example.com", Description: "prod"}, {URL: "http://localhost:8080"}}}
			}},
		{id: "M07", area: "Metadata", title: "A validator that enforces no tag rules",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				type out struct {
					Name string `json:"name" validate:"min=3"`
				}
				app := zinc.New(zinc.Config{Validator: noopValidator{}})
				app.Get("/v", zinc.Typed(func(*zinc.Context, struct{}) (out, error) { return out{Name: "x"}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if prop(s.responseSchema("GET", "/v", 200), "name")["minLength"] != nil {
					f.add("the spec claims minLength from a tag no validator enforces")
				}
			}},
		{id: "M06", area: "Metadata", title: "Example on a body field",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				type in struct {
					Name string `json:"name" example:"Rex"`
					Age  int    `json:"age" example:"3"`
					Tags []int  `json:"tags" example:"[1,2]"`
				}
				app := zinc.New()
				app.Post("/p", zinc.Typed(func(*zinc.Context, in) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			expect: func(f *findings, s spec) {
				sch := s.requestSchema("POST", "/p", "application/json")
				if prop(sch, "age")["examples"] == nil || prop(sch, "tags")["examples"] == nil {
					f.add("examples lost: %v", props(sch))
				}
			}},
	}
}

func servingScenarios() []scenario {
	fetch := func(app *zinc.App, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		return w
	}
	return []scenario{
		{id: "V01", area: "Serving", title: "Served spec equals OpenAPISpec; docs page",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/a", noContent)
				app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{Title: "Audit", Version: "1"})
				app.Get("/docs", apidocs.New()).Hidden()
				return app, zinc.OpenAPIConfig{Title: "Audit", Version: "1"}
			},
			expect: func(f *findings, s spec) {
				if s.hasPath("/openapi.json") || s.hasPath("/docs") {
					f.add("spec or docs route listed")
				}
			}},
		{id: "V02", area: "Serving", title: "Routes added after the first spec request",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{Title: "Audit", Version: "1"})
				first := fetch(app, "/openapi.json").Body.String()
				app.Get("/late", noContent)
				if second := fetch(app, "/openapi.json").Body.String(); first == second || !strings.Contains(second, "/late") {
					panic("the served spec didn't pick up /late")
				}
				return app, zinc.OpenAPIConfig{}
			}},
		{id: "V03", area: "Serving", title: "Protected spec route",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{}, func(c *zinc.Context) error {
					if c.Header("X-Key") != "k" {
						return zinc.ErrUnauthorized
					}
					return c.Next()
				})
				if fetch(app, "/openapi.json").Code != http.StatusUnauthorized {
					panic("unprotected")
				}
				return app, zinc.OpenAPIConfig{}
			}},
		{id: "V04", area: "Serving", title: "Two spec endpoints with different configs",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/a", noContent)
				app.OpenAPI("/v1.json", zinc.OpenAPIConfig{Title: "One", Version: "1"})
				app.OpenAPI("/v2.json", zinc.OpenAPIConfig{Title: "Two", Version: "2"})
				if !strings.Contains(fetch(app, "/v2.json").Body.String(), `"Two"`) {
					panic("second config not used")
				}
				return app, zinc.OpenAPIConfig{}
			}},
		{id: "V05", area: "Serving", title: "Spec route on a path a route already uses",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/openapi.json", noContent)
				app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{})
				return app, zinc.OpenAPIConfig{}
			},
			note: "Expected: a registration panic, like any duplicate route."},
		{id: "V07", area: "Serving", title: "Spec served by default",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New(zinc.Config{OpenAPI: zinc.OpenAPIConfig{Title: "Default", Version: "1"}})
				app.Get("/a", noContent)
				if body := fetch(app, "/openapi.json").Body.String(); !strings.Contains(body, `"Default"`) || !strings.Contains(body, `"/a"`) {
					panic("no spec at /openapi.json: " + body)
				}
				off := zinc.New(zinc.Config{OpenAPIPath: "-"})
				if fetch(off, "/openapi.json").Code != http.StatusNotFound {
					panic(`OpenAPIPath "-" still serves a spec`)
				}
				return app, zinc.OpenAPIConfig{}
			}},
		{id: "V09", area: "Serving", title: "Metadata changed after the first spec request",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				route := app.Get("/x", noContent)
				fetch(app, "/openapi.json")
				route.Hidden()
				if strings.Contains(fetch(app, "/openapi.json").Body.String(), `"/x"`) {
					panic("the served spec still lists a route hidden after the first request")
				}
				return app, zinc.OpenAPIConfig{}
			}},
		{id: "V08", area: "Serving", title: "A route at /openapi.json beats the default spec",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/openapi.json", func(c *zinc.Context) error { return c.String("mine") })
				if fetch(app, "/openapi.json").Body.String() != "mine" {
					panic("the default spec replaced the app's route")
				}
				return app, zinc.OpenAPIConfig{}
			}},
		{id: "V06", area: "Serving", title: "An app with no routes",
			build: func() (*zinc.App, zinc.OpenAPIConfig) { return zinc.New(), zinc.OpenAPIConfig{} }},
	}
}

// noopValidator accepts everything: it implements Validator but enforces no
// validate tags.
type noopValidator struct{}

func (noopValidator) Validate(any) error { return nil }
