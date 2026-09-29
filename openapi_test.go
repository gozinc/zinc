// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
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
