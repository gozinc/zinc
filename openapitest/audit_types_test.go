// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/netip"
	"time"

	"github.com/0mjs/zinc"
	"github.com/google/uuid"
)

// serveValue is an app with one typed GET route that returns value.
func serveValue[Out any](path string, value Out) func() (*zinc.App, zinc.OpenAPIConfig) {
	return func() (*zinc.App, zinc.OpenAPIConfig) {
		app := zinc.New()
		app.Get(path, zinc.Typed(func(*zinc.Context, struct{}) (Out, error) { return value, nil }))
		return app, zinc.OpenAPIConfig{}
	}
}

func get(path string) probe { return probe{method: "GET", target: path, status: 200} }

type scalars struct {
	I   int     `json:"i"`
	I8  int8    `json:"i8"`
	I16 int16   `json:"i16"`
	I32 int32   `json:"i32"`
	I64 int64   `json:"i64"`
	U   uint    `json:"u"`
	U8  uint8   `json:"u8"`
	U16 uint16  `json:"u16"`
	U32 uint32  `json:"u32"`
	U64 uint64  `json:"u64"`
	F32 float32 `json:"f32"`
	F64 float64 `json:"f64"`
	B   bool    `json:"b"`
	S   string  `json:"s"`
}

type bigUnsigned struct {
	N uint64 `json:"n"`
}

type bytesHolder struct {
	Data  []byte  `json:"data"`
	Fixed [3]byte `json:"fixed"`
}

type times struct {
	At       time.Time     `json:"at"`
	Maybe    *time.Time    `json:"maybe"`
	Duration time.Duration `json:"duration"`
}

type maps struct {
	ByName map[string]int     `json:"by_name"`
	ByID   map[int]string     `json:"by_id"`
	ByAddr map[netip.Addr]int `json:"by_addr"`
}

type anything struct {
	Value any `json:"value"`
}

type page[T any] struct {
	Items []T `json:"items"`
	Next  int `json:"next"`
}

type item struct {
	ID int `json:"id"`
}

type tree struct {
	Name     string  `json:"name"`
	Children []*tree `json:"children"`
}

type nodeA struct {
	B *nodeB `json:"b"`
}

type nodeB struct {
	A *nodeA `json:"a"`
}

type base struct {
	CreatedAt string `json:"created_at"`
}

type withEmbedded struct {
	base
	Name string `json:"name"`
}

type audit struct {
	By string `json:"by"`
}

type withEmbeddedPointer struct {
	*audit
	Name string `json:"name"`
}

type asStrings struct {
	N int     `json:"n,string"`
	B bool    `json:"b,string"`
	F float64 `json:"f,string"`
}

type dashName struct {
	//lint:ignore SA5008 "-," names the field "-"; the scenario checks exactly that.
	Dash string `json:"-,"`
	Skip string `json:"-"`
}

type omitting struct {
	Always string `json:"always"`
	Empty  string `json:"empty,omitempty"`
	Zero   int    `json:"zero,omitzero"`
}

type nilSlice struct {
	Tags []string `json:"tags"`
}

type numbers struct {
	N json.Number     `json:"n"`
	R json.RawMessage `json:"r"`
}

type money struct{ cents int64 }

func (m money) MarshalJSON() ([]byte, error) { return json.Marshal(float64(m.cents) / 100) }

type described struct{ cents int64 }

func (d described) MarshalJSON() ([]byte, error) { return json.Marshal(float64(d.cents) / 100) }
func (described) OpenAPISchema() map[string]any {
	return map[string]any{"type": "number", "description": "An amount in dollars."}
}

type prices struct {
	Plain    money     `json:"plain"`
	Provided described `json:"provided"`
}

type ip struct {
	Addr netip.Addr `json:"addr"`
}

type kind string

const (
	kindCat kind = "cat"
	kindDog kind = "dog"
)

// Enum lists kind's values: Go can't list a type's constants at run time.
func (kind) Enum() []any { return []any{kindCat, kindDog} }

type withConstEnum struct {
	Kind kind `json:"kind"`
}

