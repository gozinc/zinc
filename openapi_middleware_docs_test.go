// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type tokenHeaderInput struct {
	Token string `header:"X-Token"`
	Name  string `json:"name"`
}

func operation(doc map[string]any, method, path string) map[string]any {
	return doc["paths"].(map[string]any)[path].(map[string]any)[method].(map[string]any)
}

func headerParams(op map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	params, _ := op["parameters"].([]any)
	for _, p := range params {
		pm := p.(map[string]any)
		if pm["in"] == "header" {
			out[pm["name"].(string)] = pm
		}
	}
	return out
}

func statuses(op map[string]any) []string {
	var codes []string
	for code := range op["responses"].(map[string]any) {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes
}

// Middleware descriptions reach the routes they cover: app-wide ones every
// route, a group's its routes, a route's that route, each limited to its
// methods.
func TestMiddlewareDocs(t *testing.T) {
	noop := func(c *Context) error { return c.Next() }
	ok := func(c *Context) error { return c.NoContent() }
	csrfLike := MiddlewareDoc{
		Methods: []string{"post", http.MethodDelete},
		Headers: []HeaderDoc{{Name: "X-Token", Description: "The token.", Required: true}},
		Errors:  []int{http.StatusForbidden},
	}

	app := New()
	app.Use(noop)
	app.Document(MiddlewareDoc{Errors: []int{http.StatusServiceUnavailable}})
	api := app.Group("/api", noop).Document(csrfLike)
	api.Get("/things", ok)
	api.Post("/things", ok)
	api.Post("/bound", Typed(func(*Context, tokenHeaderInput) (NoContent, error) { return NoContent{}, nil }))
	app.Get("/health", ok)
	app.Get("/limited", noop, ok).Document(MiddlewareDoc{Errors: []int{http.StatusTooManyRequests}})
	doc := specDoc(t, app)

	get, post := operation(doc, "get", "/api/things"), operation(doc, "post", "/api/things")
	if h := headerParams(get); len(h) != 0 {
		t.Errorf("GET is outside the methods, but has headers %v", h)
	}
	if got := statuses(get); slices.Contains(got, "403") || !slices.Contains(got, "503") {
		t.Errorf("GET statuses %v: want the app's 503 and not the group's 403", got)
	}
	h := headerParams(post)["X-Token"]
	if h == nil || h["required"] != true || h["description"] != "The token." {
		t.Errorf("POST header: %v", headerParams(post))
	}
	if got := statuses(post); !slices.Contains(got, "403") || !slices.Contains(got, "503") {
		t.Errorf("POST statuses %v: want 403 and 503", got)
	}
	// A header the input already binds is described once, from the input.
	bound := headerParams(operation(doc, "post", "/api/bound"))
	if len(bound) != 1 || bound["X-Token"]["description"] != nil {
		t.Errorf("bound header: %v", bound)
	}
	if got := statuses(operation(doc, "get", "/health")); slices.Contains(got, "403") || !slices.Contains(got, "503") {
		t.Errorf("/health statuses %v", got)
	}
	if got := statuses(operation(doc, "get", "/limited")); !slices.Contains(got, "429") {
		t.Errorf("/limited statuses %v", got)
	}

	mustPanicWith(t, "Document error status 302 is not an error status", func() {
		New().Document(MiddlewareDoc{Errors: []int{http.StatusFound}})
	})
	mustPanicWith(t, "Document header needs a Name", func() {
		New().Group("/x").Document(MiddlewareDoc{Headers: []HeaderDoc{{Description: "?"}}})
	})
	mustPanicWith(t, "Document on group \"/x\" after", func() {
		g := New().Group("/x")
		g.Get("/", ok)
		g.Document(MiddlewareDoc{})
	})
}

// A middleware's credentials join each requirement on the methods it covers,
// reach the spec's schemes, and add no 401.
func TestMiddlewareDocSecurity(t *testing.T) {
	ok := func(c *Context) error { return c.NoContent() }
	token := OpenAPISecurityScheme{Type: "apiKey", In: "header", Name: "X-CSRF-Token"}
	csrfLike := MiddlewareDoc{Methods: []string{http.MethodPost}, Security: map[string]OpenAPISecurityScheme{"csrf": token}, Errors: []int{http.StatusForbidden}}
	cfg := OpenAPIConfig{Title: "T", Version: "1", SecuritySchemes: map[string]OpenAPISecurityScheme{
		"key":    {Type: "apiKey", In: "header", Name: "X-API-Key"},
		"bearer": {Type: "http", Scheme: "bearer"},
	}}
	app := New(Config{OpenAPI: cfg})
	api := app.Group("/api").Security("key", "bearer").Document(csrfLike)
	api.Get("/things", ok)
	api.Post("/things", ok)
	public := app.Group("/public").Security().Document(csrfLike)
	public.Post("/signup", ok)
	spec, err := app.OpenAPISpec(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Security  []map[string][]string `json:"security"`
			Responses map[string]any        `json:"responses"`
		} `json:"paths"`
		Components struct {
			SecuritySchemes map[string]map[string]any `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	get, post := doc.Paths["/api/things"]["get"], doc.Paths["/api/things"]["post"]
	if want := []map[string][]string{{"key": {}}, {"bearer": {}}}; !reflect.DeepEqual(get.Security, want) {
		t.Errorf("GET security %v, want %v", get.Security, want)
	}
	if want := []map[string][]string{{"key": {}, "csrf": {}}, {"bearer": {}, "csrf": {}}}; !reflect.DeepEqual(post.Security, want) {
		t.Errorf("POST security %v, want %v", post.Security, want)
	}
	signup := doc.Paths["/public/signup"]["post"]
	if want := []map[string][]string{{"csrf": {}}}; !reflect.DeepEqual(signup.Security, want) {
		t.Errorf("public POST security %v, want %v", signup.Security, want)
	}
	if _, ok := signup.Responses["401"]; ok {
		t.Error("a middleware's credentials added a 401")
	}
	if _, ok := signup.Responses["403"]; !ok {
		t.Error("the middleware's 403 is missing")
	}
	if doc.Components.SecuritySchemes["csrf"]["name"] != "X-CSRF-Token" || doc.Components.SecuritySchemes["key"] == nil {
		t.Errorf("schemes: %v", doc.Components.SecuritySchemes)
	}

	// A scheme that contradicts the config is an error, not a silent pick.
	clash := New()
	clash.Document(MiddlewareDoc{Security: map[string]OpenAPISecurityScheme{"key": token}})
	clash.Get("/", ok)
	if _, err := clash.OpenAPISpec(cfg); err == nil || !strings.Contains(err.Error(), `middleware security scheme "key" differs`) {
		t.Errorf("clash: %v", err)
	}
}
