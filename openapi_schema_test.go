// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

func schemaJSON(t *testing.T, s *schema) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSchemaScalarsAndContainers(t *testing.T) {
	type named string
	for _, tt := range []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeFor[bool](), `{"type":"boolean"}`},
		{reflect.TypeFor[string](), `{"type":"string"}`},
		{reflect.TypeFor[named](), `{"type":"string"}`},
		{reflect.TypeFor[int](), `{"type":"integer","format":"int64"}`},
		{reflect.TypeFor[int32](), `{"type":"integer","format":"int32"}`},
		{reflect.TypeFor[uint8](), `{"type":"integer","format":"int32","minimum":0}`},
		{reflect.TypeFor[uint64](), `{"type":"integer","format":"int64","minimum":0}`},
		{reflect.TypeFor[float32](), `{"type":"number","format":"float"}`},
		{reflect.TypeFor[float64](), `{"type":"number","format":"double"}`},
		{reflect.TypeFor[[]string](), `{"type":"array","items":{"type":"string"}}`},
		{reflect.TypeFor[[3]int](), `{"type":"array","items":{"type":"integer","format":"int64"},"minItems":3,"maxItems":3}`},
		{reflect.TypeFor[[]byte](), `{"type":"string","contentEncoding":"base64"}`},
		{reflect.TypeFor[map[string]int](), `{"type":"object","additionalProperties":{"type":"integer","format":"int64"}}`},
		{reflect.TypeFor[map[int]bool](), `{"type":"object","additionalProperties":{"type":"boolean"}}`},
		{reflect.TypeFor[*string](), `{"type":["string","null"]}`},
		{reflect.TypeFor[any](), `{}`},
		{reflect.TypeFor[*any](), `{}`},
		{reflect.TypeFor[time.Time](), `{"type":"string","format":"date-time"}`},
		{reflect.TypeFor[time.Duration](), `{"type":"integer","format":"int64","description":"nanoseconds"}`},
		{reflect.TypeFor[json.RawMessage](), `{}`},
		{reflect.TypeFor[netip.Addr](), `{"type":"string"}`}, // a TextMarshaler
	} {
		if got := schemaJSON(t, newSchemaGen().schemaFor(tt.typ)); got != tt.want {
			t.Errorf("%v:\n got %s\nwant %s", tt.typ, got, tt.want)
		}
	}
}

type schemaAddress struct {
	City string `json:"city"`
}

type schemaUser struct {
	ID       int64          `json:"id" doc:"The user's ID." example:"42"`
	Email    string         `json:"email" validate:"required,email"`
	Name     string         `json:"name,omitempty" validate:"min=1,max=80"`
	Age      *int           `json:"age" validate:"gte=0,lt=150"`
	Role     string         `json:"role" validate:"oneof=admin member"`
	Tags     []string       `json:"tags" validate:"max=5,dive,min=1"`
	Home     schemaAddress  `json:"home" doc:"Where they live."`
	Work     *schemaAddress `json:"work"`
	Count    int            `json:"count,string"`
	Created  time.Time      `json:"created_at"`
	internal string
	Skipped  string        `json:"-"`
	Callback func()        `json:"cb"`
	Friends  []*schemaUser `json:"friends"` // recursive
}

// Unexported fields aren't encoded, so they aren't in the schema.
var _ = schemaUser{internal: "not in the schema"}

func TestSchemaStructComponent(t *testing.T) {
	g := newSchemaGen()
	if got := schemaJSON(t, g.schemaFor(reflect.TypeFor[schemaUser]())); got != `{"$ref":"#/components/schemas/schemaUser"}` {
		t.Fatalf("ref: %s", got)
	}
	want := `{"type":"object","properties":{` +
		`"id":{"type":"integer","format":"int64","description":"The user's ID.","examples":[42]},` +
		`"email":{"type":"string","format":"email"},` +
		`"name":{"type":"string","minLength":1,"maxLength":80},` +
		`"age":{"type":["integer","null"],"format":"int64","minimum":0,"exclusiveMaximum":150},` +
		`"role":{"type":"string","enum":["admin","member"]},` +
		`"tags":{"type":"array","items":{"type":"string"},"maxItems":5},` +
		`"home":{"$ref":"#/components/schemas/schemaAddress","description":"Where they live."},` +
		`"work":{"anyOf":[{"$ref":"#/components/schemas/schemaAddress"},{"type":"null"}]},` +
		`"count":{"type":"string"},` +
		`"created_at":{"type":"string","format":"date-time"},` +
		`"friends":{"type":"array","items":{"anyOf":[{"$ref":"#/components/schemas/schemaUser"},{"type":"null"}]}}` +
		`},"required":["email"]}`
	if got := schemaJSON(t, g.components["schemaUser"]); got != want {
		t.Fatalf("component:\n got %s\nwant %s", got, want)
	}
	if got := schemaJSON(t, g.components["schemaAddress"]); got != `{"type":"object","properties":{"city":{"type":"string"}}}` {
		t.Fatalf("address: %s", got)
	}
}

type schemaInner struct{ A, B int }
type schemaInner2 struct{ A int }
type schemaTagged struct {
	X int `json:"a"`
}
type schemaEmbedConflict struct {
	schemaInner
	schemaInner2        // A clashes with schemaInner.A at the same depth: both dropped
	B            string // beats schemaInner.B
}
type schemaEmbedTagged struct {
	schemaInner2
	schemaTagged
}

