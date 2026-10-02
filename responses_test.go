// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProblemErrors(t *testing.T) {
	type in struct {
		Age int `query:"age"`
	}
	app := New(Config{ErrorHandler: ProblemErrors})
	app.Get("/missing", func(c *Context) error {
		return NewError(404, "pet not found").WithDetail("id", "7").WithDetail("status", 1).WithHeader("X-Trace", "t")
	})
	app.Get("/plain", func(c *Context) error { return NewError(http.StatusConflict) })
	app.Get("/internal", func(c *Context) error { return errors.New("db password is hunter2") })
	app.Get("/bind", Typed(func(_ *Context, _ in) (NoContent, error) { return NoContent{}, nil }))

	for _, tt := range []struct{ path, body string }{
		{"/missing", `{"type":"about:blank","title":"Not Found","status":404,"detail":"pet not found","id":"7"}`},
		{"/plain", `{"type":"about:blank","title":"Conflict","status":409}`},
		{"/internal", `{"type":"about:blank","title":"Internal Server Error","status":500}`},
		{"/bind?age=old", `{"type":"about:blank","title":"Bad Request","status":400,"detail":"invalid query parameter","errors":{"age":"must be an integer"}}`},
		{"/nowhere", `{"type":"about:blank","title":"Not Found","status":404}`},
	} {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
		if got := strings.TrimSpace(w.Body.String()); got != tt.body || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: %s %q\n got %s\nwant %s", tt.path, w.Header().Get("Content-Type"), w.Code, got, tt.body)
		}
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("GET", "/missing", nil))
	if w.Header().Get("X-Trace") != "t" {
		t.Error("an HTTPError's headers are sent")
	}

	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"application/problem+json"`, `"$ref": "#/components/schemas/Problem"`, `RFC 9457 problem details`} {
		if !strings.Contains(string(spec), want) {
			t.Errorf("spec lacks %s", want)
		}
	}
	if strings.Contains(string(spec), `"#/components/schemas/Error"`) {
		t.Error("the spec still describes the default envelope")
	}
}

type createdPet struct {
	Location string         `header:"Location" json:"-"`
	Total    int            `header:"X-Total-Count" json:"-"`
	Tags     []string       `header:"X-Tag" json:"-"`
	Modified time.Time      `header:"Last-Modified" json:"-"`
	Etag     *string        `header:"ETag" json:"-"`
	Cookies  []*http.Cookie `header:"Set-Cookie" json:"-"`
	ID       string         `json:"id"`
	Echoed   string         `header:"X-Echo" json:"echoed"`
}

func TestOutputHeaders(t *testing.T) {
	app := New()
	app.Post("/pets", Typed(func(*Context, struct{}) (createdPet, error) {
		return createdPet{
			Location: "/pets/7", Tags: []string{"a", "b"},
			Modified: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
			Cookies:  []*http.Cookie{{Name: "seen", Value: "1"}},
			ID:       "7", Echoed: "body",
		}, nil
	})).Status(http.StatusCreated)
	app.Get("/nil", Typed(func(*Context, struct{}) (*createdPet, error) { return nil, nil }))

	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("POST", "/pets", nil))
	h := w.Header()
	if w.Code != 201 || h.Get("Location") != "/pets/7" || h.Get("X-Total-Count") != "0" ||
		strings.Join(h.Values("X-Tag"), ",") != "a,b" || h.Get("Last-Modified") != "Fri, 02 Oct 2026 12:00:00 GMT" ||
		h.Get("Set-Cookie") != "seen=1" || h.Get("X-Echo") != "" {
		t.Errorf("headers: %d %v", w.Code, h)
	}
	if _, ok := h["Etag"]; ok {
		t.Error("a nil pointer sends no header")
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"id":"7","echoed":"body"}` {
		t.Errorf("body: %s", got)
	}
	w = httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("GET", "/nil", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "null" {
		t.Errorf("nil output: %d %s", w.Code, w.Body)
	}

	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Headers map[string]struct {
					Required bool           `json:"required"`
					Schema   map[string]any `json:"schema"`
				} `json:"headers"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	headers := doc.Paths["/pets"]["post"].Responses["201"].Headers
	if len(headers) != 6 || !headers["X-Total-Count"].Required || headers["Location"].Required ||
		headers["X-Tag"].Schema["type"] != "array" || headers["Last-Modified"].Schema["format"] != "date-time" {
		t.Errorf("spec headers: %+v", headers)
	}
	if _, ok := headers["X-Echo"]; ok {
		t.Error("a header field that's in the body isn't a response header")
	}

	mustPanicWith(t, `is a cookie, so its tag must be header:"Set-Cookie"`, func() {
		type bad struct {
			C *http.Cookie `header:"Cookie" json:"-"`
		}
		Typed(func(*Context, struct{}) (bad, error) { return bad{}, nil })
	})
	mustPanicWith(t, "sets Set-Cookie, so it must be a *http.Cookie", func() {
		type bad struct {
			C string `header:"Set-Cookie" json:"-"`
		}
		Typed(func(*Context, struct{}) (bad, error) { return bad{}, nil })
	})
	mustPanicWith(t, "can't be the X-Map header", func() {
		type bad struct {
			M map[string]string `header:"X-Map" json:"-"`
		}
		Typed(func(*Context, struct{}) (bad, error) { return bad{}, nil })
	})
}

type examplePet struct {
	ID       string `json:"id" openapi:"readonly"`
	Name     string `json:"name"`
	Password string `json:"password,omitempty" openapi:"writeonly"`
	Legacy   string `json:"legacy,omitempty" openapi:"deprecated"`
}

type exampleInput struct {
	Store string `path:"store"`
	Name  string `json:"name"`
}

type namedRecord struct {
	Name string `json:"name"`
}

func (namedRecord) OpenAPIName() string { return "Record" }

func TestExamples(t *testing.T) {
	build := func(cfg Config, register func(*App)) (string, error) {
		app := New(cfg)
		register(app)
		spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
		return string(spec), err
	}
	spec, err := build(Config{}, func(app *App) {
		app.Post("/stores/{store}/pets", Typed(func(*Context, exampleInput) (examplePet, error) { return examplePet{}, nil })).
			Status(201).
			RequestExample("tom", exampleInput{Store: "s1", Name: "Tom"}).
			Example(201, "created", examplePet{ID: "7", Name: "Tom"}).
			Errors(404).
			Example(404, "no store", NewError(404, "store not found"))
		app.Get("/record", Typed(func(*Context, struct{}) (namedRecord, error) { return namedRecord{}, nil }))
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"examples":{"tom":{"value":{"name":"Tom"}}}`,
		`"examples":{"created":{"value":{"id":"7","name":"Tom"}}}`,
		`"examples":{"no store":{"value":{"error":{"status":404,"message":"store not found"}}}}`,
		`"readOnly":true`, `"writeOnly":true`, `"deprecated":true`,
		`"$ref":"#/components/schemas/Record"`,
	} {
		if !strings.Contains(compactJSON(t, spec), want) {
			t.Errorf("spec lacks %s\n%s", want, spec)
			break
		}
	}

	spec, err = build(Config{ErrorHandler: ProblemErrors}, func(app *App) {
		app.Get("/", func(c *Context) error { return nil }).Errors(409).Example(409, "taken", NewError(409, "name taken"))
	})
	if err != nil || !strings.Contains(compactJSON(t, spec), `"examples":{"taken":{"value":{"type":"about:blank","title":"Conflict","status":409,"detail":"name taken"}}}`) {
		t.Errorf("problem example: %v\n%s", err, spec)
	}

	for _, tt := range []struct {
		name     string
		cfg      Config
		register func(*App)
		want     string
	}{
		{"wrong type", Config{}, func(app *App) {
			app.Get("/", Typed(func(*Context, struct{}) (examplePet, error) { return examplePet{}, nil })).Example(200, "x", namedRecord{})
		}, `GET /: example "x" for 200: value is a zinc.namedRecord; the response is a zinc.examplePet`},
		{"undocumented status", Config{}, func(app *App) {
			app.Get("/", Typed(func(*Context, struct{}) (examplePet, error) { return examplePet{}, nil })).Example(404, "x", NewError(404))
		}, "the route doesn't document 404; declare it with Errors or Response"},
		{"error not an HTTPError", Config{}, func(app *App) {
			app.Get("/", func(c *Context) error { return nil }).Errors(404).Example(404, "x", "missing")
		}, "an error example is an *HTTPError"},
		{"custom error handler", Config{ErrorHandler: func(*Context, error) {}}, func(app *App) {
			app.Get("/", func(c *Context) error { return nil }).Errors(404).Example(404, "x", NewError(404))
		}, "the 404 response has no body"},
		{"no request body", Config{}, func(app *App) {
			app.Get("/", Typed(func(*Context, struct{}) (examplePet, error) { return examplePet{}, nil })).RequestExample("x", struct{}{})
		}, "is for a request body, and the route has none"},
	} {
		if _, err := build(tt.cfg, tt.register); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
	mustPanicWith(t, "Example needs a name", func() {
		New().Get("/", func(c *Context) error { return nil }).Example(200, " ", 1)
	})
}

func compactJSON(t *testing.T, s string) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(s)); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
