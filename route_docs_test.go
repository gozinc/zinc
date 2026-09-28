// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type docUser struct {
	ID string `json:"id"`
}

// docTwin has docUser's layout, so their Typed closures share a code pointer.
type docTwin struct {
	ID string `json:"id"`
}

type docCreate struct {
	Org  string `path:"org"`
	Name string `json:"name"`
}

func docOf(t *testing.T, r Route) *routeDoc {
	t.Helper()
	return r.table.routeDocs[r.index]
}

func TestTypedRoutesCarryTheirTypes(t *testing.T) {
	app := New()
	passthrough := func(c *Context) error { return c.Next() }
	api := app.Group("/api", passthrough)

	create := api.Post("/orgs/{org}/users", Typed(func(_ *Context, in docCreate) (docUser, error) {
		return docUser{ID: in.Org + "/" + in.Name}, nil
	}))
	twin := app.Get("/twin", Typed(func(*Context, docTwin) (docTwin, error) { return docTwin{}, nil }))
	ptr := app.Get("/ptr", Typed(func(*Context, struct{}) (*docUser, error) { return nil, nil }))

	for _, tt := range []struct {
		route   Route
		in, out reflect.Type
	}{
		{create, reflect.TypeFor[docCreate](), reflect.TypeFor[docUser]()},
		{twin, reflect.TypeFor[docTwin](), reflect.TypeFor[docTwin]()},
		{ptr, reflect.TypeFor[struct{}](), reflect.TypeFor[*docUser]()},
	} {
		doc := docOf(t, tt.route)
		if doc == nil || !doc.typed || doc.in != tt.in || doc.out != tt.out {
			t.Fatalf("route %d: doc %+v, want %v → %v", tt.route.index, doc, tt.in, tt.out)
		}
	}

	// A described handler still serves requests normally.
	r := httptest.NewRequest("POST", "/api/orgs/acme/users", strings.NewReader(`{"name":"ada"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"id":"acme/ada"}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPlainHandlersAreNeverCalledAtRegistration(t *testing.T) {
	app := New()
	calls := 0
	plain := app.Get("/plain", func(c *Context) error { calls++; return c.String("ok") })
	typed := Typed(func(*Context, docUser) (docUser, error) { return docUser{}, nil })
	wrapped := app.Get("/wrapped", func(c *Context) error { calls++; return typed(c) })
	if calls != 0 {
		t.Fatalf("a plain handler ran %d times at registration", calls)
	}
	if docOf(t, plain) != nil || docOf(t, wrapped) != nil {
		t.Fatal("plain routes got metadata without asking for it")
	}
}

func TestRouteDocMethods(t *testing.T) {
	app := New()
	type conflict struct {
		Reason string `json:"reason"`
	}
	r := app.Put("/users/{id}", func(c *Context) error { return nil }).
		Input(docCreate{}).
		Output(docUser{}).
		Summary("Update a user").
		Description("Replaces the **whole** user.").
		Tags("users", "admin", "users").
		Deprecated().
		Response(409, conflict{}).
		Response(409, nil). // replaces the first 409
		Response(204, nil).
		Security("apiKey")
	doc := docOf(t, r)
	if doc.typed || doc.in != reflect.TypeFor[docCreate]() || doc.out != reflect.TypeFor[docUser]() {
		t.Fatalf("types: %+v", doc)
	}
	if doc.summary != "Update a user" || doc.description != "Replaces the **whole** user." || !doc.deprecated || doc.hidden {
		t.Fatalf("text and flags: %+v", doc)
	}
	if !reflect.DeepEqual(doc.tags, []string{"users", "admin"}) {
		t.Fatalf("tags %v", doc.tags)
	}
	if len(doc.responses) != 2 || doc.responses[0] != (docResponse{409, nil}) || doc.responses[1] != (docResponse{204, nil}) {
		t.Fatalf("responses %v", doc.responses)
	}
	if !doc.securitySet || !reflect.DeepEqual(doc.security, []string{"apiKey"}) {
		t.Fatalf("security %v", doc.security)
	}
	if hidden := app.Get("/internal", func(*Context) error { return nil }).Hidden(); !docOf(t, hidden).hidden {
		t.Fatal("Hidden not recorded")
	}
}

func TestRouteDocMethodsPanicOnMisuse(t *testing.T) {
	app := New()
	typed := app.Post("/t", Typed(func(*Context, docUser) (docUser, error) { return docUser{}, nil }))
	plain := app.Get("/p", func(*Context) error { return nil })

	mustPanic(t, "Input on a Typed route", func() { typed.Input(docUser{}) })
	mustPanic(t, "Output on a Typed route", func() { typed.Output(docUser{}) })
	mustPanic(t, "Input needs a value", func() { plain.Input(nil) })
	mustPanic(t, "Response status 42", func() { plain.Response(42, nil) })
	mustPanic(t, "Summary on a route that was not registered", func() { Route{}.Summary("x") })
}

func TestGroupDocDefaults(t *testing.T) {
	app := New()
	admin := app.Group("/admin").Tags("admin").Security("apiKey")
	users := admin.Group("/users").Tags("users")
	list := users.Get("", func(*Context) error { return nil }).Tags("listing")
	health := admin.Get("/health", func(*Context) error { return nil }).Security() // public
	root := app.Get("/", func(*Context) error { return nil })

	if doc := docOf(t, list); !reflect.DeepEqual(doc.tags, []string{"admin", "users", "listing"}) || !reflect.DeepEqual(doc.security, []string{"apiKey"}) {
		t.Fatalf("nested group route: tags %v security %v", doc.tags, doc.security)
	}
	if doc := docOf(t, health); !doc.securitySet || len(doc.security) != 0 {
		t.Fatalf("route override to public: %+v", doc)
	}
	if docOf(t, root) != nil {
		t.Fatal("a route outside the group got its defaults")
	}

	mustPanic(t, `Tags on group "/admin" after child group /admin/users`, func() { admin.Tags("late") })
	mustPanic(t, `Security on group "/admin/users" after route GET /admin/users`, func() { users.Security("late") })
}