// The generated field sets match what encoding/json writes.
func TestSchemaEmbeddingFollowsEncodingJSON(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  string
	}{
		{schemaEmbedConflict{}, `{"type":"object","properties":{"B":{"type":"string"}}}`},
		{schemaEmbedTagged{}, `{"type":"object","properties":{"A":{"type":"integer","format":"int64"},"a":{"type":"integer","format":"int64"}}}`},
	} {
		typ := reflect.TypeOf(tt.value)
		g := newSchemaGen()
		g.schemaFor(typ)
		if got := schemaJSON(t, g.components[typ.Name()]); got != tt.want {
			t.Errorf("%v:\n got %s\nwant %s", typ, got, tt.want)
		}
		encoded, _ := json.Marshal(tt.value)
		var keys map[string]any
		_ = json.Unmarshal(encoded, &keys)
		for _, p := range g.components[typ.Name()].properties {
			if _, ok := keys[p.name]; !ok {
				t.Errorf("%v: schema has %q, encoding/json doesn't write it: %s", typ, p.name, encoded)
			}
		}
		if len(keys) != len(g.components[typ.Name()].properties) {
			t.Errorf("%v: encoding/json writes %s", typ, encoded)
		}
	}
}

type schemaCreate struct {
	Org    string `path:"org"`
	DryRun bool   `query:"dry_run"`
	Tenant string `header:"X-Tenant"`
	Name   string `json:"name" validate:"required"`
}

func TestSchemaBodyLeavesOutParameters(t *testing.T) {
	g := newSchemaGen()
	body := schemaJSON(t, g.bodySchemaFor(reflect.TypeFor[schemaCreate]()))
	full := schemaJSON(t, g.schemaFor(reflect.TypeFor[schemaCreate]()))
	if body != `{"$ref":"#/components/schemas/schemaCreateBody"}` || full != `{"$ref":"#/components/schemas/schemaCreate"}` {
		t.Fatalf("refs: body %s, full %s", body, full)
	}
	if got := schemaJSON(t, g.components["schemaCreateBody"]); got != `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}` {
		t.Fatalf("body component: %s", got)
	}
	if got := schemaJSON(t, g.components["schemaCreate"]); got != `{"type":"object","properties":{"Org":{"type":"string"},"DryRun":{"type":"boolean"},"Tenant":{"type":"string"},"name":{"type":"string"}},"required":["name"]}` {
		t.Fatalf("full component: %s", got)
	}
	// A type with no parameter fields uses one component for both.
	if got := schemaJSON(t, g.bodySchemaFor(reflect.TypeFor[schemaAddress]())); got != `{"$ref":"#/components/schemas/schemaAddress"}` {
		t.Fatalf("plain body: %s", got)
	}
}

type schemaDate struct{ time.Time }

func (schemaDate) OpenAPISchema() map[string]any {
	return map[string]any{"type": "string", "format": "date"}
}

type schemaMoney struct{ cents int64 }

func (m schemaMoney) MarshalJSON() ([]byte, error) { return json.Marshal(float64(m.cents) / 100) }

func TestSchemaProvidersAndCustomJSON(t *testing.T) {
	g := newSchemaGen()
	if got := schemaJSON(t, g.schemaFor(reflect.TypeFor[schemaDate]())); got != `{"format":"date","type":"string"}` {
		t.Fatalf("provider: %s", got)
	}
	if got := schemaJSON(t, g.schemaFor(reflect.TypeFor[*schemaDate]())); got != `{"anyOf":[{"format":"date","type":"string"},{"type":"null"}]}` {
		t.Fatalf("pointer to provider: %s", got)
	}
	// Custom MarshalJSON without a provider: the shape is unknown, so any.
	if got := schemaJSON(t, g.schemaFor(reflect.TypeFor[schemaMoney]())); got != `{}` {
		t.Fatalf("custom JSON: %s", got)
	}
}

type schemaPage[T any] struct {
	Items []T `json:"items"`
}

type schemaNode struct {
	Children []schemaNode `json:"children"`
}

func TestSchemaNamesAndRecursion(t *testing.T) {
	g := newSchemaGen()
	g.schemaFor(reflect.TypeFor[schemaPage[schemaUser]]())
	g.schemaFor(reflect.TypeFor[schemaNode]())
	for _, name := range []string{"schemaPage_zinc.schemaUser", "schemaNode", "schemaUser", "schemaAddress"} {
		if g.components[name] == nil {
			t.Fatalf("missing component %q; have %v", name, keysOf(g.components))
		}
	}
	if got := schemaJSON(t, g.components["schemaNode"]); got != `{"type":"object","properties":{"children":{"type":"array","items":{"$ref":"#/components/schemas/schemaNode"}}}}` {
		t.Fatalf("recursive: %s", got)
	}

	// A second type named schemaAddress, here a local one, gets its package
	// added to its name instead of overwriting the first.
	type schemaAddress struct {
		Street string `json:"street"`
	}
	local := schemaJSON(t, g.schemaFor(reflect.TypeFor[schemaAddress]()))
	if local != `{"$ref":"#/components/schemas/zinc.schemaAddress"}` {
		t.Fatalf("same-name type: %s; have %v", local, keysOf(g.components))
	}
}

func keysOf(m map[string]*schema) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestSanitizeComponentName(t *testing.T) {
	for in, want := range map[string]string{
		"User":                                   "User",
		"Page[example.com/shop.User]":            "Page_shop.User",
		"Pair[int,github.com/a/b.Thing]":         "Pair_int_b.Thing",
		"Map[string,map[string]example.com/x.Y]": "Map_string_map_string_x.Y",
	} {
		if got := sanitizeComponentName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
