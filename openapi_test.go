// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

type oaValidator struct{}

func (oaValidator) Validate(any) error { return nil }

type oaPet struct {
	ID    int64    `json:"id" doc:"The pet's ID." example:"7"`
	Name  string   `json:"name" validate:"required,min=1,max=40"`
	Kind  string   `json:"kind" validate:"oneof=cat dog"`
	Owner *oaOwner `json:"owner"`
}

type oaOwner struct {
	Email string `json:"email" validate:"required,email"`
}

type oaCreatePet struct {
	Store  string `path:"store"`
	DryRun bool   `query:"dry_run" doc:"Validate without saving."`
	Tenant string `header:"x-tenant" validate:"required"`
	Name   string `json:"name" validate:"required"`
	Kind   string `json:"kind"`
}

type oaPetID struct {
	ID int64 `path:"id"`
}

type oaListPets struct {
	Page    int      `query:"page" validate:"min=1"`
	Kinds   []string `query:"kind" enum:"cat,dog"`
	Limit   int8     `query:"limit" default:"20"`
	Sort    oaSort   `query:"sort" default:"name"`
	Session string   `cookie:"session"`
}

// oaSort lists its values, so it becomes an enum component.
type oaSort string

func (oaSort) Enum() []any { return []any{"name", "age"} }

type oaUpload struct {
	Caption string                `form:"caption" validate:"required"`
	Photo   *multipart.FileHeader `form:"photo"`
}

type oaConflict struct {
	Reason string `json:"reason"`
}

// oaFixture registers one route for each rule the spec builder follows.
func oaFixture() *App {
	app := New(Config{Validator: oaValidator{}})
	app.Use(func(c *Context) error { return c.Next() })

	stores := app.Group("/stores/{store}").Tags("pets").Security("apiKey")
	stores.Post("/pets", Typed(func(*Context, oaCreatePet) (oaPet, error) { return oaPet{}, nil })).
		Status(http.StatusCreated).
		Name("pets.create").
		Summary("Add a pet").
		Description("Adds a pet to the store. **Dry runs** save nothing.").
		Response(http.StatusConflict, oaConflict{})

	app.Get("/pets", Typed(func(*Context, oaListPets) ([]oaPet, error) { return nil, nil })).Name("pets.list")
	app.Get("/pets/{id}", Typed(func(*Context, oaPetID) (*oaPet, error) { return nil, nil })).Tags("pets").Errors(http.StatusNotFound)
	app.Delete("/pets/{id}", Typed(func(*Context, oaPetID) (NoContent, error) { return NoContent{}, nil })).Deprecated()

	// A plain handler that documents itself, and one that doesn't.
	app.Put("/pets/{id}", func(c *Context) error { return nil }).
		Input(oaCreatePet{}).
		Output(oaPet{}).
		Response(http.StatusNotFound, nil).
		Security() // public, overriding the default
	app.Get("/health", func(c *Context) error { return c.String("ok") })

	app.Post("/pets/{id}/photo", Typed(func(*Context, oaUpload) (NoContent, error) { return NoContent{}, nil }))
	app.Get("/files/{path...}", func(c *Context) error { return nil })
	app.Post("/pets/{id}/adopt", Typed(func(*Context, oaPetID) (NoContent, error) { return NoContent{}, nil })).
		SecurityAll("apiKey", "oauth:pets:write")

	admin := app.Group("/admin").Hidden()
	admin.Get("/stats", func(c *Context) error { return nil })

	// Left out: hidden routes, methods OpenAPI 3.1 has no field for, mounts.
	app.Get("/internal/metrics", func(c *Context) error { return nil }).Hidden()
	app.Add("PURGE", "/cache", func(c *Context) error { return nil })
	app.Mount("/legacy", http.NotFoundHandler())
	return app
}

