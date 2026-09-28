// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding/json"
	"flag"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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
	Page  int      `query:"page" validate:"min=1"`
	Kinds []string `query:"kind"`
}

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
	app.Get("/pets/{id}", Typed(func(*Context, oaPetID) (*oaPet, error) { return nil, nil })).Tags("pets")
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

	// Left out: hidden routes, methods OpenAPI 3.1 has no field for, mounts.
	app.Get("/internal/metrics", func(c *Context) error { return nil }).Hidden()
	app.Add("PURGE", "/cache", func(c *Context) error { return nil })
	app.Mount("/legacy", http.NotFoundHandler())
	return app
}

func oaFixtureConfig() OpenAPIConfig {
	return OpenAPIConfig{
		Title:       "Pet Store",
		Version:     "1.2.0",
		Description: "The fixture for Zinc's OpenAPI golden test.",
		Servers:     []OpenAPIServer{{URL: "https://api.example.com", Description: "Production"}},
		SecuritySchemes: map[string]OpenAPISecurityScheme{
			"apiKey": {Type: "apiKey", In: "header", Name: "X-API-Key"},
			"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
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
	for _, name := range []string{"oaPet", "oaCreatePetBody"} {
		if strings.Contains(string(doc.Components.Schemas[name]), `"required"`) {
			t.Errorf("without a Validator, %s has required fields: %s", name, doc.Components.Schemas[name])
		}
	}
	if strings.Contains(string(doc.Paths), `"required": true`) && strings.Count(string(doc.Paths), `"required": true`) != 1 {
		t.Errorf("without a Validator, more than the path parameter is required:\n%s", doc.Paths)
	}
	for _, rule := range []string{`"minLength"`, `"maxLength"`, `"enum"`, `"minimum": 1`, `"422"`} {
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
