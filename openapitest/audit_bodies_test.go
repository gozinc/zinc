// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"bytes"
	"mime/multipart"

	"github.com/0mjs/zinc"
)

type newPet struct {
	Name  string   `json:"name" validate:"required"`
	Kind  string   `json:"kind" validate:"omitempty,oneof=cat dog"`
	Owner *owner   `json:"owner"`
	Tags  []string `json:"tags"`
}

type formLogin struct {
	User     string `form:"user" validate:"required"`
	Password string `form:"password" validate:"required"`
}

type upload struct {
	Caption string                  `form:"caption"`
	Photo   *multipart.FileHeader   `form:"photo"`
	Extras  []*multipart.FileHeader `form:"extras"`
}

type jsonOrForm struct {
	Name string `json:"name" form:"name"`
}

type xmlPet struct {
	Name string `json:"name" xml:"name"`
}

type getWithBody struct {
	Filter string `json:"filter"`
}

type deleteWithBody struct {
	ID     int    `path:"id"`
	Reason string `json:"reason"`
}

type stringified struct {
	N int `json:"n,string"`
}

type patchPet struct {
	Name *string `json:"name"`
	Age  *int    `json:"age"`
}

func typedPost[In any](path string, cfg zinc.Config) func() (*zinc.App, zinc.OpenAPIConfig) {
	return func() (*zinc.App, zinc.OpenAPIConfig) {
		app := zinc.New(cfg)
		app.Post(path, zinc.Typed(func(_ *zinc.Context, in In) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
		return app, zinc.OpenAPIConfig{}
	}
}

func post(path, body string, status int) probe {
	return probe{method: "POST", target: path, body: body, status: status}
}

func multipartBody() (string, string) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("caption", "hi")
	fw, _ := w.CreateFormFile("photo", "a.png")
	_, _ = fw.Write([]byte("png"))
	fw, _ = w.CreateFormFile("extras", "b.png")
	_, _ = fw.Write([]byte("png"))
	_ = w.Close()
	return b.String(), w.FormDataContentType()
}

func bodyScenarios() []scenario {
	mp, mpType := multipartBody()
	return []scenario{
		{id: "B01", area: "Bodies", title: "JSON body with nested and optional fields",
			build: typedPost[newPet]("/pets", validated()),
			probes: []probe{
				post("/pets", `{"name":"Rex"}`, 204),
				post("/pets", `{"name":"Rex","kind":"dog","owner":{"email":"a@b.co"},"tags":["x"]}`, 204),
				post("/pets", `{"kind":"dog"}`, 422),
			},
			expect: func(f *findings, s spec) {
				sch := s.requestSchema("POST", "/pets", "application/json")
				if !has(required(sch), "name") || has(required(sch), "tags") {
					f.add("request body should require only name: %v", required(sch))
				}
			}},
		{id: "B02", area: "Bodies", title: "Same struct as request and response",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New(validated())
				app.Post("/pets", zinc.Typed(func(_ *zinc.Context, in newPet) (newPet, error) { return in, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{post("/pets", `{"name":"Rex"}`, 200)},
			expect: func(f *findings, s spec) {
				f.add("info: components %v", s.components())
			}},
		{id: "B03", area: "Bodies", title: "Array body on a plain handler (.Input)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Post("/batch", func(c *zinc.Context) error {
					var items []item
					if err := c.Bind().JSON(&items); err != nil {
						return err
					}
					return c.NoContent()
				}).Input([]item{})
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{post("/batch", `[{"id":1},{"id":2}]`, 204)},
			expect: func(f *findings, s spec) {
				if !typeIs(s.requestSchema("POST", "/batch", "application/json"), "array") {
					f.add("body should be an array")
				}
			}},
		{id: "B04", area: "Bodies", title: "URL-encoded form",
			build:  typedPost[formLogin]("/login", validated()),
			probes: []probe{{method: "POST", target: "/login", body: "user=a&password=b", contentType: "application/x-www-form-urlencoded", status: 204}},
			expect: func(f *findings, s spec) {
				sch := s.requestSchema("POST", "/login", "application/x-www-form-urlencoded")
				if sch == nil || !has(required(sch), "user") {
					f.add("form body missing or user not required: %v", sch)
				}
			}},
		{id: "B05", area: "Bodies", title: "Multipart with one and several files",
			build:  typedPost[upload]("/upload", zinc.Config{}),
			probes: []probe{{method: "POST", target: "/upload", body: mp, contentType: mpType, status: 204}},
			expect: func(f *findings, s spec) {
				sch := s.requestSchema("POST", "/upload", "multipart/form-data")
				if sch == nil || !typeIs(prop(sch, "extras"), "array") {
					f.add("multipart body or file array missing: %v", sch)
				}
			}},
		{id: "B06", area: "Bodies", title: "One struct accepting JSON or a form",
			build: typedPost[jsonOrForm]("/either", zinc.Config{}),
			probes: []probe{
				post("/either", `{"name":"a"}`, 204),
				{method: "POST", target: "/either", body: "name=a", contentType: "application/x-www-form-urlencoded", status: 204},
			},
			expect: func(f *findings, s spec) {
				if s.requestSchema("POST", "/either", "application/json") == nil || s.requestSchema("POST", "/either", "application/x-www-form-urlencoded") == nil {
					f.add("both media types should be documented: %v", s.op("POST", "/either")["requestBody"])
				}
			}},
		{id: "B07", area: "Bodies", title: "text/plain body",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Post("/note", func(c *zinc.Context) error {
					var s string
					if err := c.Bind().Text(&s); err != nil {
						return err
					}
					return c.NoContent()
				}).Input("")
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "POST", target: "/note", body: "hello", contentType: "text/plain", status: 204}},
			expect: func(f *findings, s spec) {
				if s.requestSchema("POST", "/note", "text/plain") == nil {
					f.add("a text/plain body is documented as %v", s.op("POST", "/note")["requestBody"])
				}
			}},
		{id: "B08", area: "Bodies", title: "XML body",
			build:  typedPost[xmlPet]("/x", zinc.Config{}),
			probes: []probe{{method: "POST", target: "/x", body: "<xmlPet><name>a</name></xmlPet>", contentType: "application/xml", status: 204}},
			expect: func(f *findings, s spec) {
				if s.requestSchema("POST", "/x", "application/xml") == nil {
					f.add("the route accepts XML, but only JSON is documented")
				}
			}},
		{id: "B09", area: "Bodies", title: "Custom decoder (YAML-like)",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New(zinc.Config{Decoders: map[string]zinc.Decoder{"application/x-kv": func(b []byte, v any) error { return nil }}})
				app.Post("/kv", zinc.Typed(func(_ *zinc.Context, in newPet) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "POST", target: "/kv", body: "name=x", contentType: "application/x-kv", status: 204}},
			expect: func(f *findings, s spec) {
				if s.requestSchema("POST", "/kv", "application/x-kv") == nil {
					f.add("configured decoders' media types aren't documented")
				}
			}},
		{id: "B10", area: "Bodies", title: "GET route whose input has JSON fields",
			build:  typedGet[getWithBody]("/search", zinc.Config{}),
			probes: []probe{get("/search")},
			expect: func(f *findings, s spec) {
				if s.op("GET", "/search")["requestBody"] != nil {
					f.add("GET should have no documented body")
				}
			},
			note: "Binding still decodes a body sent with GET; the spec leaves it out on purpose."},
		{id: "B11", area: "Bodies", title: "DELETE with a body",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Delete("/pets/{id}", zinc.Typed(func(_ *zinc.Context, in deleteWithBody) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "DELETE", target: "/pets/1", body: `{"reason":"sold"}`, status: 204}},
			expect: func(f *findings, s spec) {
				if s.requestSchema("DELETE", "/pets/{id}", "application/json") == nil {
					f.add("DELETE body missing")
				}
			}},
		{id: "B12", area: "Bodies", title: "No input (struct{})",
			build:  typedPost[struct{}]("/ping", zinc.Config{}),
			probes: []probe{{method: "POST", target: "/ping", status: 204}},
			expect: func(f *findings, s spec) {
				if s.op("POST", "/ping")["requestBody"] != nil {
					f.add("struct{} input should have no body")
				}
				if s.response("POST", "/ping", 400) != nil {
					f.add("no input, so no 400")
				}
			}},
		{id: "B13", area: "Bodies", title: "Required body (validator), sent empty",
			build:  typedPost[newPet]("/pets", validated()),
			probes: []probe{{method: "POST", target: "/pets", status: 422}},
			expect: func(f *findings, s spec) {
				rb, _ := s.op("POST", "/pets")["requestBody"].(map[string]any)
				if rb["required"] != true {
					f.add("requestBody.required should be true")
				}
			}},
		{id: "B14", area: "Bodies", title: "Request field with ,string",
			build:  typedPost[stringified]("/n", zinc.Config{}),
			probes: []probe{post("/n", `{"n":"5"}`, 204)}},
		{id: "B15", area: "Bodies", title: "PATCH with pointer fields",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Patch("/pets/{id}", zinc.Typed(func(_ *zinc.Context, in patchPet) (zinc.NoContent, error) { return zinc.NoContent{}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{{method: "PATCH", target: "/pets/1", body: `{"name":null}`, status: 204}, {method: "PATCH", target: "/pets/1", body: `{"age":3}`, status: 204}}},
		{id: "B16", area: "Bodies", title: "Unknown fields in a request",
			build:  typedPost[newPet]("/pets", validated()),
			probes: []probe{post("/pets", `{"name":"Rex","colour":"brown"}`, 204)},
			note:   "encoding/json ignores unknown fields, so the schema mustn't forbid them."},
	}
}