func oaFixtureConfig() OpenAPIConfig {
	return OpenAPIConfig{
		Title:          "Pet Store",
		Version:        "1.2.0",
		Description:    "The fixture for Zinc's OpenAPI golden test.",
		TermsOfService: "https://example.com/terms",
		Contact:        &OpenAPIContact{Name: "API team", Email: "api@example.com"},
		License:        &OpenAPILicense{Name: "MIT", Identifier: "MIT"},
		ExternalDocs:   &OpenAPIExternalDocs{URL: "https://example.com/docs"},
		Servers:        []OpenAPIServer{{URL: "https://api.example.com", Description: "Production"}},
		Tags:           []OpenAPITag{{Name: "pets", Description: "Everything about pets"}},
		SecuritySchemes: map[string]OpenAPISecurityScheme{
			"apiKey": {Type: "apiKey", In: "header", Name: "X-API-Key"},
			"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
			"oauth": {Type: "oauth2", Flows: &OpenAPIOAuthFlows{ClientCredentials: &OpenAPIOAuthFlow{
				TokenURL: "https://id.example.com/token",
				Scopes:   map[string]string{"pets:write": "Change pets"},
			}}},
		},
		Security: []string{"bearer"},
	}
}

// TestOpenAPIGolden compares the fixture's spec with the checked-in file.
// Run go test -run TestOpenAPIGolden -update to rewrite it after a change,
// and review the diff. The openapitest module validates the same file
// against the OpenAPI 3.1 schema.
func TestOpenAPIGolden(t *testing.T) {
	got, err := oaFixture().OpenAPISpec(oaFixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	again, _ := oaFixture().OpenAPISpec(oaFixtureConfig())
	if !bytes.Equal(got, again) {
		t.Fatal("the spec isn't the same for the same routes")
	}
	path := filepath.Join("testdata", "openapi", "golden.json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("spec differs from %s; run with -update and review the diff.\n%s", path, firstDiff(want, got))
	}
}

func firstDiff(want, got []byte) string {
	w, g := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := range max(len(w), len(g)) {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return "line " + strconv.Itoa(i+1) + ":\n want " + wl + "\n  got " + gl
		}
	}
	return ""
}

func TestOpenAPISpecErrorsAndDefaults(t *testing.T) {
	app := New()
	app.Get("/a", func(c *Context) error { return nil }).Security("missing")
	_, err := app.OpenAPISpec(OpenAPIConfig{})
	if err == nil || !strings.Contains(err.Error(), `GET /a names security scheme "missing"`) {
		t.Fatalf("err = %v", err)
	}
	_, err = New().OpenAPISpec(OpenAPIConfig{Security: []string{"nope"}})
	if err == nil || !strings.Contains(err.Error(), `OpenAPIConfig.Security names security scheme "nope"`) {
		t.Fatalf("err = %v", err)
	}

	spec, err := New().OpenAPISpec(OpenAPIConfig{})
	if err != nil {
		t.Fatal(err)
	}
	// Under go test the main module is zinc itself, in development.
	for _, want := range []string{`"openapi": "3.1.0"`, `"title": "zinc"`, `"version": "0.0.0"`, `"paths": {}`} {
		if !bytes.Contains(spec, []byte(want)) {
			t.Fatalf("empty app spec lacks %s:\n%s", want, spec)
		}
	}
	if !bytes.Contains(spec, []byte(`"components": {}`)) {
		t.Fatalf("empty app has components:\n%s", spec)
	}
}

