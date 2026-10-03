// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type patternInput struct {
	Key    string  `json:"key" pattern:"^[a-z0-9][a-z0-9-]{0,39}$" validate:"required"`
	Code   *string `json:"code,omitempty" pattern:"^[A-Z]{3}$"`
	Tenant string  `header:"X-Tenant" pattern:"^t-"`
}

// acceptAll is a Validator that checks nothing and claims nothing.
type acceptAll struct{}

func (acceptAll) Validate(any) error { return nil }

// A pattern tag is checked and documented whatever the validator, like an
// enum, and a zero value counts as left out.
func TestPatternTag(t *testing.T) {
	for _, cfg := range []Config{{}, {Validator: acceptAll{}}} {
		app := New(cfg)
		app.Post("/things", Typed(func(_ *Context, in patternInput) (NoContent, error) { return NoContent{}, nil }))
		send := func(body string, header ...string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodPost, "/things", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			for i := 0; i+1 < len(header); i += 2 {
				r.Header.Set(header[i], header[i+1])
			}
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			return w
		}
		if w := send(`{"key":"web-app","code":"GBP"}`, "X-Tenant", "t-acme"); w.Code != http.StatusNoContent {
			t.Fatalf("%T: valid input: %d %s", cfg.Validator, w.Code, w.Body)
		}
		// Left out, or empty, isn't checked against the pattern.
		if w := send(`{"key":"web-app"}`); w.Code != http.StatusNoContent {
			t.Fatalf("%T: optional fields left out: %d %s", cfg.Validator, w.Code, w.Body)
		}
		w := send(`{"key":"Web App!","code":"gbp"}`, "X-Tenant", "acme")
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%T: invalid input: %d %s", cfg.Validator, w.Code, w.Body)
		}
		for _, want := range []string{
			`"key":"must match the pattern ^[a-z0-9][a-z0-9-]{0,39}$"`,
			`"code":"must match the pattern ^[A-Z]{3}$"`,
			`"X-Tenant":"must match the pattern ^t-"`,
		} {
			if !strings.Contains(w.Body.String(), want) {
				t.Errorf("%T: response lacks %s: %s", cfg.Validator, want, w.Body)
			}
		}

		spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Paths map[string]map[string]struct {
				Parameters []struct {
					Name   string         `json:"name"`
					Schema map[string]any `json:"schema"`
				} `json:"parameters"`
			} `json:"paths"`
			Components struct {
				Schemas map[string]struct {
					Properties map[string]map[string]any `json:"properties"`
				} `json:"schemas"`
			} `json:"components"`
		}
		if err := json.Unmarshal(spec, &doc); err != nil {
			t.Fatal(err)
		}
		props := doc.Components.Schemas["PatternInputBody"].Properties
		if props["key"]["pattern"] != "^[a-z0-9][a-z0-9-]{0,39}$" {
			t.Errorf("%T: key's pattern: %v", cfg.Validator, props["key"])
		}
		if props["code"]["pattern"] != "^[A-Z]{3}$" {
			t.Errorf("%T: a pointer field's pattern sits on its value: %v", cfg.Validator, props["code"])
		}
		params := doc.Paths["/things"]["post"].Parameters
		if len(params) != 1 || params[0].Name != "X-Tenant" || params[0].Schema["pattern"] != "^t-" {
			t.Errorf("%T: header parameter: %+v", cfg.Validator, params)
		}
	}
}

// A pattern that can't work fails when the route is registered, not when a
// request arrives.
func TestPatternTagMistakes(t *testing.T) {
	mustPanicWith(t, "pattern tags can't be used: zinc.badPattern.Key: pattern \"[a-z\"", func() {
		type badPattern struct {
			Key string `json:"key" pattern:"[a-z"`
		}
		New().Post("/", Typed(func(*Context, badPattern) (NoContent, error) { return NoContent{}, nil }))
	})
	mustPanicWith(t, "zinc.numberPattern.Count: pattern applies only to strings, not int", func() {
		type numberPattern struct {
			Count int `json:"count" pattern:"^[0-9]+$"`
		}
		New().Post("/", Typed(func(*Context, numberPattern) (NoContent, error) { return NoContent{}, nil }))
	})
	// A custom Validator doesn't change that: Zinc checks patterns itself.
	mustPanicWith(t, "pattern tags can't be used", func() {
		type nestedBad struct {
			Inner struct {
				Code string `json:"code" pattern:"(("`
			} `json:"inner"`
		}
		New(Config{Validator: acceptAll{}}).Post("/", Typed(func(*Context, nestedBad) (NoContent, error) { return NoContent{}, nil }))
	})
}
