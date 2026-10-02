// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error { c.closed = true; return nil }

func TestTypedOutputTypes(t *testing.T) {
	files := fstest.MapFS{"report.csv": {Data: []byte("a,b\n")}}
	stream := &closeTracker{Reader: strings.NewReader("data: hi\n\n")}
	app := New()
	app.Get("/text", Typed(func(*Context, struct{}) (Text, error) { return "hello", nil }))
	app.Get("/html", Typed(func(*Context, struct{}) (HTML, error) { return "<h1>hi</h1>", nil }))
	app.Get("/bytes", Typed(func(*Context, struct{}) (Bytes, error) { return Bytes{Type: "text/csv", Data: []byte("a,b\n")}, nil }))
	app.Get("/opaque", Typed(func(*Context, struct{}) (Bytes, error) { return Bytes{Data: []byte{1, 2}}, nil }))
	app.Get("/file", Typed(func(*Context, struct{}) (File, error) { return File{Path: "report.csv", FS: files, Name: "r.csv"}, nil }))
	app.Get("/stream", Typed(func(*Context, struct{}) (Stream, error) {
		return Stream{Type: "text/event-stream", Reader: stream}, nil
	}))
	app.Get("/redirect", Typed(func(*Context, struct{}) (Redirect, error) { return "/new", nil }))
	app.Get("/moved", Typed(func(*Context, struct{}) (Redirect, error) { return "/new", nil })).Status(http.StatusMovedPermanently)
	app.Post("/created", Typed(func(*Context, struct{}) (Text, error) { return "made", nil })).Status(http.StatusCreated)

	for _, tt := range []struct {
		path, contentType, body, header string
		status                          int
	}{
		{"/text", "text/plain; charset=utf-8", "hello", "", 200},
		{"/html", "text/html; charset=utf-8", "<h1>hi</h1>", "", 200},
		{"/bytes", "text/csv", "a,b\n", "", 200},
		{"/opaque", "application/octet-stream", "\x01\x02", "", 200},
		{"/file", "text/csv; charset=utf-8", "a,b\n", `attachment; filename="r.csv"`, 200},
		{"/stream", "text/event-stream", "data: hi\n\n", "", 200},
		{"/redirect", "", "", "/new", http.StatusFound},
		{"/moved", "", "", "/new", http.StatusMovedPermanently},
		{"/created", "text/plain; charset=utf-8", "made", "", http.StatusCreated},
	} {
		method := "GET"
		if tt.path == "/created" {
			method = "POST"
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest(method, tt.path, nil))
		if w.Code != tt.status || (tt.contentType != "" && w.Header().Get("Content-Type") != tt.contentType) || w.Body.String() != tt.body {
			t.Errorf("%s: %d %q %q", tt.path, w.Code, w.Header().Get("Content-Type"), w.Body)
		}
		switch tt.path {
		case "/file":
			if w.Header().Get("Content-Disposition") != tt.header {
				t.Errorf("%s: disposition %q", tt.path, w.Header().Get("Content-Disposition"))
			}
		case "/redirect", "/moved":
			if w.Header().Get("Location") != tt.header {
				t.Errorf("%s: location %q", tt.path, w.Header().Get("Location"))
			}
		}
	}
	if !stream.closed {
		t.Error("a Stream reader that's an io.Closer wasn't closed")
	}
}

