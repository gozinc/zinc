// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type sourced struct {
	Role   string `header:"X-Role"`
	Limit  int    `query:"limit" default:"20"`
	Tenant string `header:"X-Tenant" json:"tenant" xml:"tenant"`
	Name   string `json:"name" xml:"name"`
}

// A body never fills a field tagged only for the URL, headers or cookies,
// whatever its format; a field tagged for both takes either.
func TestBodyOnlyFillsBodyFields(t *testing.T) {
	decode := func(body []byte, v any) error { return json.Unmarshal(body, v) } // a YAML stand-in
	app := New(Config{Decoders: map[string]Decoder{"application/yaml": decode}})
	var got sourced
	app.Post("/", Typed(func(_ *Context, in sourced) (NoContent, error) { got = in; return NoContent{}, nil }))
	send := func(contentType, body string, header ...string) {
		t.Helper()
		got = sourced{}
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", contentType, w.Code, w.Body)
		}
	}
	for _, ct := range []string{"application/json", "application/yaml"} {
		send(ct, `{"Role":"admin","Limit":99,"tenant":"acme","name":"Ada"}`)
		if got.Role != "" || got.Limit != 20 || got.Tenant != "acme" || got.Name != "Ada" {
			t.Errorf("%s: %+v", ct, got)
		}
	}
	send("application/xml", `<sourced><Role>admin</Role><tenant>acme</tenant><name>Ada</name></sourced>`)
	if got.Role != "" || got.Tenant != "acme" || got.Name != "Ada" {
		t.Errorf("xml: %+v", got)
	}
	// An escaped key still matches the field, and a body with non-ASCII
	// text is checked the same way.
	for _, body := range []string{`{"\u0052ole":"admin"}`, `{"\u0072ole":"admin"}`, `{"Ro\u006ce":"admin"}`, `{"Limit":99,"name":"\u00e9"}`, "{\"Limit\":99,\"name\":\"\u00e9\"}"} {
		send("application/json", body)
		if got.Role != "" || got.Limit != 20 {
			t.Errorf("%s: %+v", body, got)
		}
	}
	// The header still binds, and wins over the body for a both-tagged field.
	send("application/json", `{"tenant":"body"}`, "X-Role", "viewer", "X-Tenant", "header")
	if got.Role != "viewer" || got.Tenant != "header" {
		t.Errorf("headers: %+v", got)
	}

	// The single-source body binders follow the same rule.
	plain := New()
	plain.Post("/", func(c *Context) error {
		var in sourced
		if err := c.Bind().JSON(&in); err != nil {
			return err
		}
		return c.JSON(in)
	})
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"Role":"admin","name":"Ada"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	plain.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `"Role":""`) {
		t.Fatalf("Bind().JSON: %s", w.Body)
	}
}

// A plain handler's route status is its default; its own status, an error,
// and a typed NoContent keep theirs.
func TestRouteStatusForPlainHandlers(t *testing.T) {
	app := New()
	app.Post("/default", func(c *Context) error { return c.JSON("ok") }).Status(http.StatusCreated)
	app.Post("/own", func(c *Context) error { return c.Status(http.StatusAccepted).JSON("ok") }).Status(http.StatusCreated)
	app.Post("/error", func(c *Context) error { return ErrConflict }).Status(http.StatusCreated)
	app.Post("/typed", Typed(func(*Context, struct{}) (string, error) { return "ok", nil })).Status(http.StatusCreated)
	app.Delete("/gone", Typed(func(*Context, struct{}) (NoContent, error) { return NoContent{}, nil }))
	for target, want := range map[string]int{"POST /default": 201, "POST /own": 202, "POST /error": 409, "POST /typed": 201, "DELETE /gone": 204} {
		method, path, _ := strings.Cut(target, " ")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != want {
			t.Errorf("%s: %d, want %d", target, w.Code, want)
		}
	}
}

// Single-source binders compose: values bound from the path, query and
// headers survive a later Bind().JSON whose body names the same fields.
func TestSingleSourceBindersCompose(t *testing.T) {
	type input struct {
		Team   int      `path:"team"`
		Trace  string   `header:"X-Trace"`
		Tags   []string `query:"tag"`
		Ratio  float64  `query:"ratio"`
		Active bool     `query:"active"`
		Limit  *int     `query:"limit"`
		Name   string   `json:"name"`
	}
	app := New()
	var got input
	app.Post("/teams/{team}", func(c *Context) error {
		got = input{}
		for _, bind := range []func(any) error{c.Bind().Path, c.Bind().Query, c.Bind().Header, c.Bind().JSON} {
			if err := bind(&got); err != nil {
				return err
			}
		}
		return c.NoContent()
	})
	r := httptest.NewRequest("POST", "/teams/42?tag=a&tag=b&ratio=0.5&active=true&limit=5", strings.NewReader(`{"Team":7,"Trace":"body","Tags":["x"],"Ratio":9,"Active":false,"Limit":99,"name":"Ada"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Trace", "abc")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || got.Team != 42 || got.Trace != "abc" || strings.Join(got.Tags, ",") != "a,b" || got.Ratio != 0.5 || !got.Active || got.Limit == nil || *got.Limit != 5 || got.Name != "Ada" {
		t.Fatalf("%d %+v", w.Code, got)
	}
}

func TestBodyMentionsParams(t *testing.T) {
	plan := bindingPlanFor(reflect.TypeFor[sourced]())
	for _, tt := range []struct {
		body string
		want bool
	}{
		{`{"Role":"admin"}`, true},
		{`{"ROLE":1}`, true},
		{`<role>x</role>`, true},
		{`{"limit":5}`, true},
		{`{"name":"Ada","tenant":"x"}`, false},
		{`{"r":1}`, false},
		{`{"\u0072ole":"admin"}`, true},
		{"{\"\u017fort\":1}", true},
		{"{\"\u212aind\":1}", true},
		{``, false},
	} {
		if got := plan.bodyMentionsParams([]byte(tt.body)); got != tt.want {
			t.Errorf("%q: %v", tt.body, got)
		}
	}
}