// Without a Validator nothing enforces validate tags, so their rules stay
// out of the spec; doc and example tags still apply.
func TestOpenAPIValidationRulesNeedAValidator(t *testing.T) {
	build := func(cfg Config) string {
		app := New(cfg)
		app.Post("/stores/{store}/pets", Typed(func(*Context, oaCreatePet) (oaPet, error) { return oaPet{}, nil }))
		app.Get("/pets", Typed(func(*Context, oaListPets) ([]oaPet, error) { return nil, nil }))
		spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
		if err != nil {
			t.Fatal(err)
		}
		return string(spec)
	}
	without, with := build(Config{}), build(Config{Validator: oaValidator{}})
	// The Error envelope's own required fields are a fact about the body, not
	// a validate rule, so check the pet schemas and the operations only.
	var doc struct {
		Components struct{ Schemas map[string]json.RawMessage }
		Paths      json.RawMessage
	}
	if err := json.Unmarshal([]byte(without), &doc); err != nil {
		t.Fatal(err)
	}
	// Request schemas: nothing required without a Validator. oaPet is a
	// response schema, so its required list comes from omitempty instead.
	for _, name := range []string{"oaCreatePetBody"} {
		if strings.Contains(string(doc.Components.Schemas[name]), `"required"`) {
			t.Errorf("without a Validator, %s has required fields: %s", name, doc.Components.Schemas[name])
		}
	}
	if !strings.Contains(string(doc.Components.Schemas["oaPet"]), `"required"`) {
		t.Errorf("the response schema oaPet lost its always-sent fields: %s", doc.Components.Schemas["oaPet"])
	}
	if strings.Contains(string(doc.Paths), `"required": true`) && strings.Count(string(doc.Paths), `"required": true`) != 1 {
		t.Errorf("without a Validator, more than the path parameter is required:\n%s", doc.Paths)
	}
	// oneof becomes an enum only with a validator; the enum tag and
	// EnumProvider always document their values.
	kindEnum := func(spec string) bool {
		var d struct {
			Components struct {
				Schemas map[string]struct {
					Properties map[string]map[string]any `json:"properties"`
				} `json:"schemas"`
			} `json:"components"`
		}
		_ = json.Unmarshal([]byte(spec), &d)
		return d.Components.Schemas["oaPet"].Properties["kind"]["enum"] != nil
	}
	if !kindEnum(with) || kindEnum(without) {
		t.Errorf("oaPet.kind's oneof: with a validator %v, without %v", kindEnum(with), kindEnum(without))
	}
	for _, rule := range []string{`"minLength"`, `"maxLength"`, `"minimum": 1`, `"422"`} {
		if !strings.Contains(with, rule) {
			t.Errorf("with a Validator, the spec lacks %s", rule)
		}
		if strings.Contains(without, rule) {
			t.Errorf("without a Validator, the spec claims %s", rule)
		}
	}
	// Header and path parameters are still listed, just not marked required
	// by a validate tag (path parameters are always required).
	if !strings.Contains(without, `"name": "X-Tenant"`) || !strings.Contains(without, `"description": "The pet's ID."`) {
		t.Fatalf("without a Validator, parameters or doc tags went missing:\n%s", without)
	}
}

// Every route documents a 500. The error body is Zinc's envelope only when
// the default error handler writes it.
func TestOpenAPIErrorResponses(t *testing.T) {
	custom := New(Config{ErrorHandler: func(c *Context, err error) { _ = c.Status(500).String("oops") }})
	custom.Get("/pets/{id}", Typed(func(*Context, oaPetID) (oaPet, error) { return oaPet{}, nil }))
	custom.Get("/health", func(c *Context) error { return nil })
	spec, err := custom.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(spec)
	if strings.Count(s, `"500": {`) != 2 || strings.Count(s, `"400": {`) != 1 {
		t.Fatalf("want a 500 on both routes and a 400 on the one with input:\n%s", s)
	}
	if strings.Contains(s, "#/components/schemas/Error") || strings.Contains(s, `"Error": {`) {
		t.Fatalf("a custom ErrorHandler's body was described as Zinc's envelope:\n%s", s)
	}

	def := New()
	def.Get("/health", func(c *Context) error { return nil })
	spec, _ = def.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if !strings.Contains(string(spec), `"$ref": "#/components/schemas/Error"`) {
		t.Fatalf("default handler: the 500 lacks the error envelope:\n%s", spec)
	}
}

// A plain handler's response is unknown unless the route describes it, so the
// spec says "default: any body" rather than inventing a 200.
func TestOpenAPIUndescribedResponses(t *testing.T) {
	app := New()
	plain := func(c *Context) error { return c.String("hi") }
	app.Get("/plain", plain)
	app.Post("/status", plain).Status(http.StatusCreated)
	app.Post("/declared", plain).Response(http.StatusCreated, oaPet{})
	app.Get("/output", plain).Output(oaPet{})
	responses := func(path, method string) map[string]any {
		var doc map[string]any
		spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
		if err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(spec, &doc)
		op := doc["paths"].(map[string]any)[path].(map[string]any)[method].(map[string]any)
		return op["responses"].(map[string]any)
	}
	keys := func(m map[string]any) string {
		out := slices.Sorted(maps.Keys(m))
		return strings.Join(out, " ")
	}
	for _, tt := range []struct{ path, method, want string }{
		{"/plain", "get", "500 default"},
		{"/status", "post", "201 500"},
		{"/declared", "post", "201 500"},
		{"/output", "get", "200 500"},
	} {
		if got := keys(responses(tt.path, tt.method)); got != tt.want {
			t.Errorf("%s %s: responses %s, want %s", tt.method, tt.path, got, tt.want)
		}
	}
	def := responses("/plain", "get")["default"].(map[string]any)
	if content := def["content"].(map[string]any); len(content) != 1 || content["*/*"] == nil {
		t.Fatalf("default content: %v", def)
	}
}