type withAnonymous struct {
	Meta struct {
		Count int `json:"count"`
	} `json:"meta"`
}

type hidden struct {
	Promoted string `json:"promoted"`
}

type withUnexportedEmbed struct {
	hidden
	Name string `json:"name"`
}

type nullable struct {
	Name sql.NullString `json:"name"`
}

type deep struct {
	Grid [][]map[string][]item `json:"grid"`
}

type skipped struct {
	Name string `json:"name"`
	//lint:ignore U1000 an unexported field the spec must leave out.
	internal string
	Fn       func() `json:"fn"`
	Ch       chan int
}

type pointerPointer struct {
	N **int `json:"n"`
}

type withUUID struct {
	ID uuid.UUID `json:"id"`
}

// Cookie shares its name with net/http's Cookie.
type Cookie struct {
	Flavour string `json:"flavour"`
}

type cookies struct {
	Ours   Cookie      `json:"ours"`
	Theirs http.Cookie `json:"theirs"`
}

func typeScenarios() []scenario {
	one := 1
	onePtr := &one
	return []scenario{
		{id: "T01", area: "Types", title: "Every scalar kind",
			build: serveValue("/v", scalars{}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				for _, name := range []string{"i", "i8", "i16", "i32", "i64", "u", "u8", "u16", "u32", "u64"} {
					if !typeIs(prop(sch, name), "integer") {
						f.add("%s isn't an integer: %v", name, prop(sch, name))
					}
				}
				for _, name := range []string{"f32", "f64"} {
					if !typeIs(prop(sch, name), "number") {
						f.add("%s isn't a number", name)
					}
				}
				if !typeIs(prop(sch, "b"), "boolean") || !typeIs(prop(sch, "s"), "string") {
					f.add("bool or string mistyped")
				}
				if len(required(sch)) != 14 {
					f.add("want all 14 fields required in a response, got %v", required(sch))
				}
			}},
		{id: "T02", area: "Types", title: "uint64 beyond int64",
			build: serveValue("/v", bigUnsigned{N: math.MaxUint64}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				n := prop(s.responseSchema("GET", "/v", 200), "n")
				if n["format"] == "int64" {
					f.add("uint64 is documented as format int64, which can't hold values above 2^63-1")
				}
			}},
		{id: "T03", area: "Types", title: "[]byte and [N]byte",
			build: serveValue("/v", bytesHolder{Data: []byte("hi"), Fixed: [3]byte{1, 2, 3}}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				if !typeIs(prop(sch, "data"), "string") || prop(sch, "data")["contentEncoding"] != "base64" {
					f.add("[]byte should be a base64 string: %v", prop(sch, "data"))
				}
				if !typeIs(prop(sch, "fixed"), "array") {
					f.add("[3]byte is a JSON array of numbers: %v", prop(sch, "fixed"))
				}
			}},
		{id: "T04", area: "Types", title: "time.Time, *time.Time and time.Duration",
			build: serveValue("/v", times{At: time.Unix(0, 0).UTC(), Duration: time.Second}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				if prop(sch, "at")["format"] != "date-time" {
					f.add("time.Time isn't date-time")
				}
				if !typeIs(prop(sch, "maybe"), "null") {
					f.add("*time.Time isn't nullable")
				}
				if !typeIs(prop(sch, "duration"), "integer") {
					f.add("time.Duration isn't an integer")
				}
			}},
		{id: "T05", area: "Types", title: "Maps with string, int and TextMarshaler keys",
			build:  serveValue("/v", maps{ByName: map[string]int{"a": 1}, ByID: map[int]string{1: "a"}, ByAddr: map[netip.Addr]int{netip.MustParseAddr("10.0.0.1"): 1}}),
			probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				for _, name := range []string{"by_name", "by_id", "by_addr"} {
					if !typeIs(prop(sch, name), "object") || prop(sch, name)["additionalProperties"] == nil {
						f.add("%s should be an object with additionalProperties", name)
					}
				}
			}},
		{id: "T06", area: "Types", title: "any fields",
			build: serveValue("/v", anything{Value: map[string]any{"x": 1}}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if v := prop(s.responseSchema("GET", "/v", 200), "value"); len(v) != 0 {
					f.add("any should be an unconstrained schema, got %v", v)
				}
			}},
		{id: "T07", area: "Types", title: "Generic types",
			build: serveValue("/v", page[item]{Items: []item{{ID: 1}}, Next: 2}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				ref, _ := s.op("GET", "/v")["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)["$ref"].(string)
				if ref == "" {
					f.add("generic instance isn't a component")
				}
				f.add("info: component names: %v", s.components())
			}},
		{id: "T08", area: "Types", title: "Self and mutual recursion",
			build: func() (*zinc.App, zinc.OpenAPIConfig) {
				app := zinc.New()
				app.Get("/tree", zinc.Typed(func(*zinc.Context, struct{}) (tree, error) {
					return tree{Name: "root", Children: []*tree{{Name: "leaf", Children: []*tree{}}}}, nil
				}))
				app.Get("/pair", zinc.Typed(func(*zinc.Context, struct{}) (nodeA, error) { return nodeA{B: &nodeB{}}, nil }))
				return app, zinc.OpenAPIConfig{}
			},
			probes: []probe{get("/tree"), get("/pair")}},
		{id: "T09", area: "Types", title: "Embedded struct, promoted fields",
			build: serveValue("/v", withEmbedded{base: base{CreatedAt: "now"}, Name: "x"}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				if prop(sch, "created_at") == nil || prop(sch, "name") == nil {
					f.add("promoted fields missing: %v", props(sch))
				}
			}},
		{id: "T10", area: "Types", title: "Embedded nil pointer (encoding/json drops its fields)",
			build: serveValue("/v", withEmbeddedPointer{Name: "x"}), probes: []probe{get("/v")},
			note: "A nil embedded pointer's promoted fields are left out of the JSON, so they can't be required."},
		{id: "T11", area: "Types", title: "The ,string option",
			build: serveValue("/v", asStrings{N: 5, B: true, F: 1.5}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				for _, name := range []string{"n", "b", "f"} {
					if !typeIs(prop(sch, name), "string") {
						f.add("%s with ,string should be a string", name)
					}
				}
			}},
		{id: "T12", area: "Types", title: `A field named "-" and one skipped with "-"`,
			build: serveValue("/v", dashName{Dash: "d", Skip: "s"}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				if prop(sch, "-") == nil || len(props(sch)) != 1 {
					f.add(`want exactly the property "-": %v`, props(sch))
				}
			}},
		{id: "T13", area: "Types", title: "omitempty and omitzero",
			build: serveValue("/v", omitting{Always: "a"}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				req := required(s.responseSchema("GET", "/v", 200))
				if !has(req, "always") || has(req, "empty") || has(req, "zero") {
					f.add("required should be [always]: %v", req)
				}
			}},
		{id: "T14", area: "Types", title: "A nil slice in a response",
			build: serveValue("/v", nilSlice{}), probes: []probe{get("/v")},
			note: "encoding/json sends a nil slice as null."},
		{id: "T15", area: "Types", title: "json.Number and json.RawMessage",
			build: serveValue("/v", numbers{N: "12.5", R: json.RawMessage(`{"x":1}`)}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if typeIs(prop(s.responseSchema("GET", "/v", 200), "n"), "string") {
					f.add("json.Number is sent as a JSON number but documented as a string")
				}
			}},
		{id: "T16", area: "Types", title: "Custom MarshalJSON, with and without SchemaProvider",
			build: serveValue("/v", prices{Plain: money{150}, Provided: described{150}}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				sch := s.responseSchema("GET", "/v", 200)
				if !typeIs(prop(sch, "provided"), "number") {
					f.add("SchemaProvider's schema wasn't used: %v", prop(sch, "provided"))
				}
				if len(prop(sch, "plain")) != 0 {
					f.add("info: custom MarshalJSON without a provider is documented as %v", prop(sch, "plain"))
				}
			}},
		{id: "T17", area: "Types", title: "TextMarshaler types (netip.Addr)",
			build: serveValue("/v", ip{Addr: netip.MustParseAddr("10.0.0.1")}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if !typeIs(prop(s.responseSchema("GET", "/v", 200), "addr"), "string") {
					f.add("netip.Addr should be a string")
				}
			}},
		{id: "T18", area: "Types", title: "Go const enums",
			build: serveValue("/v", withConstEnum{Kind: kindCat}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				_ = kindDog
				if s.resolve(prop(s.responseSchema("GET", "/v", 200), "kind"))["enum"] == nil {
					f.add("a named string type with const values isn't documented as an enum")
				}
			}},
		{id: "T19", area: "Types", title: "Anonymous struct field",
			build: serveValue("/v", withAnonymous{}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if prop(prop(s.responseSchema("GET", "/v", 200), "meta"), "count") == nil {
					f.add("anonymous struct's field missing")
				}
			}},
		{id: "T20", area: "Types", title: "Unexported embedded struct with exported fields",
			build: serveValue("/v", withUnexportedEmbed{hidden: hidden{Promoted: "p"}, Name: "n"}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if prop(s.responseSchema("GET", "/v", 200), "promoted") == nil {
					f.add("encoding/json promotes fields of an unexported embedded struct; the spec lost them")
				}
			}},
		{id: "T21", area: "Types", title: "sql.NullString (a plain struct)",
			build: serveValue("/v", nullable{Name: sql.NullString{String: "x", Valid: true}}), probes: []probe{get("/v")}},
		{id: "T22", area: "Types", title: "Deep nesting: [][]map[string][]T",
			build: serveValue("/v", deep{Grid: [][]map[string][]item{{{"a": {{ID: 1}}}}}}), probes: []probe{get("/v")}},
		{id: "T23", area: "Types", title: "Unexported, func and chan fields are skipped",
			build: serveValue("/v", skipped{Name: "n"}),
			expect: func(f *findings, s spec) {
				if p := props(s.responseSchema("GET", "/v", 200)); len(p) != 1 {
					f.add("want only name: %v", p)
				}
			}},
		{id: "T24", area: "Types", title: "Pointer to pointer",
			build: serveValue("/v", pointerPointer{N: &onePtr}), probes: []probe{get("/v")}},
		{id: "T25", area: "Types", title: "uuid.UUID",
			build: serveValue("/v", withUUID{ID: uuid.MustParse("7f1c0a52-7e4c-4d3b-9c2c-6c4f0f6d2a11")}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				id := prop(s.responseSchema("GET", "/v", 200), "id")
				if !typeIs(id, "string") {
					f.add("uuid.UUID should be a string")
				}
				if id["format"] != "uuid" {
					f.add("uuid.UUID isn't documented with format uuid")
				}
			}},
		{id: "T26", area: "Types", title: "Two types with the same name (Cookie and http.Cookie)",
			build: serveValue("/v", cookies{Ours: Cookie{Flavour: "oat"}, Theirs: http.Cookie{Name: "a", Value: "b"}}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				f.add("info: component names: %v", s.components())
			}},
		{id: "T27", area: "Types", title: "A top-level scalar output",
			build: serveValue("/v", "hello"), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if !typeIs(s.responseSchema("GET", "/v", 200), "string") {
					f.add("string output isn't a string schema")
				}
			}},
		{id: "T28", area: "Types", title: "A top-level map output",
			build: serveValue("/v", map[string]item{"a": {ID: 1}}), probes: []probe{get("/v")}},
		{id: "T29", area: "Types", title: "A top-level any output",
			build: serveValue[any]("/v", []any{1, "two"}), probes: []probe{get("/v")}},
		{id: "T30", area: "Types", title: "A user type named Error",
			build: serveValue("/v", Error{Reason: "a normal result"}), probes: []probe{get("/v")},
			expect: func(f *findings, s spec) {
				if prop(s.responseSchema("GET", "/v", 200), "reason") == nil {
					f.add("the user's Error schema was replaced by Zinc's error envelope")
				}
			}},
	}
}

// Error is an application type that shares its name with Zinc's error
// envelope component.
type Error struct {
	Reason string `json:"reason"`
}
