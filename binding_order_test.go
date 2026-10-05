// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// send runs one request against app and returns the response.
func sendBind(app *App, method, target, contentType, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

// A struct binds its path and query whatever the body's Content-Type; a
// text/plain body, which can't fill a struct, is refused unless it's empty.
func TestTextPlainKeepsURLBinding(t *testing.T) {
	type input struct {
		ID int    `path:"id"`
		Q  string `query:"q"`
	}
	app := New()
	var got input
	app.Post("/items/{id}", Typed(func(_ *Context, in input) (NoContent, error) { got = in; return NoContent{}, nil }))
	app.Post("/text", func(c *Context) error {
		var s string
		if err := c.Bind().All(&s); err != nil {
			return err
		}
		return c.String(s)
	})

	if w := sendBind(app, "POST", "/items/7?q=x", "text/plain", ""); w.Code != http.StatusNoContent || got.ID != 7 || got.Q != "x" {
		t.Fatalf("empty text body: %d %+v", w.Code, got)
	}
	if w := sendBind(app, "POST", "/items/7?q=x", "text/plain", "hello"); w.Code < 400 {
		t.Fatalf("text body into a struct: %d", w.Code)
	}
	if w := sendBind(app, "POST", "/text", "text/plain", "hello"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello") {
		t.Fatalf("text body into a string: %d %s", w.Code, w.Body)
	}
}

// A form field comes from the form body, never the query string.
func TestFormFieldsComeFromTheBody(t *testing.T) {
	type input struct {
		Name string `form:"name"`
	}
	app := New()
	var got input
	app.Post("/typed", Typed(func(_ *Context, in input) (NoContent, error) { got = in; return NoContent{}, nil }))
	app.Post("/form", func(c *Context) error {
		got = input{}
		if err := c.Bind().Form(&got); err != nil {
			return err
		}
		return c.NoContent()
	})
	for _, path := range []string{"/typed", "/form"} {
		got = input{}
		sendBind(app, "POST", path+"?name=fromquery", "application/x-www-form-urlencoded", "other=1")
		if got.Name != "" {
			t.Errorf("%s: the query filled a form field: %+v", path, got)
		}
		sendBind(app, "POST", path+"?name=fromquery", "application/x-www-form-urlencoded", "name=frombody")
		if got.Name != "frombody" {
			t.Errorf("%s: %+v", path, got)
		}
	}
}

// Media types are case-insensitive.
func TestMediaTypeCase(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	app := New()
	var got input
	app.Post("/", Typed(func(_ *Context, in input) (NoContent, error) { got = in; return NoContent{}, nil }))
	for _, ct := range []string{"Application/JSON", "APPLICATION/JSON; charset=utf-8"} {
		got = input{}
		if w := sendBind(app, "POST", "/", ct, `{"name":"Ada"}`); w.Code != http.StatusNoContent || got.Name != "Ada" {
			t.Errorf("%s: %d %+v", ct, w.Code, got)
		}
	}
}

// A default applies to a field whatever its source, when the request
// leaves it out.
func TestDefaultsApplyToEverySource(t *testing.T) {
	type input struct {
		Org   string `path:"org" default:"acme"`
		Count int    `json:"count" default:"5"`
		Limit int    `query:"limit" default:"20"`
	}
	app := New()
	var got input
	app.Post("/items", Typed(func(_ *Context, in input) (NoContent, error) { got = in; return NoContent{}, nil }))
	app.Post("/orgs/{org}/items", Typed(func(_ *Context, in input) (NoContent, error) { got = in; return NoContent{}, nil }))
	for _, tt := range []struct {
		target, body string
		want         input
	}{
		{"/items", `{}`, input{Org: "acme", Count: 5, Limit: 20}},
		{"/items", `{"name":"x"}`, input{Org: "acme", Count: 5, Limit: 20}},
		{"/items?limit=3", `{"count":7}`, input{Org: "acme", Count: 7, Limit: 3}},
		{"/items", `{"count":0}`, input{Org: "acme", Count: 0, Limit: 20}},
		{"/orgs/zinc/items", `{}`, input{Org: "zinc", Count: 5, Limit: 20}},
	} {
		got = input{}
		if w := sendBind(app, "POST", tt.target, "application/json", tt.body); w.Code != http.StatusNoContent || got != tt.want {
			t.Errorf("%s %s: %d %+v, want %+v", tt.target, tt.body, w.Code, got, tt.want)
		}
	}
}

// The error for a field binding can't fill gives advice that fits it.
func TestUnsupportedFieldAdvice(t *testing.T) {
	type pointers struct {
		IDs []*int `query:"id"`
	}
	type mapped struct {
		M map[string]string `query:"m"`
	}
	if err := bindingPlanFor(reflect.TypeFor[pointers]()).err; err == nil || !strings.Contains(err.Error(), "not a pointer") {
		t.Errorf("[]*int: %v", err)
	}
	if err := bindingPlanFor(reflect.TypeFor[mapped]()).err; err == nil || !strings.Contains(err.Error(), "a pointer to one") {
		t.Errorf("map: %v", err)
	}
}