func TestOutputTypesInTheSpec(t *testing.T) {
	app := New(Config{Decoders: map[string]Decoder{"application/yaml": func(b []byte, v any) error { return json.Unmarshal(b, v) }}})
	app.Get("/text", Typed(func(*Context, struct{}) (Text, error) { return "", nil }))
	app.Get("/csv", Typed(func(*Context, struct{}) (Bytes, error) { return Bytes{}, nil })).Produces(200, "text/csv")
	app.Get("/blob", Typed(func(*Context, struct{}) (Bytes, error) { return Bytes{}, nil }))
	app.Get("/old", Typed(func(*Context, struct{}) (Redirect, error) { return "", nil })).Status(http.StatusSeeOther)
	app.Get("/export", func(c *Context) error { return nil }).Produces(200, "text/csv", "application/json").Output(oaPet{})
	app.Post("/import", func(c *Context) error { return nil }).Input("").Consumes("text/csv")
	type xmlIn struct {
		Name string `json:"name" xml:"name"`
	}
	app.Post("/xml", Typed(func(*Context, xmlIn) (NoContent, error) { return NoContent{}, nil }))
	spec, err := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]map[string]any `json:"content"`
			} `json:"requestBody"`
			Responses map[string]struct {
				Headers map[string]any            `json:"headers"`
				Content map[string]map[string]any `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	media := func(path, method, status string) map[string]map[string]any {
		return doc.Paths[path][method].Responses[status].Content
	}
	if m := media("/text", "get", "200"); m["text/plain"]["schema"] == nil || len(m) != 1 {
		t.Errorf("Text: %v", m)
	}
	if m := media("/csv", "get", "200"); m["text/csv"]["schema"] == nil || len(m) != 1 {
		t.Errorf("Bytes with Produces: %v", m)
	}
	if m := media("/blob", "get", "200"); m["application/octet-stream"] == nil || m["application/octet-stream"]["schema"] != nil {
		t.Errorf("Bytes: %v", m)
	}
	if r := doc.Paths["/old"]["get"].Responses["303"]; r.Headers["Location"] == nil || r.Content != nil {
		t.Errorf("Redirect: %+v", r)
	}
	if m := media("/export", "get", "200"); m["text/csv"] == nil || m["application/json"] == nil {
		t.Errorf("plain Produces: %v", m)
	}
	if _, ok := doc.Paths["/export"]["get"].Responses["default"]; ok {
		t.Error("Produces should replace the default response")
	}
	if c := doc.Paths["/import"]["post"].RequestBody.Content; c["text/csv"]["schema"] == nil || len(c) != 1 {
		t.Errorf("Consumes: %v", c)
	}
	if c := doc.Paths["/xml"]["post"].RequestBody.Content; c["application/json"] == nil || c["application/xml"] == nil || c["application/yaml"] == nil {
		t.Errorf("JSON, XML and the decoder's type: %v", c)
	}

	mustPanicWith(t, `Produces media type "csv" isn't a type/subtype`, func() {
		New().Get("/", func(c *Context) error { return nil }).Produces(200, "csv")
	})
	mustPanicWith(t, "Consumes needs at least one media type", func() {
		New().Post("/", func(c *Context) error { return nil }).Consumes()
	})
	mustPanicWith(t, "route status 404 is not a success or redirect status", func() {
		New().Get("/", func(c *Context) error { return nil }).Status(404)
	})
}

func TestMultipartMediaTag(t *testing.T) {
	type upload struct {
		Avatar *multipart.FileHeader `form:"avatar" media:"image/png,image/jpeg"`
	}
	app := New()
	app.Post("/", Typed(func(*Context, upload) (NoContent, error) { return NoContent{}, nil }))
	send := func(ct string) *httptest.ResponseRecorder {
		body := "--z\r\nContent-Disposition: form-data; name=\"avatar\"; filename=\"a\"\r\nContent-Type: " + ct + "\r\n\r\nx\r\n--z--\r\n"
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "multipart/form-data; boundary=z")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	if w := send("image/jpeg"); w.Code != http.StatusNoContent {
		t.Fatalf("jpeg: %d %s", w.Code, w.Body)
	}
	if w := send("text/plain"); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"avatar":"must be image/png or image/jpeg"`) {
		t.Fatalf("text: %d %s", w.Code, w.Body)
	}
	spec, _ := app.OpenAPISpec(OpenAPIConfig{Title: "T", Version: "1"})
	if !strings.Contains(string(spec), `"contentType": "image/png, image/jpeg"`) {
		t.Fatalf("encoding: %s", spec)
	}
}
