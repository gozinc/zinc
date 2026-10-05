// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"testing"
)

// specCodes returns the response codes the spec documents for method path.
func specCodes(t *testing.T, app *App, method, path string) []int {
	t.Helper()
	raw, err := app.OpenAPISpec(OpenAPIConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	op, ok := doc.Paths[path][method]
	if !ok {
		t.Fatalf("spec has no %s %s", method, path)
	}
	var codes []int
	for code := range op.Responses {
		n, err := strconv.Atoi(code)
		if err != nil {
			t.Fatalf("response code %q", code)
		}
		codes = append(codes, n)
	}
	slices.Sort(codes)
	return codes
}

// specSuccess returns the one documented code that isn't 500.
func specSuccess(t *testing.T, codes []int) int {
	t.Helper()
	var success []int
	for _, code := range codes {
		if code != http.StatusInternalServerError {
			success = append(success, code)
		}
	}
	if len(success) != 1 {
		t.Fatalf("spec codes %v, want one success code and 500", codes)
	}
	return success[0]
}

// The status a typed route sends and the status its spec documents come
// from one rule, for every output kind and declared status.
func TestTypedStatusMatchesSpec(t *testing.T) {
	kinds := []struct {
		name     string
		handler  HandlerFunc
		redirect bool
	}{
		{"struct", Typed(func(*Context, struct{}) (struct{ ID string }, error) { return struct{ ID string }{"1"}, nil }), false},
		{"NoContent", Typed(func(*Context, struct{}) (NoContent, error) { return NoContent{}, nil }), false},
		{"Redirect", Typed(func(*Context, struct{}) (Redirect, error) { return "/new", nil }), true},
		{"Text", Typed(func(*Context, struct{}) (Text, error) { return "hi", nil }), false},
		{"Bytes", Typed(func(*Context, struct{}) (Bytes, error) { return Bytes{Data: []byte("hi")}, nil }), false},
	}
	statuses := []int{0, 200, 201, 204, 301, 303}
	for _, kind := range kinds {
		for _, status := range statuses {
			t.Run(fmt.Sprintf("%s/%d", kind.name, status), func(t *testing.T) {
				app := New()
				route := app.Get("/r", kind.handler)
				if status != 0 {
					if kind.redirect && (status < 300 || status > 399) {
						mustPanic(t, "Redirect output", func() { route.Status(status) })
						return
					}
					route.Status(status)
				}
				rec := performRequest(t, app, http.MethodGet, "/r", nil, nil)
				want := specSuccess(t, specCodes(t, app, "get", "/r"))
				if rec.Code != want {
					t.Fatalf("sent %d, spec documents %d", rec.Code, want)
				}
				if kind.redirect && rec.Header().Get(HeaderLocation) != "/new" {
					t.Fatalf("Location = %q", rec.Header().Get(HeaderLocation))
				}
			})
		}
	}
}

func TestTypedErrorStatusMatchesSpec(t *testing.T) {
	app := New()
	app.Get("/r", Typed(func(*Context, struct{}) (NoContent, error) { return NoContent{}, errors.New("boom") })).Status(200)
	rec := performRequest(t, app, http.MethodGet, "/r", nil, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("sent %d, want 500", rec.Code)
	}
	if codes := specCodes(t, app, "get", "/r"); !slices.Contains(codes, http.StatusInternalServerError) {
		t.Fatalf("spec codes %v, want 500 among them", codes)
	}
}

func TestPlainRedirectOutputRejectsNonRedirectStatus(t *testing.T) {
	app := New()
	mustPanic(t, "GET /a has a Redirect output", func() {
		app.Get("/a", func(c *Context) error { return c.Redirect("/b") }).Output(Redirect("")).Status(201)
	})
	mustPanic(t, "GET /c has a Redirect output", func() {
		app.Get("/c", func(c *Context) error { return c.Redirect("/b") }).Status(201).Output(Redirect(""))
	})
}

type failingJSON struct{}

func (failingJSON) MarshalJSON() ([]byte, error) { return nil, errors.New("no") }

type headeredFailure struct {
	Location string       `header:"Location" json:"-"`
	Cookie   *http.Cookie `header:"Set-Cookie" json:"-"`
	Trace    string       `header:"X-Trace" json:"-"`
	Bad      failingJSON  `json:"bad"`
}

// A failed encode sends a clean error: none of the output's header fields.
func TestTypedEncodeFailureSendsNoOutputHeaders(t *testing.T) {
	app := New()
	app.Post("/r", Typed(func(*Context, struct{}) (headeredFailure, error) {
		return headeredFailure{Location: "/x", Cookie: &http.Cookie{Name: "s", Value: "1"}, Trace: "t"}, nil
	})).Status(201)
	rec := performRequest(t, app, http.MethodPost, "/r", nil, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	for _, name := range []string{"Location", "Set-Cookie", "X-Trace"} {
		if v := rec.Header().Values(name); len(v) != 0 {
			t.Errorf("%s = %q on the error response", name, v)
		}
	}
}

// Header fields are still sent, and the body is unchanged, when encoding
// succeeds.
func TestTypedOutputHeadersAfterEncode(t *testing.T) {
	type created struct {
		Location string `header:"Location" json:"-"`
		ID       string `json:"id"`
	}
	app := New()
	app.Post("/r", Typed(func(*Context, struct{}) (created, error) {
		return created{Location: "/r/1", ID: "1"}, nil
	})).Status(201)
	rec := performRequest(t, app, http.MethodPost, "/r", nil, nil)
	if rec.Code != 201 || rec.Header().Get("Location") != "/r/1" || rec.Body.String() != "{\"id\":\"1\"}\n" {
		t.Fatalf("got %d %q %q", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if ct := rec.Header().Get(HeaderContentType); ct != jsonType {
		t.Fatalf("Content-Type = %q", ct)
	}
}

// A status middleware chose with Context.Status wins over NoContent's 204
// and over the route's declared status, as it does for other outputs.
func TestTypedNoContentKeepsMiddlewareStatus(t *testing.T) {
	setStatus := func(code int) HandlerFunc {
		return func(c *Context) error {
			c.Status(code)
			return c.Next()
		}
	}
	handler := Typed(func(*Context, struct{}) (NoContent, error) { return NoContent{}, nil })
	tests := []struct {
		name     string
		mw       int
		declared int
		want     int
	}{
		{"middleware over default", 202, 0, 202},
		{"middleware over declared", 202, 200, 202},
		{"200 from middleware is the default", 200, 0, 204},
		{"declared without middleware", 0, 200, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := New()
			handlers := []HandlerFunc{handler}
			if tt.mw != 0 {
				handlers = []HandlerFunc{setStatus(tt.mw), handler}
			}
			route := app.Get("/r", handlers...)
			if tt.declared != 0 {
				route.Status(tt.declared)
			}
			rec := performRequest(t, app, http.MethodGet, "/r", nil, nil)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// A handler that panics when described is treated as untyped rather than
// crashing registration.
func TestDescribeHandlerRecovers(t *testing.T) {
	h := HandlerFunc(func(*Context) error { panic("describe") })
	typedPCs.Store(handlerPC(h), struct{}{})
	defer typedPCs.Delete(handlerPC(h))
	if _, ok := describeHandler(h); ok {
		t.Fatal("describeHandler reported types for a panicking handler")
	}
}
