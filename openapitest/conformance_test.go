// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The conformance app answers real requests; each response body must validate
// against the schema its spec documents for that status. The spec says what
// the server sends, and this checks it.

type owner struct {
	Email string `json:"email" validate:"required,email"`
}

type pet struct {
	ID       int64             `json:"id" doc:"The pet's ID."`
	Name     string            `json:"name" validate:"required,min=1,max=40"`
	Kind     string            `json:"kind" validate:"oneof=cat dog"`
	Owner    *owner            `json:"owner"`
	Tags     []string          `json:"tags"`
	Born     time.Time         `json:"born"`
	Weight   float64           `json:"weight"`
	Chipped  bool              `json:"chipped"`
	Extra    map[string]string `json:"extra,omitempty"`
	Parent   *pet              `json:"parent,omitempty"`
	Internal string            `json:"-"`
}

type createPet struct {
	Store string `path:"store"`
	Name  string `json:"name" validate:"required,min=1,max=40"`
	Kind  string `json:"kind" validate:"omitempty,oneof=cat dog"`
}

type petID struct {
	ID int64 `path:"id"`
}

type listPets struct {
	Kind string `query:"kind"`
}

type conflict struct {
	Reason string `json:"reason"`
}

func conformanceApp() *zinc.App {
	// No Validator: Zinc enforces createPet's tags itself, so 422s are real.
	app := zinc.New()
	full := pet{ID: 7, Name: "Rex", Kind: "dog", Owner: &owner{Email: "ada@example.com"}, Tags: []string{"good"},
		Born: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), Weight: 12.5, Chipped: true,
		Extra: map[string]string{"colour": "brown"}, Parent: &pet{ID: 1, Name: "Max", Kind: "dog", Tags: []string{}}}

	app.Post("/stores/{store}/pets", zinc.Typed(func(c *zinc.Context, in createPet) (pet, error) {
		switch in.Name {
		case "Taken": // the handler writes its own body: Response
			return pet{}, c.Status(http.StatusConflict).JSON(conflict{Reason: "name taken"})
		case "Closed": // an error through the error handler: Errors
			return pet{}, zinc.NewError(http.StatusForbidden, "store closed")
		}
		return pet{ID: 8, Name: in.Name, Kind: in.Kind, Tags: []string{}}, nil
	})).Status(http.StatusCreated).Response(http.StatusConflict, conflict{}).Errors(http.StatusForbidden).Name("createPet")

	app.Get("/pets/{id}", zinc.Typed(func(_ *zinc.Context, in petID) (*pet, error) {
		switch in.ID {
		case 7:
			return &full, nil
		case 500:
			return nil, errors.New("database down")
		case 404:
			return nil, zinc.NotFound("no such pet")
		}
		return nil, nil // documented: the output is *pet, so null is allowed
	})).Errors(http.StatusNotFound).Name("getPet")
	app.Get("/pets", zinc.Typed(func(_ *zinc.Context, in listPets) ([]pet, error) {
		return []pet{full}, nil
	})).Name("listPets")
	app.Delete("/pets/{id}", zinc.Typed(func(_ *zinc.Context, in petID) (zinc.NoContent, error) {
		return zinc.NoContent{}, nil
	})).Name("deletePet")
	app.Put("/pets/{id}", func(c *zinc.Context) error {
		var in createPet
		if err := c.Bind().All(&in); err != nil {
			return err
		}
		return c.JSON(pet{ID: 7, Name: in.Name, Kind: in.Kind, Tags: []string{}})
	}).Input(createPet{}).Output(pet{}).Name("replacePet")
	return app
}

type exchange struct {
	method, target, body string
	status               int
}