type oaOptionalQuery struct {
	Limit *int `query:"limit"`
}

type oaEither struct {
	Name string `json:"name" form:"name"`
}

type oaFormOnly struct {
	Name string `form:"name"`
}

func TestOpenAPIParametersAndBodies(t *testing.T) {
	app := New()
	app.Get("/q", Typed(func(*Context, oaOptionalQuery) (NoContent, error) { return NoContent{}, nil }))
	app.Post("/either", Typed(func(*Context, oaEither) (NoContent, error) { return NoContent{}, nil }))
	app.Post("/form", Typed(func(*Context, oaFormOnly) (NoContent, error) { return NoContent{}, nil }))
	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(spec)
	// A query value is absent or a number; it's never null.
	if !strings.Contains(s, `"name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",`) {
		t.Fatalf("pointer query parameter:\n%s", s)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]any `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}
	_ = json.Unmarshal(spec, &doc)
	media := func(path string) string {
		return strings.Join(slices.Sorted(maps.Keys(doc.Paths[path]["post"].RequestBody.Content)), " ")
	}
	// Binding decodes JSON into a form field too.
	if got := media("/either"); got != "application/json application/x-www-form-urlencoded" {
		t.Fatalf("/either accepts %s", got)
	}
	if got := media("/form"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("/form accepts %s", got)
	}
}

func TestOpenAPISecuritySchemeChecks(t *testing.T) {
	oauth := OpenAPISecurityScheme{Type: "oauth2", Flows: &OpenAPIOAuthFlows{
		ClientCredentials: &OpenAPIOAuthFlow{TokenURL: "https://id.example.com/token"},
	}}
	app := New()
	app.Get("/me", func(c *Context) error { return nil }).Security("oauth")
	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1", SecuritySchemes: map[string]OpenAPISecurityScheme{"oauth": oauth}})
	if err != nil {
		t.Fatal(err)
	}
	// OpenAPI requires the scopes object, even when there are none.
	if !strings.Contains(string(spec), `"clientCredentials": {
            "tokenUrl": "https://id.example.com/token",
            "scopes": {}`) {
		t.Fatalf("oauth2 flows:\n%s", spec)
	}
	if oauth.Flows.ClientCredentials.Scopes != nil {
		t.Fatal("the caller's flow was modified")
	}

	for _, tt := range []struct {
		scheme OpenAPISecurityScheme
		want   string
	}{
		{OpenAPISecurityScheme{Type: "oauth2"}, `type "oauth2" needs Flows`},
		{OpenAPISecurityScheme{Type: "oauth2", Flows: &OpenAPIOAuthFlows{AuthorizationCode: &OpenAPIOAuthFlow{TokenURL: "/t"}}}, "AuthorizationCode flow needs AuthorizationURL"},
		{OpenAPISecurityScheme{Type: "oauth2", Flows: &OpenAPIOAuthFlows{Password: &OpenAPIOAuthFlow{}}}, "Password flow needs TokenURL"},
		{OpenAPISecurityScheme{Type: "http"}, `type "http" needs Scheme`},
		{OpenAPISecurityScheme{Type: "apiKey", Name: "k", In: "body"}, `type "apiKey" needs Name, and In`},
		{OpenAPISecurityScheme{Type: "openIdConnect"}, "needs OpenIDConnectURL"},
		{OpenAPISecurityScheme{Type: "jwt"}, `unknown type "jwt"`},
	} {
		cfg := OpenAPIConfig{SecuritySchemes: map[string]OpenAPISecurityScheme{"s": tt.scheme}}
		if _, err := New().OpenAPISpec(cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%+v: err = %v, want %q", tt.scheme, err, tt.want)
		}
		mustPanicWith(t, tt.want, func() { New().OpenAPI("/openapi.json", cfg) })
	}
}

func mustPanicWith(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), want) {
			t.Errorf("panic = %v, want %q", r, want)
		}
	}()
	f()
}

func TestOpenAPISecurityRequirementsAndAuthResponses(t *testing.T) {
	cfg := OpenAPIConfig{
		Title: "T", Version: "1",
		SecuritySchemes: map[string]OpenAPISecurityScheme{
			"key":    {Type: "apiKey", In: "header", Name: "X-Key"},
			"bearer": {Type: "http", Scheme: "bearer"},
		},
	}
	app := New()
	app.Get("/either", func(c *Context) error { return nil }).Security("key", "bearer")
	app.Get("/both", func(c *Context) error { return nil }).SecurityAll("key", "bearer:admin")
	app.Get("/open", func(c *Context) error { return nil })
	spec := func(cfg OpenAPIConfig) map[string]any {
		t.Helper()
		raw, err := app.OpenAPISpec(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		_ = json.Unmarshal(raw, &doc)
		return doc
	}
	op := func(doc map[string]any, path string) map[string]any {
		return doc["paths"].(map[string]any)[path].(map[string]any)["get"].(map[string]any)
	}
	statuses := func(op map[string]any) string {
		return strings.Join(slices.Sorted(maps.Keys(op["responses"].(map[string]any))), " ")
	}
	doc := spec(cfg)
	if got := fmt.Sprint(op(doc, "/either")["security"]); got != "[map[key:[]] map[bearer:[]]]" {
		t.Errorf("either: %s", got)
	}
	if got := fmt.Sprint(op(doc, "/both")["security"]); got != "[map[bearer:[admin] key:[]]]" {
		t.Errorf("both: %s", got)
	}
	for path, want := range map[string]string{"/either": "401 500 default", "/both": "401 403 500 default", "/open": "500 default"} {
		if got := statuses(op(doc, path)); got != want {
			t.Errorf("%s: responses %s, want %s", path, got, want)
		}
	}
	cfg.NoAuthResponses = true
	if got := statuses(op(spec(cfg), "/both")); got != "500 default" {
		t.Errorf("NoAuthResponses: %s", got)
	}
	cfg.NoAuthResponses = false
	cfg.Security = []string{"key"}
	if got := statuses(op(spec(cfg), "/open")); got != "401 500 default" {
		t.Errorf("default security: %s", got)
	}
}

func TestOpenAPITagsAndInfo(t *testing.T) {
	app := New()
	app.Get("/a", func(c *Context) error { return nil }).Tags("zebra", "pets")
	app.Get("/b", func(c *Context) error { return nil }).Tags("apple")
	admin := app.Group("/admin").Tags("admin").Hidden()
	admin.Get("/x", func(c *Context) error { return nil })
	admin.Group("/deep").Get("/y", func(c *Context) error { return nil })
	raw, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1", Tags: []OpenAPITag{{Name: "pets", Description: "Pets."}}})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tags  []OpenAPITag   `json:"tags"`
		Paths map[string]any `json:"paths"`
	}
	_ = json.Unmarshal(raw, &doc)
	// Configured tags first, then the others in the order routes use them;
	// a hidden group's tags aren't listed.
	if got := fmt.Sprint(doc.Tags); got != "[{pets Pets. <nil>} {zebra  <nil>} {apple  <nil>}]" {
		t.Errorf("tags: %s", got)
	}
	if doc.Paths["/admin/x"] != nil || doc.Paths["/admin/deep/y"] != nil {
		t.Errorf("hidden group listed: %v", slices.Collect(maps.Keys(doc.Paths)))
	}

	for _, tt := range []struct {
		cfg  OpenAPIConfig
		want string
	}{
		{OpenAPIConfig{License: &OpenAPILicense{Identifier: "MIT"}}, "License needs Name"},
		{OpenAPIConfig{License: &OpenAPILicense{Name: "MIT", Identifier: "MIT", URL: "https://x"}}, "not both"},
		{OpenAPIConfig{ExternalDocs: &OpenAPIExternalDocs{}}, "ExternalDocs needs URL"},
		{OpenAPIConfig{Tags: []OpenAPITag{{Description: "x"}}}, "needs Name"},
		{OpenAPIConfig{Security: []string{"nope:read"}}, `security scheme "nope:read"`},
	} {
		if _, err := New().OpenAPISpec(tt.cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%+v: err = %v, want %q", tt.cfg, err, tt.want)
		}
		mustPanicWith(t, tt.want, func() { New().OpenAPI("/openapi.json", tt.cfg) })
	}
}

type Error struct {
	Reason string `json:"reason"`
}

// Zinc's error envelope keeps its name; a user type named Error gets a
// qualified one instead of being replaced.
func TestOpenAPIUserErrorType(t *testing.T) {
	app := New()
	app.Get("/v", Typed(func(*Context, struct{}) (Error, error) { return Error{}, nil }))
	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	_ = json.Unmarshal(spec, &doc)
	if !strings.Contains(string(doc.Components.Schemas["Error"]), `"error"`) {
		t.Fatalf("Error is not the envelope: %s", doc.Components.Schemas["Error"])
	}
	if !strings.Contains(string(doc.Components.Schemas["zinc.Error"]), `"reason"`) {
		t.Fatalf("the user's Error is missing: %v", slices.Collect(maps.Keys(doc.Components.Schemas)))
	}
	if !strings.Contains(string(spec), `"$ref": "#/components/schemas/zinc.Error"`) {
		t.Fatal("the response doesn't refer to the user's Error")
	}
	// With a custom error handler there's no envelope, so the name is free.
	custom := New(Config{ErrorHandler: TextErrors})
	custom.Get("/v", Typed(func(*Context, struct{}) (Error, error) { return Error{}, nil }))
	spec, _ = custom.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if !strings.Contains(string(spec), `"$ref": "#/components/schemas/Error"`) {
		t.Fatalf("without the envelope the user's Error should keep its name:\n%s", spec)
	}
}

func TestOpenAPIPathCollisions(t *testing.T) {
	h := func(c *Context) error { return nil }
	for _, tt := range []struct {
		name     string
		register func(app *App)
		want     string
	}{
		{"parameter and catch-all", func(app *App) {
			app.Get("/files/{path}", h)
			app.Get("/files/{path...}", h)
		}, "are the same OpenAPI operation, GET /files/{path}"},
		{"renamed parameter", func(app *App) {
			app.Get("/pets/{id}", h)
			app.Post("/pets/{name}", h)
		}, "paths /pets/{id} and /pets/{name} differ only in parameter names"},
		{"nested renamed parameters", func(app *App) {
			app.Get("/a/{x}/b/{y}", h)
			app.Get("/a/{p}/b/{q}/c", h)
			app.Put("/a/{p}/b/{q}", h)
		}, "paths /a/{x}/b/{y} and /a/{p}/b/{q} differ only in parameter names"},
	} {
		app := New()
		tt.register(app)
		if _, err := app.OpenAPISpec(OpenAPIConfig{}); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		}
	}

	// Hiding one of the two resolves it, as the error suggests.
	app := New()
	app.Get("/files/{path}", h)
	app.Get("/files/{path...}", h).Hidden()
	if _, err := app.OpenAPISpec(OpenAPIConfig{}); err != nil {
		t.Fatal(err)
	}
	// The same names on several methods are fine.
	app = New()
	app.Get("/pets/{id}", h)
	app.Put("/pets/{id}", h)
	app.Get("/pets/{id}/photo", h)
	if _, err := app.OpenAPISpec(OpenAPIConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestPathShape(t *testing.T) {
	for in, want := range map[string]string{
		"/":                  "/",
		"/pets":              "/pets",
		"/pets/{id}":         "/pets/{}",
		"/a/{x}/b/{y}/c":     "/a/{}/b/{}/c",
		"/v1/users:{action}": "/v1/users:{}",
		"/broken/{x":         "/broken/{x",
	} {
		if got := pathShape(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
