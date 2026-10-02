// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type vtSize string

func (vtSize) Enum() []any { return []any{vtSize("s"), vtSize("m")} }

type vtOwner struct {
	Email string `json:"email" validate:"required,email"`
}

type vtPet struct {
	Name    string             `json:"name" validate:"required,min=2,max=5"`
	Age     int                `json:"age" validate:"omitempty,gte=1,lt=30"`
	Kind    string             `json:"kind" validate:"omitempty,oneof=cat dog"`
	Tags    []string           `json:"tags" validate:"omitempty,max=2"`
	ID      string             `json:"id" validate:"omitempty,uuid"`
	Site    string             `json:"site" validate:"omitempty,http_url"`
	Color   string             `json:"color" enum:"red,blue"`
	Size    vtSize             `json:"size"`
	Owner   *vtOwner           `json:"owner"`
	Friends []vtOwner          `json:"friends"`
	ByName  map[string]vtOwner `json:"by_name"`
	Limit   int                `query:"limit" validate:"min=1" default:"10"`
}

func TestBuiltinValidator(t *testing.T) {
	app := New()
	app.Post("/pets", Typed(func(_ *Context, in vtPet) (NoContent, error) { return NoContent{}, nil }))
	send := func(body string) (int, map[string]string) {
		t.Helper()
		r := httptest.NewRequest("POST", "/pets", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		var out struct {
			Error struct{ Fields map[string]string } `json:"error"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Error.Fields
	}
	if code, fields := send(`{"name":"Tom"}`); code != 204 {
		t.Fatalf("valid: %d %v", code, fields)
	}
	for _, tt := range []struct{ body, field, msg string }{
		{`{}`, "name", "is required"},
		{`{"name":"T"}`, "name", "must be at least 2 characters"},
		{`{"name":"Tomasz"}`, "name", "must be at most 5 characters"},
		{`{"name":"Tom","age":30}`, "age", "must be less than 30"},
		{`{"name":"Tom","kind":"cow"}`, "kind", "must be one of: cat, dog"},
		{`{"name":"Tom","tags":["a","b","c"]}`, "tags", "must have at most 2 items"},
		{`{"name":"Tom","id":"nope"}`, "id", "must be a UUID"},
		{`{"name":"Tom","site":"ftp://x"}`, "site", "must be an http or https URL"},
		{`{"name":"Tom","color":"green"}`, "color", "must be one of: red, blue"},
		{`{"name":"Tom","size":"xl"}`, "size", "must be one of: s, m"},
		{`{"name":"Tom","owner":{"email":"x"}}`, "owner.email", "must be an email address"},
		{`{"name":"Tom","friends":[{"email":"a@b.co"},{}]}`, "friends[1].email", "is required"},
		{`{"name":"Tom","by_name":{"ann":{"email":"bad"}}}`, "by_name[ann].email", "must be an email address"},
	} {
		code, fields := send(tt.body)
		if code != 422 || fields[tt.field] != tt.msg {
			t.Errorf("%s: %d %v, want %s %q", tt.body, code, fields, tt.field, tt.msg)
		}
	}
	// Plain handlers get the same checks through binding.
	app.Post("/plain", func(c *Context) error {
		var in vtOwner
		return c.Bind().JSON(&in)
	})
	r := httptest.NewRequest("POST", "/plain", strings.NewReader(`{"email":"nope"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 422 {
		t.Errorf("plain: %d %s", w.Code, w.Body)
	}
}

type vtSilent struct{ calls *int }

func (v vtSilent) Validate(any) error { *v.calls++; return nil }

type vtDeclared struct{}

func (vtDeclared) Validate(any) error { return errors.New("refused") }
func (vtDeclared) RuleSet() []string  { return []string{"required"} }

func TestValidatorChoices(t *testing.T) {
	type in struct {
		Name  string `json:"name" validate:"max=2"`
		Color string `json:"color" enum:"red"`
	}
	post := func(app *App, body string) int {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w.Code
	}
	// A Validator replaces the built-in rules, but enum values are Zinc's own
	// claims, so they're still checked.
	calls := 0
	app := New(Config{Validator: vtSilent{&calls}})
	app.Post("/", Typed(func(*Context, in) (NoContent, error) { return NoContent{}, nil }))
	if code := post(app, `{"name":"long"}`); code != 204 || calls != 1 {
		t.Errorf("silent validator: %d, %d calls", code, calls)
	}
	if code := post(app, `{"color":"blue"}`); code != 422 {
		t.Errorf("enum with a Validator: %d", code)
	}
	// A declared rule set that lacks a tag's rule fails at registration.
	mustPanicWith(t, "the Validator (zinc.vtDeclared) doesn't enforce these validate rules: zinc.in.Name: max=2", func() {
		New(Config{Validator: vtDeclared{}}).Post("/", Typed(func(*Context, in) (NoContent, error) { return NoContent{}, nil }))
	})
	// A rule Zinc doesn't have: a typed handler fails at registration, and a
	// plain handler's binding fails with 500 rather than skip it.
	type alnum struct {
		Code string `json:"code" validate:"alphanum"`
	}
	mustPanicWith(t, "Zinc's built-in validator doesn't enforce these validate rules: zinc.alnum.Code: alphanum", func() {
		New().Post("/", Typed(func(*Context, alnum) (NoContent, error) { return NoContent{}, nil }))
	})
	plain := New()
	plain.Post("/", func(c *Context) error { var v alnum; return c.Bind().JSON(&v) })
	if code := post(plain, `{"code":"x"}`); code != 500 {
		t.Errorf("unsupported rule in a plain handler: %d", code)
	}
}

func TestValidateResponses(t *testing.T) {
	type out struct {
		Name string `json:"name" validate:"required"`
	}
	for _, on := range []bool{false, true} {
		app := New(Config{ValidateResponses: on})
		app.Get("/", Typed(func(*Context, struct{}) ([]out, error) { return []out{{"a"}, {}}, nil }))
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		want := http.StatusOK
		if on {
			want = http.StatusInternalServerError
		}
		if w.Code != want {
			t.Errorf("ValidateResponses %v: %d %s", on, w.Code, w.Body)
		}
	}
	var got error
	app := New(Config{ValidateResponses: true, ErrorHandler: func(c *Context, err error) { got = err; DefaultErrorHandler(c, err) }})
	app.Get("/pets/{id}", Typed(func(*Context, struct{}) (out, error) { return out{}, nil }))
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/pets/1", nil))
	if got == nil || got.Error() != "zinc: GET /pets/{id}: the response breaks its contract: response.name is required" {
		t.Errorf("error: %v", got)
	}
}

func TestAppValidate(t *testing.T) {
	type out struct {
		Location string `header:"Location"`
		Name     string `json:"name" openapi:"readonly,secret"`
	}
	type echo struct {
		Region string `header:"X-Region" json:"region"`
	}
	type plainIn struct {
		Code string `json:"code" validate:"alphanum"`
	}
	app := New()
	app.Get("/out", Typed(func(*Context, struct{}) (out, error) { return out{}, nil }))
	app.Get("/echo", Typed(func(_ *Context, in echo) (echo, error) { return in, nil }))
	app.Post("/plain", func(c *Context) error { return nil }).Input(plainIn{})
	app.Get("/a/{id}", func(c *Context) error { return nil })
	app.Get("/a/{name}/x", func(c *Context) error { return nil })
	app.Post("/a/{name}", func(c *Context) error { return nil })
	err := app.Validate()
	if err == nil {
		t.Fatal("no problems found")
	}
	for _, want := range []string{
		`GET /out: zinc.out.Location has header:"Location" but is sent in the JSON body`,
		`GET /out: zinc.out.Name: unknown openapi tag word "secret"`,
		`POST /plain: zinc: Zinc's built-in validator doesn't enforce these validate rules: zinc.plainIn.Code: alphanum`,
		`differ only in parameter names`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "echo") {
		t.Errorf("an echoed input's header field was reported:\n%v", err)
	}
	clean := New()
	clean.Get("/ok", Typed(func(*Context, struct{}) (echo, error) { return echo{}, nil }))
	if err := clean.Validate(); err == nil || !strings.Contains(err.Error(), "X-Region") {
		t.Errorf("a non-echoed output with a body header field: %v", err)
	}
	if err := New().Validate(); err != nil {
		t.Errorf("empty app: %v", err)
	}
}
