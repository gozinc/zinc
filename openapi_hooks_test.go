// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"strings"
	"testing"
)

func TestSpecHooks(t *testing.T) {
	app := New()
	app.Get("/pets", Typed(func(*Context, struct{}) ([]oaPet, error) { return nil, nil })).
		Summary("List pets").
		Operation(func(op map[string]any) { op["x-rate-limit"] = 100 }).
		Operation(func(op map[string]any) { op["summary"] = "All pets" })
	spec, err := app.OpenAPISpec(OpenAPIConfig{
		Title: "T", Version: "1",
		Extensions: map[string]any{"x-logo": map[string]string{"url": "/logo.png"}},
		Mutate: func(spec map[string]any) error {
			spec["info"].(map[string]any)["x-audience"] = "public"
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(spec)
	for _, want := range []string{`"x-rate-limit": 100`, `"summary": "All pets"`, `"x-logo": {`, `"x-audience": "public"`} {
		if !strings.Contains(got, want) {
			t.Errorf("spec lacks %s", want)
		}
	}
	// Zinc's order is kept, and new members follow.
	if strings.Index(got, `"openapi"`) > strings.Index(got, `"info"`) || strings.Index(got, `"paths"`) > strings.Index(got, `"x-logo"`) ||
		strings.Index(got, `"summary"`) > strings.Index(got, `"responses"`) {
		t.Errorf("order changed:\n%s", got)
	}

	for _, tt := range []struct {
		name string
		cfg  OpenAPIConfig
		want string
	}{
		{"broken ref", OpenAPIConfig{Mutate: func(s map[string]any) error {
			delete(s["components"].(map[string]any)["schemas"].(map[string]any), "OaPet")
			return nil
		}}, "the spec after its hooks: #/paths/~1pets"},
		{"no title", OpenAPIConfig{Mutate: func(s map[string]any) error {
			delete(s["info"].(map[string]any), "title")
			return nil
		}}, "info.title is missing"},
		{"mutate error", OpenAPIConfig{Mutate: func(map[string]any) error { return errors.New("nope") }}, "OpenAPIConfig.Mutate: nope"},
		{"bad extension", OpenAPIConfig{Extensions: map[string]any{"logo": 1}}, `Extensions key "logo" must start with "x-"`},
	} {
		tt.cfg.Title, tt.cfg.Version = "T", "1"
		if _, err := app.OpenAPISpec(tt.cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
	mustPanicWith(t, "Operation hook is nil", func() { New().Get("/", func(c *Context) error { return nil }).Operation(nil) })
}
