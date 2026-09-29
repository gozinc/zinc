// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func getSpec(t *testing.T, app *App, method, target string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

func TestOpenAPIServesTheSpec(t *testing.T) {
	app := oaFixture()
	cfg := oaFixtureConfig()
	app.OpenAPI("/openapi.json", cfg)

	w := getSpec(t, app, "GET", "/openapi.json", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != MIMEJSON {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Content-Type"))
	}
	want, err := app.OpenAPISpec(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.Body.Bytes(), want) {
		t.Fatal("served spec differs from OpenAPISpec")
	}
	if strings.Contains(w.Body.String(), `"/openapi.json"`) {
		t.Fatal("the spec route lists itself")
	}

	// HEAD gets the headers without the body.
	if head := getSpec(t, app, "HEAD", "/openapi.json", nil); head.Code != 200 || head.Body.Len() != 0 {
		t.Fatalf("HEAD: %d, %d bytes", head.Code, head.Body.Len())
	}
}

func TestOpenAPICachesAndRebuilds(t *testing.T) {
	app := New()
	app.Get("/a", func(c *Context) error { return nil })
	route := app.OpenAPI("/openapi.json", OpenAPIConfig{Title: "T", Version: "1"})
	if doc := app.router.routeDocs[route.index]; doc == nil || !doc.hidden {
		t.Fatal("the spec route isn't hidden")
	}

	served := &servedSpec{app: app, cfg: OpenAPIConfig{Title: "T", Version: "1"}}
	first, err := served.bytes()
	if err != nil {
		t.Fatal(err)
	}
	cached := served.cached.Load()
	if again, _ := served.bytes(); served.cached.Load() != cached || !bytes.Equal(again, first) {
		t.Fatal("an unchanged app rebuilt the spec")
	}

	app.Get("/b", func(c *Context) error { return nil })
	second, _ := served.bytes()
	if served.cached.Load() == cached || bytes.Contains(first, []byte(`"/b"`)) || !bytes.Contains(second, []byte(`"/b"`)) {
		t.Fatalf("a route added after the first build isn't in the next spec:\n%s", second)
	}

	// Through the route too.
	if body := getSpec(t, app, "GET", "/openapi.json", nil).Body.String(); !strings.Contains(body, `"/b"`) {
		t.Fatalf("served spec lacks /b:\n%s", body)
	}
}

func TestOpenAPIConcurrentFirstRequests(t *testing.T) {
	app := oaFixture()
	app.OpenAPI("/openapi.json", oaFixtureConfig())
	var wg sync.WaitGroup
	bodies := make([]string, 16)
	for i := range bodies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bodies[i] = getSpec(t, app, "GET", "/openapi.json", nil).Body.String()
		}()
	}
	wg.Wait()
	for _, b := range bodies {
		if b != bodies[0] || b == "" {
			t.Fatal("concurrent first requests got different specs")
		}
	}
}

func TestOpenAPIErrorsAndMiddleware(t *testing.T) {
	mustPanic(t, `OpenAPIConfig.Security names security scheme "nope"`, func() {
		New().OpenAPI("/openapi.json", OpenAPIConfig{Security: []string{"nope"}})
	})

	app := New()
	app.Get("/a", func(c *Context) error { return nil }).Security("missing")
	app.OpenAPI("/openapi.json", OpenAPIConfig{})
	if w := getSpec(t, app, "GET", "/openapi.json", nil); w.Code != 500 {
		t.Fatalf("unknown scheme on a route: %d %s", w.Code, w.Body.String())
	}

	// Middleware passed to OpenAPI protects the spec route.
	app = New()
	requireKey := func(c *Context) error {
		if c.Header("X-API-Key") != "secret" {
			return ErrUnauthorized
		}
		return c.Next()
	}
	app.OpenAPI("/openapi.json", OpenAPIConfig{}, requireKey)
	if w := getSpec(t, app, "GET", "/openapi.json", nil); w.Code != 401 {
		t.Fatalf("without a key: %d", w.Code)
	}
	if w := getSpec(t, app, "GET", "/openapi.json", http.Header{"X-Api-Key": {"secret"}}); w.Code != 200 {
		t.Fatalf("with a key: %d", w.Code)
	}
}

func TestOpenAPIServedByDefault(t *testing.T) {
	app := New(Config{OpenAPI: OpenAPIConfig{Title: "Shop", Version: "2"}})
	app.Get("/pets", func(c *Context) error { return nil })
	rec := getSpec(t, app, "GET", "/openapi.json", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"title": "Shop"`) || !strings.Contains(rec.Body.String(), `"/pets"`) {
		t.Fatalf("default spec: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `"/openapi.json"`) {
		t.Fatal("the spec lists itself")
	}
	if rec := getSpec(t, app, "HEAD", "/openapi.json", nil); rec.Code != http.StatusOK {
		t.Fatalf("HEAD: %d", rec.Code)
	}
	if rec := getSpec(t, app, "POST", "/openapi.json", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("POST: %d", rec.Code)
	}

	// App middleware runs for it, so it can be protected like any route.
	guarded := New()
	guarded.Use(func(c *Context) error {
		if c.Header("X-Key") != "k" {
			return ErrUnauthorized
		}
		return c.Next()
	})
	if rec := getSpec(t, guarded, "GET", "/openapi.json", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unguarded: %d", rec.Code)
	}
	if rec := getSpec(t, guarded, "GET", "/openapi.json", http.Header{"X-Key": {"k"}}); rec.Code != http.StatusOK {
		t.Fatalf("guarded: %d", rec.Code)
	}
}

func TestOpenAPIDefaultGivesWay(t *testing.T) {
	off := New(Config{OpenAPIPath: "-"})
	if rec := getSpec(t, off, "GET", "/openapi.json", nil); rec.Code != http.StatusNotFound {
		t.Fatalf(`"-": %d`, rec.Code)
	}

	moved := New(Config{OpenAPIPath: "/api/spec.json"})
	if getSpec(t, moved, "GET", "/api/spec.json", nil).Code != http.StatusOK || getSpec(t, moved, "GET", "/openapi.json", nil).Code != http.StatusNotFound {
		t.Fatal("OpenAPIPath not used")
	}

	// A route at the path wins.
	own := New()
	own.Get("/openapi.json", func(c *Context) error { return c.String("mine") })
	if rec := getSpec(t, own, "GET", "/openapi.json", nil); rec.Body.String() != "mine" {
		t.Fatalf("route: %s", rec.Body)
	}

	// App.OpenAPI replaces the default.
	explicit := New()
	explicit.OpenAPI("/docs/openapi.json", OpenAPIConfig{})
	if getSpec(t, explicit, "GET", "/openapi.json", nil).Code != http.StatusNotFound || getSpec(t, explicit, "GET", "/docs/openapi.json", nil).Code != http.StatusOK {
		t.Fatal("App.OpenAPI didn't replace the default")
	}

	mustPanicWith(t, `Config.OpenAPIPath "spec.json" must start with /`, func() { New(Config{OpenAPIPath: "spec.json"}) })
	mustPanicWith(t, "needs Flows", func() {
		New(Config{OpenAPI: OpenAPIConfig{SecuritySchemes: map[string]OpenAPISecurityScheme{"o": {Type: "oauth2"}}}})
	})
}
