// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// encodeKV writes a map as sorted "key=value" lines, standing in for a
// third-party format such as YAML.
func encodeKV(v any) ([]byte, error) {
	m, ok := v.(Map)
	if !ok {
		return nil, errors.New("kv: only maps")
	}
	return []byte(fmt.Sprintf("ok=%v", m["ok"])), nil
}

func TestEncoders(t *testing.T) {
	marked := func(v any) ([]byte, error) {
		data, err := json.Marshal(v)
		return append([]byte(`{"custom":true,"value":`), append(data, '}')...), err
	}
	app := New(Config{Encoders: map[string]Encoder{
		"application/x-kv": encodeKV,
		"application/json": marked,
	}})
	app.Get("/encode", func(c *Context) error { return c.Status(StatusCreated).Encode("application/x-kv", Map{"ok": true}) })
	app.Get("/unknown", func(c *Context) error { return c.Encode("application/yaml", Map{"ok": true}) })
	app.Get("/json", func(c *Context) error { return c.JSON(Map{"a": 1}) })
	app.Get("/pretty", func(c *Context) error { return c.JSONPretty(Map{"a": 1}, "  ") })
	app.Get("/negotiate", func(c *Context) error {
		return c.Negotiate(map[string]any{"application/x-kv": Map{"ok": true}, "text/plain": "plain"})
	})
	app.Get("/sse", func(c *Context) error { return c.SSE(Event{Data: Map{"a": 1}}) })

	cases := []struct {
		path, accept string
		status       int
		contentType  string
		body         string
	}{
		{"/encode", "", 201, "application/x-kv", "ok=true"},
		{"/json", "", 200, jsonType, `{"custom":true,"value":{"a":1}}`},
		{"/pretty", "", 200, jsonType, "{\n  \"custom\": true,\n  \"value\": {\n    \"a\": 1\n  }\n}"},
		{"/negotiate", "application/x-kv", 200, "application/x-kv", "ok=true"},
		{"/sse", "", 200, "text/event-stream", `data: {"custom":true,"value":{"a":1}}`},
		{"/unknown", "", 500, jsonType, ""},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.accept != "" {
			req.Header.Set(HeaderAccept, tc.accept)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != tc.status || !strings.HasPrefix(rec.Header().Get(HeaderContentType), tc.contentType) {
			t.Errorf("%s: %d %q, want %d %q", tc.path, rec.Code, rec.Header().Get(HeaderContentType), tc.status, tc.contentType)
		}
		if tc.body != "" && !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%s: body %q, want %q", tc.path, rec.Body.String(), tc.body)
		}
	}
}

// Without configured encoders, Encode still writes the built-in formats.
func TestEncodeBuiltins(t *testing.T) {
	app := New()
	app.Get("/json", func(c *Context) error { return c.Encode("application/json; charset=utf-8", Map{"a": 1}) })
	app.Get("/xml", func(c *Context) error { return c.Encode("text/xml", xmlPayload{Value: "x"}) })
	for path, want := range map[string]string{"/json": "{\"a\":1}\n", "/xml": "<response><value>x</value></response>"} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 || rec.Body.String() != want {
			t.Errorf("%s: %d %q, want %q", path, rec.Code, rec.Body.String(), want)
		}
	}
}

// A JSON decoder entry replaces encoding/json everywhere JSON is read,
// including requests without a Content-Type.
func TestJSONDecoderOverride(t *testing.T) {
	calls := 0
	app := New(Config{Decoders: map[string]Decoder{"application/json": func(data []byte, v any) error {
		calls++
		return json.Unmarshal(data, v)
	}}})
	app.Post("/", func(c *Context) error {
		var in struct {
			Name string `json:"name"`
		}
		if err := c.Bind().Body(&in); err != nil {
			return err
		}
		if err := c.Bind().JSON(&in); err != nil {
			return err
		}
		return c.String(in.Name)
	})
	for _, contentType := range []string{"application/json", ""} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"zinc"}`))
		if contentType != "" {
			req.Header.Set(HeaderContentType, contentType)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Body.String() != "zinc" {
			t.Fatalf("Content-Type %q: %d %q", contentType, rec.Code, rec.Body.String())
		}
	}
	if calls != 4 {
		t.Fatalf("custom decoder calls = %d, want 4", calls)
	}
}

// An app without Decoders or Encoders pays nothing for them on the JSON path.
func TestDefaultJSONPathAllocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are unreliable under -race")
	}
	plain := New()
	custom := New(Config{Decoders: map[string]Decoder{"application/x-kv": func([]byte, any) error { return nil }}})
	handler := func(c *Context) error {
		var in struct {
			Name string `json:"name"`
		}
		if err := c.Bind().Body(&in); err != nil {
			return err
		}
		return c.JSON(in)
	}
	plain.Post("/", handler)
	custom.Post("/", handler)
	measure := func(app *App) float64 {
		return testing.AllocsPerRun(200, func() {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"zinc"}`))
			req.Header.Set(HeaderContentType, "application/json")
			app.ServeHTTP(&discardWriter{header: http.Header{}}, req)
		})
	}
	if p, c := measure(plain), measure(custom); c > p {
		t.Fatalf("configuring an unrelated decoder costs allocations on JSON: %.1f vs %.1f", c, p)
	}
}
