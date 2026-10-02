// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"math/big"
	"net/netip"
	"reflect"
	"strings"
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
		{reflect.TypeFor[int8](), `{"type":"integer","format":"int32","minimum":-128,"maximum":127}`},
		{reflect.TypeFor[uint8](), `{"type":"integer","format":"int32","minimum":0,"maximum":255}`},
		{reflect.TypeFor[uint32](), `{"type":"integer","format":"int64","minimum":0,"maximum":4294967295}`},
		{reflect.TypeFor[uint64](), `{"type":"integer","minimum":0}`}, // no format holds it
		{reflect.TypeFor[float32](), `{"type":"number","format":"float"}`},
		{reflect.TypeFor[float64](), `{"type":"number","format":"double"}`},
		{reflect.TypeFor[[]string](), `{"type":["array","null"],"items":{"type":"string"}}`}, // a nil slice is null
		{reflect.TypeFor[[3]int](), `{"type":"array","items":{"type":"integer","format":"int64"},"minItems":3,"maxItems":3}`},
		{reflect.TypeFor[[]byte](), `{"type":["string","null"],"contentEncoding":"base64"}`},
		{reflect.TypeFor[map[string]int](), `{"type":["object","null"],"additionalProperties":{"type":"integer","format":"int64"}}`},
		{reflect.TypeFor[map[int]bool](), `{"type":["object","null"],"additionalProperties":{"type":"boolean"}}`},
		{reflect.TypeFor[*string](), `{"type":["string","null"]}`},
		{reflect.TypeFor[any](), `{}`},
		{reflect.TypeFor[*any](), `{}`},
		{reflect.TypeFor[time.Time](), `{"type":"string","format":"date-time"}`},
		{reflect.TypeFor[time.Duration](), `{"type":"integer","format":"int64","description":"nanoseconds"}`},
		{reflect.TypeFor[json.RawMessage](), `{}`},
		{reflect.TypeFor[json.Number](), `{"type":"number"}`},
		{reflect.TypeFor[netip.Addr](), `{"type":"string"}`}, // a TextMarshaler
	} {
		if got := schemaJSON(t, newSchemaGen().schemaFor(tt.typ)); got != tt.want {
			t.Errorf("%v:\n got %s\nwant %s", tt.typ, got, tt.want)
		}
	}
	// A request sends a list or an object; only a response may carry the
	// null of a nil slice or map.
	if got := schemaJSON(t, newSchemaGen().inputSchemaFor(reflect.TypeFor[[]string]())); got != `{"type":"array","items":{"type":"string"}}` {
		t.Errorf("input []string: %s", got)
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
		`"tags":{"type":["array","null"],"items":{"type":"string"},"maxItems":5},` +
		`"home":{"$ref":"#/components/schemas/schemaAddress","description":"Where they live."},` +
		`"work":{"anyOf":[{"$ref":"#/components/schemas/schemaAddress"},{"type":"null"}]},` +
		`"count":{"type":"string"},` +
		`"created_at":{"type":"string","format":"date-time"},` +
		`"friends":{"type":["array","null"],"items":{"anyOf":[{"$ref":"#/components/schemas/schemaUser"},{"type":"null"}]}}` +
		`},"required":["id","email","age","role","tags","home","work","count","created_at","friends"]}`
	if got := schemaJSON(t, g.components["schemaUser"]); got != want {
		t.Fatalf("component:\n got %s\nwant %s", got, want)
	}
	// A response always carries a field without omitempty, so the output
	// schema requires it; a request has to carry validate:"required", and a
	// field whose rules reject its zero value, such as min=1 or oneof.
	if got := schemaJSON(t, g.inputSchemaFor(reflect.TypeFor[schemaUser]())); got != `{"$ref":"#/components/schemas/schemaUserInput"}` {
		t.Fatalf("input ref: %s", got)
	}
	if got := string(mustJSON(t, g.components["schemaUserInput"].required)); got != `["email","name","role"]` {
		t.Fatalf("input required: %s", got)
	}
	// schemaAddress's city has no omitempty, so responses require it and
	// requests don't: two components.
	if g.components["schemaAddressInput"] == nil {
		t.Fatal("schemaAddress's input schema differs (city is required only in responses) but has no component")
	}
	if got := schemaJSON(t, g.components["schemaAddress"]); got != `{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}` {
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
		{schemaEmbedConflict{}, `{"type":"object","properties":{"B":{"type":"string"}},"required":["B"]}`},
		{schemaEmbedTagged{}, `{"type":"object","properties":{"A":{"type":"integer","format":"int64"},"a":{"type":"integer","format":"int64"}},"required":["A","a"]}`},
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

type schemaBase struct {
	ID int `json:"id"`
}

type schemaNilable struct {
	*schemaBase
	List    []int          `json:"list"`
	Maybe   []int          `json:"maybe,omitempty"`
	Zero    map[string]int `json:"zero,omitzero"`
	Payload json.Number    `json:"payload"`
}

// A response schema says what encoding/json can write: null for a nil slice
// or map, unless omitempty or omitzero drops it, and no promoted fields when
// an embedded pointer is nil.
func TestSchemaFollowsEncodingJSONNils(t *testing.T) {
	g := newSchemaGen()
	g.schemaFor(reflect.TypeFor[schemaNilable]())
	want := `{"type":"object","properties":{"id":{"type":"integer","format":"int64"},"list":{"type":["array","null"],"items":{"type":"integer","format":"int64"}},"maybe":{"type":"array","items":{"type":"integer","format":"int64"}},"zero":{"type":"object","additionalProperties":{"type":"integer","format":"int64"}},"payload":{"type":"number"}},"required":["list","payload"]}`
	if got := schemaJSON(t, g.components["schemaNilable"]); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	encoded, _ := json.Marshal(schemaNilable{Payload: "1.5"})
	if string(encoded) != `{"list":null,"payload":1.5}` {
		t.Fatalf("encoding/json writes %s", encoded)
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
	if got := schemaJSON(t, g.components["schemaCreate"]); got != `{"type":"object","properties":{"Org":{"type":"string"},"DryRun":{"type":"boolean"},"Tenant":{"type":"string"},"name":{"type":"string"}},"required":["Org","DryRun","Tenant","name"]}` {
		t.Fatalf("full component: %s", got)
	}
	// A type with no parameter fields uses its input component as the body.
	if got := schemaJSON(t, g.bodySchemaFor(reflect.TypeFor[schemaAddress]())); got != `{"$ref":"#/components/schemas/schemaAddressInput"}` {
		t.Fatalf("plain body: %s", got)
	}
	// Where input and output agree, they share one component.
	type allRequired struct {
		N int `json:"n" validate:"required"`
	}
	g2 := newSchemaGen()
	if in, out := schemaJSON(t, g2.inputSchemaFor(reflect.TypeFor[allRequired]())), schemaJSON(t, g2.schemaFor(reflect.TypeFor[allRequired]())); in != out {
		t.Fatalf("identical schemas got two components: %s and %s", in, out)
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
	for _, name := range []string{"schemaPageSchemaUser", "schemaNode", "schemaUser", "schemaAddress"} {
		if g.components[name] == nil {
			t.Fatalf("missing component %q; have %v", name, keysOf(g.components))
		}
	}
	if got := schemaJSON(t, g.components["schemaNode"]); got != `{"type":"object","properties":{"children":{"type":["array","null"],"items":{"$ref":"#/components/schemas/schemaNode"}}},"required":["children"]}` {
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

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
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
		"User":                                        "User",
		"Page[example.com/shop.User]":                 "PageUser",
		"Pair[int,github.com/a/b.Thing]":              "PairIntThing",
		"Map[string,map[string]example.com/x.Y]":      "MapStringMapStringY",
		"Page[[]*example.com/shop.User]":              "PageListUser",
		"Box[[4]int]":                                 "BoxArrayInt",
		"Page[example.com/x.Box[example.com/y.Item]]": "PageBoxItem",
		"Wrap[github.com/satori/go.uuid.UUID]":        "WrapUUID",
	} {
		if got := sanitizeComponentName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// With the validator's omitempty, a zero value skips the other rules, so the
// schema must accept it too. Without it, the zero value fails, so a request
// must send the field: code is required.
func TestSchemaValidateOmitEmptyAllowsZero(t *testing.T) {
	type pet struct {
		Kind  string `json:"kind" validate:"omitempty,oneof=cat dog"`
		Email string `json:"email" validate:"omitempty,email,min=3,max=80"`
		Age   int    `json:"age" validate:"omitempty,min=1,max=30"`
		Size  int    `json:"size" validate:"omitempty,oneof=1 2"`
		Tags  []int  `json:"tags" validate:"omitempty,min=1"`
		Code  string `json:"code" validate:"oneof=a b"`
	}
	g := newSchemaGen()
	ref := schemaJSON(t, g.inputSchemaFor(reflect.TypeFor[pet]()))
	name := strings.TrimPrefix(strings.Trim(ref, `{}"`), `$ref":"#/components/schemas/`)
	want := `{"type":"object","properties":{"kind":{"type":"string","enum":["cat","dog",""]},"email":{"type":"string","maxLength":80},"age":{"type":"integer","format":"int64","maximum":30},"size":{"type":"integer","format":"int64","enum":[1,2,0]},"tags":{"type":"array","items":{"type":"integer","format":"int64"}},"code":{"type":"string","enum":["a","b"]}},"required":["code"]}`
	if got := schemaJSON(t, g.components[name]); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

type schemaLevel int

func (schemaLevel) Enum() []any { return []any{1, 2, 3} }

type schemaMoneyAmount struct{ Units int64 }

func TestSchemaEnumsAndTypeMap(t *testing.T) {
	type holder struct {
		Level  schemaLevel       `json:"level"`
		Levels []schemaLevel     `json:"levels"`
		Size   string            `json:"size" enum:"s, m, l"`
		Sizes  []int             `json:"sizes" enum:"1,2"`
		Price  schemaMoneyAmount `json:"price"`
		Big    *big.Int          `json:"big"`
	}
	g := newSchemaGen()
	g.types = map[reflect.Type]map[string]any{reflect.TypeFor[schemaMoneyAmount](): {"type": "string", "format": "decimal"}}
	g.schemaFor(reflect.TypeFor[holder]())
	want := `{"type":"object","properties":{"level":{"$ref":"#/components/schemas/schemaLevel"},"levels":{"type":["array","null"],"items":{"$ref":"#/components/schemas/schemaLevel"}},"size":{"type":"string","enum":["s","m","l"]},"sizes":{"type":["array","null"],"items":{"type":"integer","format":"int64","enum":[1,2]}},"price":{"format":"decimal","type":"string"},"big":{"type":["integer","null"]}},"required":["level","levels","size","sizes","price","big"]}`
	if got := schemaJSON(t, g.components["holder"]); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
	if got := schemaJSON(t, g.components["schemaLevel"]); got != `{"type":"integer","enum":[1,2,3]}` {
		t.Errorf("enum component: %s", got)
	}
}
