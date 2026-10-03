// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type createPetInput struct {
	Name string `json:"name" validate:"required"`
}

type petRecord struct {
	ID   string  `json:"id"`
	Kind petKind `json:"kind"`
}

type petKind string

func (petKind) Enum() []any         { return []any{"cat", "dog"} }
func (petKind) OpenAPIName() string { return "PetKind" }

type updatePetBody struct {
	ID   string `path:"id"`
	Name string `json:"name" validate:"required"`
}

type taggedResponse struct {
	ID   string `json:"id"`
	ETag string `header:"ETag" json:"-"`
}

type headerInput struct {
	Token string `header:"X-CSRF-Token" validate:"required"`
}

func serveApp(app *App, method, target, body string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

func specDoc(t *testing.T, app *App) map[string]any {
	t.Helper()
	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func componentNames(doc map[string]any) []string {
	var names []string
	for name := range doc["components"].(map[string]any)["schemas"].(map[string]any) {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Default names start with a capital, as generated clients' types do, and
// never repeat the Input or Body suffix a type's name already has.
func TestSchemaDefaultNames(t *testing.T) {
	app := New()
	app.Post("/pets", Typed(func(*Context, createPetInput) (petRecord, error) { return petRecord{}, nil }))
	app.Put("/pets/{id}", Typed(func(*Context, updatePetBody) (petRecord, error) { return petRecord{}, nil }))
	got := componentNames(specDoc(t, app))
	want := []string{"CreatePetInput", "Error", "PetKind", "PetRecord", "UpdatePetBody"}
	if !slices.Equal(got, want) {
		t.Fatalf("components %v, want %v", got, want)
	}
}

// SchemaNamer names enum types as well as structs.
func TestSchemaNamerOnEnum(t *testing.T) {
	app := New()
	app.Get("/pets/{id}", Typed(func(*Context, struct{}) (petRecord, error) { return petRecord{}, nil }))
	doc := specDoc(t, app)
	pet := doc["components"].(map[string]any)["schemas"].(map[string]any)["PetRecord"].(map[string]any)
	kind := pet["properties"].(map[string]any)["kind"].(map[string]any)
	if kind["$ref"] != "#/components/schemas/PetKind" {
		t.Fatalf("kind: %v", kind)
	}
}

// Headers are spelled in the spec as the tag spells them, in requests and
// responses; net/http still sends its canonical form.
func TestHeaderSpelling(t *testing.T) {
	app := New()
	app.Get("/tagged", Typed(func(*Context, struct{}) (taggedResponse, error) {
		return taggedResponse{ID: "1", ETag: `"v1"`}, nil
	}))
	app.Post("/form", Typed(func(*Context, headerInput) (NoContent, error) { return NoContent{}, nil }))
	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"ETag": {`, `"name": "X-CSRF-Token"`} {
		if !strings.Contains(string(spec), want) {
			t.Errorf("spec lacks %s", want)
		}
	}
	for _, unwanted := range []string{`"Etag"`, `"X-Csrf-Token"`, `"x-csrf-token"`} {
		if strings.Contains(string(spec), unwanted) {
			t.Errorf("spec has %s", unwanted)
		}
	}
	w := serveApp(app, http.MethodGet, "/tagged", "")
	if w.Header().Get("ETag") != `"v1"` {
		t.Fatalf("ETag header: %v", w.Header())
	}
	if w := serveApp(app, http.MethodPost, "/form", "", "x-csrf-token", "abc"); w.Code != http.StatusNoContent {
		t.Fatalf("a header in any case binds: %d %s", w.Code, w.Body)
	}
}