func TestResponsesMatchTheSpec(t *testing.T) {
	app := conformanceApp()
	spec, err := app.OpenAPISpec(zinc.OpenAPIConfig{Title: "Conformance", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(spec))
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://zinc.test/conformance.json"
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	paths := doc.(map[string]any)["paths"].(map[string]any)

	for _, ex := range []exchange{
		{"POST", "/stores/main/pets", `{"name":"Rex","kind":"dog"}`, 201},
		{"POST", "/stores/main/pets", `{"name":"Taken","kind":"dog"}`, 409},
		{"POST", "/stores/main/pets", `{"name":"Closed","kind":"dog"}`, 403},
		{"POST", "/stores/main/pets", `{"name":""}`, 422},
		{"POST", "/stores/main/pets", `{"name":`, 400},
		{"GET", "/pets/7", "", 200},
		{"GET", "/pets/9", "", 200}, // a nil *pet is sent as null
		{"GET", "/pets/404", "", 404},
		{"GET", "/pets/500", "", 500},
		{"GET", "/pets/abc", "", 400},
		{"GET", "/pets?kind=dog", "", 200},
		{"DELETE", "/pets/7", "", 204},
		{"PUT", "/pets/7", `{"name":"Rex","kind":"cat"}`, 200},
	} {
		r := httptest.NewRequest(ex.method, ex.target, strings.NewReader(ex.body))
		if ex.body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		name := ex.method + " " + ex.target
		if w.Code != ex.status {
			t.Errorf("%s: status %d, want %d: %s", name, w.Code, ex.status, w.Body.String())
			continue
		}

		// Find the documented response for this route and status.
		path := r.URL.Path
		route := ""
		for p := range paths {
			if matches(p, path) {
				route = p
			}
		}
		op, _ := paths[route].(map[string]any)[strings.ToLower(ex.method)].(map[string]any)
		if op == nil {
			t.Errorf("%s: no operation in the spec", name)
			continue
		}
		resp, ok := op["responses"].(map[string]any)[strconv.Itoa(w.Code)].(map[string]any)
		if !ok {
			t.Errorf("%s: status %d isn't documented", name, w.Code)
			continue
		}
		content, hasContent := resp["content"].(map[string]any)
		if w.Body.Len() == 0 {
			if hasContent {
				t.Errorf("%s: documented a body, sent none", name)
			}
			continue
		}
		if !hasContent {
			t.Errorf("%s: sent a body, documented none: %s", name, w.Body.String())
			continue
		}
		if _, ok := content["application/json"]; !ok {
			t.Errorf("%s: body not documented as application/json", name)
			continue
		}
		ptr := "#/paths/" + escape(route) + "/" + strings.ToLower(ex.method) + "/responses/" + strconv.Itoa(w.Code) + "/content/application~1json/schema"
		sch, err := c.Compile(url + ptr)
		if err != nil {
			t.Errorf("%s: compiling %s: %v", name, ptr, err)
			continue
		}
		var body any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: body isn't JSON: %v", name, err)
			continue
		}
		if err := sch.Validate(body); err != nil {
			t.Errorf("%s: body doesn't match its schema:\n%s\n%v", name, w.Body.String(), err)
		}
	}
}

// matches reports whether an OpenAPI path template matches a request path.
func matches(template, path string) bool {
	ts, ps := strings.Split(template, "/"), strings.Split(path, "/")
	if len(ts) != len(ps) {
		return false
	}
	for i := range ts {
		if !strings.HasPrefix(ts[i], "{") && ts[i] != ps[i] {
			return false
		}
	}
	return true
}

func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

// TestConformanceCatchesMismatches proves the check above can fail: bodies
// that break the documented pet schema are rejected.
func TestConformanceCatchesMismatches(t *testing.T) {
	spec, err := conformanceApp().OpenAPISpec(zinc.OpenAPIConfig{Title: "Conformance", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := jsonschema.UnmarshalJSON(bytes.NewReader(spec))
	const url = "https://zinc.test/conformance.json"
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(url + "#/components/schemas/pet")
	if err != nil {
		t.Fatal(err)
	}
	good := `{"id":1,"name":"Rex","kind":"dog","owner":null,"tags":[],"born":"2020-01-02T03:04:05Z","weight":1,"chipped":false}`
	var v any
	_ = json.Unmarshal([]byte(good), &v)
	if err := sch.Validate(v); err != nil {
		t.Fatalf("a valid pet was rejected: %v", err)
	}
	for name, bad := range map[string]string{
		"id as a string":  `{"id":"1","name":"Rex"}`,
		"no name":         `{"id":1}`,
		"empty name":      `{"id":1,"name":""}`,
		"unknown kind":    `{"id":1,"name":"Rex","kind":"fish"}`,
		"bad email":       `{"id":1,"name":"Rex","owner":{"email":"nope"}}`,
		"born not a date": `{"id":1,"name":"Rex","born":"yesterday"}`,
	} {
		_ = json.Unmarshal([]byte(bad), &v)
		if sch.Validate(v) == nil {
			t.Errorf("%s: accepted %s", name, bad)
		}
	}
}
