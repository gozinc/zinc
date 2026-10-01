// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type Paging struct {
	Limit int `query:"limit" default:"20"`
	Sort  string
}

type tenancy struct {
	Tenant string `header:"X-Tenant"`
}

type deep struct {
	*Paging
}

type unexportedPtr struct {
	*tenancy
	Name string `query:"name"`
}

// Tagged fields of embedded structs bind, as Go promotes them: by value,
// through an exported pointer allocated only when a value arrives, and
// through more than one level.
func TestBindingPromotesEmbeddedFields(t *testing.T) {
	type input struct {
		deep
		tenancy
		Name string `query:"name"`
	}
	var got input
	app := New()
	app.Get("/", Typed(func(_ *Context, in input) (NoContent, error) { got = in; return NoContent{}, nil }))
	serveTest := func(target string, header ...string) {
		t.Helper()
		r := httptest.NewRequest("GET", target, nil)
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body)
		}
	}
	serveTest("/?limit=7&name=a", "X-Tenant", "acme")
	if got.Paging == nil || got.Limit != 7 || got.Tenant != "acme" || got.Name != "a" {
		t.Fatalf("got %+v (paging %+v)", got, got.Paging)
	}
	serveTest("/?name=b")
	// The default fills the promoted field, allocating its embedded pointer.
	if got.Paging == nil || got.Limit != 20 {
		t.Fatalf("default: %+v", got.Paging)
	}

	// The spec lists the promoted parameters.
	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name": "limit"`, `"name": "X-Tenant"`, `"default": 20`} {
		if !strings.Contains(string(spec), want) {
			t.Errorf("spec lacks %s", want)
		}
	}
}

func TestBindingRejectsWhatItCantFill(t *testing.T) {
	mustPanicWith(t, "embeds *tenancy, whose tagged fields binding can't fill", func() {
		New().Get("/", Typed(func(*Context, unexportedPtr) (NoContent, error) { return NoContent{}, nil }))
	})
	type mapped struct {
		Options map[string]string `query:"options"`
	}
	mustPanicWith(t, "Options has a binding tag, but binding can't fill a map[string]string", func() {
		New().Get("/", Typed(func(*Context, mapped) (NoContent, error) { return NoContent{}, nil }))
	})
	// A plain handler's binder returns the same error.
	app := New()
	app.Get("/", func(c *Context) error {
		var in mapped
		return c.Bind().Query(&in)
	})
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("GET", "/?options=x", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("plain binder: %d", w.Code)
	}
}

// A typed route's declared status is the default: an error, or a status the
// handler sets, wins over it.
func TestTypedStatusPrecedence(t *testing.T) {
	app := New()
	app.Post("/err", Typed(func(*Context, struct{}) (string, error) { return "", errors.New("boom") })).Status(http.StatusCreated)
	app.Post("/missing", Typed(func(*Context, struct{}) (string, error) { return "", NotFound("no") })).Status(http.StatusCreated)
	app.Post("/declared", Typed(func(*Context, struct{}) (string, error) { return "ok", nil })).Status(http.StatusCreated)
	app.Delete("/gone", Typed(func(*Context, struct{}) (NoContent, error) { return NoContent{}, nil }))
	app.Delete("/accepted", Typed(func(c *Context, _ struct{}) (NoContent, error) {
		c.Status(http.StatusAccepted)
		return NoContent{}, nil
	}))
	for target, want := range map[string]int{"POST /err": 500, "POST /missing": 404, "POST /declared": 201, "DELETE /gone": 204, "DELETE /accepted": 202} {
		method, path, _ := strings.Cut(target, " ")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != want {
			t.Errorf("%s: %d, want %d", target, w.Code, want)
		}
	}
}
