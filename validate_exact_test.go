// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// The rule these tests hold Zinc to: every nested value and every slice
// element gets its declared validation, and limits are exact.

type vxTree struct {
	Name  string   `json:"name" validate:"required"`
	Child *vxTree  `json:"child,omitempty"`
	Kids  []vxTree `json:"kids,omitempty"`
}

type vxA struct {
	Code string `json:"code" validate:"required"`
	B    *vxB   `json:"b,omitempty"`
}

type vxB struct {
	Size int  `json:"size" validate:"lte=3"`
	A    *vxA `json:"a,omitempty"`
}

// vxSend posts body to app and returns the status and
// the invalid fields.
func vxSend(t *testing.T, app *App, body string) (int, map[string]string) {
	t.Helper()
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	var out struct {
		Error struct{ Fields map[string]string } `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Error.Fields
}

func vxApp[In any]() *App {
	app := New()
	app.Post("/", Typed(func(*Context, In) (NoContent, error) { return NoContent{}, nil }))
	return app
}

func TestValidateRecursiveTypes(t *testing.T) {
	tree := vxApp[vxTree]()
	for _, tt := range []struct{ body, field string }{
		{`{"name":""}`, "name"},
		{`{"name":"r","child":{"name":""}}`, "child.name"},
		{`{"name":"r","child":{"name":"a","child":{"name":""}}}`, "child.child.name"},
		{`{"name":"r","child":{"name":"a","child":{"name":"b","child":{}}}}`, "child.child.child.name"},
		{`{"name":"r","kids":[{"name":"a"},{"name":"b","kids":[{"name":""}]}]}`, "kids[1].kids[0].name"},
		{`{"name":"r","child":{"name":"a","kids":[{"name":"b","child":{}}]}}`, "child.kids[0].child.name"},
	} {
		code, fields := vxSend(t, tree, tt.body)
		if code != 422 || fields[tt.field] != "is required" || len(fields) != 1 {
			t.Errorf("%s: %d %v, want %s", tt.body, code, fields, tt.field)
		}
	}
	for _, body := range []string{
		`{"name":"r"}`,
		`{"name":"r","child":{"name":"a","child":{"name":"b","child":{"name":"c"}}}}`,
		`{"name":"r","kids":[{"name":"a","kids":[{"name":"b"}]}]}`,
	} {
		if code, fields := vxSend(t, tree, body); code != 204 {
			t.Errorf("valid %s: %d %v", body, code, fields)
		}
	}
	// Types that refer to each other.
	mutual := vxApp[vxA]()
	if code, fields := vxSend(t, mutual, `{"code":"x","b":{"size":1,"a":{"code":"y","b":{"size":4}}}}`); code != 422 || fields["b.a.b.size"] != "must be at most 3" {
		t.Errorf("mutual: %d %v", code, fields)
	}
	if code, fields := vxSend(t, mutual, `{"code":"x","b":{"a":{"code":""}}}`); code != 422 || fields["b.a.code"] != "is required" {
		t.Errorf("mutual: %d %v", code, fields)
	}
	// A recursive type with rules only below the edge still has a plan, so
	// 422 is documented and the check runs.
	type leaf struct {
		Next *vxTree `json:"next"`
	}
	if rulePlanFor(reflect.TypeFor[leaf]()) == nil {
		t.Error("no plan for a type whose nested recursive type has rules")
	}
	type plainList struct {
		Name string     `json:"name"`
		Next *plainList `json:"next"`
	}
	if rulePlanFor(reflect.TypeFor[plainList]()) != nil {
		t.Error("a recursive type with nothing to check has a plan")
	}
}

// A Go value can point back at an ancestor, which JSON can't express. A
// struct met again on its own path is already being checked, so the walk
// ends there; everything else is checked wherever it's reached.
func TestValidateCycles(t *testing.T) {
	plan := rulePlanFor(reflect.TypeFor[vxTree]())
	check := func(v *vxTree) invalidFields {
		t.Helper()
		return plan.check(reflect.ValueOf(v).Elem(), true, "", nil)
	}
	root := &vxTree{Name: "r"}
	root.Child = root
	if errs := check(root); errs != nil {
		t.Errorf("self cycle: %v", errs)
	}
	bad := &vxTree{}
	bad.Child = bad
	root = &vxTree{Name: "r", Child: bad}
	if errs := check(root); len(errs) != 1 || errs["child.name"] != "is required" {
		t.Errorf("cycle below the root: %v", errs)
	}
	// Through a slice: the kids hold the root again.
	root = &vxTree{Name: "r"}
	root.Kids = []vxTree{{Name: "a"}, {}}
	root.Kids[0].Kids = root.Kids
	if errs := check(root); len(errs) != 2 || errs["kids[1].name"] != "is required" || errs["kids[0].kids[1].name"] != "is required" {
		t.Errorf("slice cycle: %v", errs)
	}
	// A long cycle, past the depth where the path is indexed.
	nodes := make([]vxTree, 100)
	for i := range nodes {
		nodes[i] = vxTree{Name: "n", Child: &nodes[(i+1)%len(nodes)]}
	}
	nodes[70].Name = ""
	if errs := check(&nodes[0]); len(errs) != 1 || errs[strings.Repeat("child.", 70)+"name"] != "is required" {
		t.Errorf("long cycle: %v", errs)
	}
	// Through a map: map values are copies, so the map itself is tracked.
	type node struct {
		Name string          `json:"name" validate:"required"`
		Next map[string]node `json:"next"`
	}
	m := map[string]node{}
	m["a"] = node{Name: "a", Next: m}
	m["b"] = node{Next: m}
	errs := rulePlanFor(reflect.TypeFor[node]()).check(reflect.ValueOf(node{Name: "r", Next: m}), true, "", nil)
	if len(errs) != 1 || errs["next[b].name"] != "is required" {
		t.Errorf("map cycle: %v", errs)
	}
}

// A deep JSON value is checked to its bottom.
func TestValidateDeepValues(t *testing.T) {
	const depth = 5000
	body := strings.Repeat(`{"name":"n","child":`, depth) + `{"name":""}` + strings.Repeat("}", depth)
	code, fields := vxSend(t, vxApp[vxTree](), body)
	if code != 422 || fields[strings.Repeat("child.", depth)+"name"] != "is required" {
		t.Errorf("deep: %d, %d fields", code, len(fields))
	}
}

func TestValidateRecursiveResponses(t *testing.T) {
	var got error
	app := New(Config{ValidateResponses: true, ErrorHandler: func(c *Context, err error) { got = err; DefaultErrorHandler(c, err) }})
	app.Get("/", Typed(func(*Context, struct{}) (vxTree, error) {
		return vxTree{Name: "r", Child: &vxTree{Name: "a", Child: &vxTree{}}}, nil
	}))
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || got == nil || !strings.HasSuffix(got.Error(), "response.child.child.name is required") {
		t.Errorf("%d %v", w.Code, got)
	}
}

type vxSize string

func (vxSize) Enum() []any { return []any{vxSize("s"), vxSize("m")} }

type vxPtrKind string

func (*vxPtrKind) Enum() []any { return []any{vxPtrKind("cat"), vxPtrKind("dog")} }

func TestValidateEnumElements(t *testing.T) {
	type in struct {
		Roles  []string          `json:"roles" enum:"reader,writer"`
		Pair   [2]int            `json:"pair" enum:"1,2"`
		Maybe  []*string         `json:"maybe" enum:"x"`
		Sizes  []vxSize          `json:"sizes"`
		BySize map[string]vxSize `json:"by_size"`
		Kind   vxPtrKind         `json:"kind"`
	}
	app := vxApp[in]()
	for _, tt := range []struct{ body, field, msg string }{
		{`{"roles":["administrator"]}`, "roles[0]", "must be one of: reader, writer"},
		{`{"roles":["reader","boss"]}`, "roles[1]", "must be one of: reader, writer"},
		{`{"roles":[""]}`, "roles[0]", "must be one of: reader, writer"},
		{`{"pair":[1,3]}`, "pair[1]", "must be one of: 1, 2"},
		{`{"maybe":[null,"y"]}`, "maybe[1]", "must be one of: x"},
		{`{"sizes":["s","xl"]}`, "sizes[1]", "must be one of: s, m"},
		{`{"by_size":{"a":"xl"}}`, "by_size[a]", "must be one of: s, m"},
		{`{"kind":"cow"}`, "kind", "must be one of: cat, dog"},
	} {
		code, fields := vxSend(t, app, tt.body)
		if code != 422 || fields[tt.field] != tt.msg {
			t.Errorf("%s: %d %v, want %s %q", tt.body, code, fields, tt.field, tt.msg)
		}
	}
	if code, fields := vxSend(t, app, `{"roles":["reader","writer"],"pair":[1,2],"maybe":[null,"x"],"sizes":["m"],"by_size":{"a":"s"},"kind":"cat"}`); code != 204 {
		t.Errorf("valid: %d %v", code, fields)
	}
	// Enums are Zinc's own claim, so a custom Validator doesn't change this.
	custom := New(Config{Validator: acceptAll{}})
	custom.Post("/", Typed(func(*Context, in) (NoContent, error) { return NoContent{}, nil }))
	if code, fields := vxSend(t, custom, `{"roles":["boss"]}`); code != 422 || fields["roles[0]"] == "" {
		t.Errorf("custom validator: %d %v", code, fields)
	}
}

// An enum tag whose values can't be parsed as the field's type fails at
// registration, like a bad pattern tag, whatever the validator.
func TestValidateEnumTagMistakes(t *testing.T) {
	type badInt struct {
		Count int `json:"count" enum:"1,not-an-integer"`
	}
	mustPanicWith(t, `enum and pattern tags can't be used: zinc.badInt.Count: enum value "not-an-integer" isn't an int`, func() {
		New().Post("/", Typed(func(*Context, badInt) (NoContent, error) { return NoContent{}, nil }))
	})
	mustPanicWith(t, `zinc.badInt.Count: enum value "not-an-integer"`, func() {
		New(Config{Validator: acceptAll{}}).Post("/", Typed(func(*Context, badInt) (NoContent, error) { return NoContent{}, nil }))
	})
	type badElem struct {
		IDs []uint `json:"ids" enum:"1,-1"`
	}
	mustPanicWith(t, `zinc.badElem.IDs: enum value "-1" isn't a uint`, func() {
		New().Post("/", Typed(func(*Context, badElem) (NoContent, error) { return NoContent{}, nil }))
	})
	type badMap struct {
		M map[string]string `json:"m" enum:"a"`
	}
	mustPanicWith(t, `zinc.badMap.M: enum applies to strings, numbers and booleans, and slices and arrays of them, not map[string]string`, func() {
		New().Post("/", Typed(func(*Context, badMap) (NoContent, error) { return NoContent{}, nil }))
	})
	type nested struct {
		Inner badInt `json:"inner"`
	}
	mustPanicWith(t, `zinc.badInt.Count: enum value`, func() {
		New().Post("/", Typed(func(*Context, nested) (NoContent, error) { return NoContent{}, nil }))
	})
	// A plain handler's input is reported by App.Validate.
	app := New()
	app.Post("/plain", func(c *Context) error { return nil }).Input(badInt{})
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), `POST /plain: zinc: these enum and pattern tags can't be used: zinc.badInt.Count: enum value "not-an-integer" isn't an int`) {
		t.Errorf("App.Validate: %v", err)
	}
}

// vxBoundType is a struct with one field N of type t and the validate tag
// tag, built at run time so every bound can be tried.
func vxBoundType(t reflect.Type, tag string) reflect.Type {
	return reflect.StructOf([]reflect.StructField{{Name: "N", Type: t, Tag: reflect.StructTag(`json:"n" validate:"` + tag + `"`)}})
}

func TestValidateBoundsAreExact(t *testing.T) {
	two53 := new(big.Int).Lsh(big.NewInt(1), 53)
	maxI := big.NewInt(math.MaxInt64)
	minI := big.NewInt(math.MinInt64)
	maxU := new(big.Int).SetUint64(math.MaxUint64)
	holds := map[string]func(c int) bool{
		"min": func(c int) bool { return c >= 0 }, "gte": func(c int) bool { return c >= 0 },
		"max": func(c int) bool { return c <= 0 }, "lte": func(c int) bool { return c <= 0 },
		"gt": func(c int) bool { return c > 0 }, "lt": func(c int) bool { return c < 0 },
		"len": func(c int) bool { return c == 0 },
	}
	for _, domain := range []struct {
		typ      reflect.Type
		lo, hi   *big.Int
		boundary []*big.Int
	}{
		{reflect.TypeFor[int64](), minI, maxI, []*big.Int{two53, new(big.Int).Neg(two53), maxI, minI}},
		{reflect.TypeFor[int](), minI, maxI, []*big.Int{two53, maxI, minI}},
		{reflect.TypeFor[uint64](), big.NewInt(0), maxU, []*big.Int{two53, maxU, maxI, big.NewInt(0)}},
		{reflect.TypeFor[uint](), big.NewInt(0), maxU, []*big.Int{two53, maxU}},
	} {
		for _, b := range domain.boundary {
			for name, ok := range holds {
				st := vxBoundType(domain.typ, name+"="+b.String())
				plan := rulePlanFor(st)
				if plan == nil || len(plan.unsupported) > 0 {
					t.Fatalf("%s %s=%s: plan %+v", domain.typ, name, b, plan)
				}
				for _, d := range []int64{-1, 0, 1} {
					n := new(big.Int).Add(b, big.NewInt(d))
					if n.Cmp(domain.lo) < 0 || n.Cmp(domain.hi) > 0 {
						continue
					}
					v := reflect.New(st).Elem()
					if domain.typ.Kind() == reflect.Int64 || domain.typ.Kind() == reflect.Int {
						v.Field(0).SetInt(n.Int64())
					} else {
						v.Field(0).SetUint(n.Uint64())
					}
					errs := plan.check(v, true, "", nil)
					if want := ok(n.Cmp(b)); (errs == nil) != want {
						t.Errorf("%s %s=%s, value %s: errors %v, want valid %v", domain.typ, name, b, n, errs, want)
					}
				}
			}
		}
	}
	// Lengths compare as integers.
	st := vxBoundType(reflect.TypeFor[string](), "max=3")
	v := reflect.New(st).Elem()
	v.Field(0).SetString("héé")
	if errs := rulePlanFor(st).check(v, true, "", nil); errs != nil {
		t.Errorf("3 characters, max=3: %v", errs)
	}
	// Over JSON, the exact integer reaches the check.
	type maxIn struct {
		N uint64 `json:"n" validate:"max=9007199254740992"`
	}
	type gtIn struct {
		N uint64 `json:"n" validate:"gt=9007199254740992"`
	}
	maxApp, gtApp := vxApp[maxIn](), vxApp[gtIn]()
	for _, tt := range []struct {
		n           string
		maxOK, gtOK bool
	}{
		{"9007199254740991", true, false},
		{"9007199254740992", true, false},
		{"9007199254740993", false, true},
	} {
		body := `{"n":` + tt.n + `}`
		if code, _ := vxSend(t, maxApp, body); (code == 204) != tt.maxOK {
			t.Errorf("max, %s: %d", tt.n, code)
		}
		if code, _ := vxSend(t, gtApp, body); (code == 204) != tt.gtOK {
			t.Errorf("gt, %s: %d", tt.n, code)
		}
	}
}

// A bound the field's type can't hold fails at registration rather than
// being rounded into another rule.
func TestValidateBoundMistakes(t *testing.T) {
	for _, tt := range []struct {
		typ  reflect.Type
		tag  string
		want string
	}{
		{reflect.TypeFor[uint64](), "min=-1", "min=-1 (the bound must be a whole number from 0 to 18446744073709551615)"},
		{reflect.TypeFor[int](), "max=1.5", "max=1.5 (the bound must be a whole number from -9223372036854775808 to 9223372036854775807)"},
		{reflect.TypeFor[int64](), "lt=9223372036854775808", "lt=9223372036854775808 (the bound must be a whole number"},
		{reflect.TypeFor[string](), "len=2.5", "len=2.5 (the bound must be a whole number"},
		{reflect.TypeFor[[]int](), "max=x", "max=x (the bound must be a whole number"},
		{reflect.TypeFor[float64](), "max=NaN", "max=NaN (the bound must be a finite number)"},
		{reflect.TypeFor[float32](), "gt=1e39", "gt=1e39 (the bound must be a finite number)"},
	} {
		st := vxBoundType(tt.typ, tt.tag)
		plan := rulePlanFor(st)
		if plan == nil || len(plan.unsupported) != 1 || !strings.Contains(plan.unsupported[0], tt.want) {
			t.Errorf("%s %s: %+v, want %q", tt.typ, tt.tag, plan, tt.want)
		}
	}
	type badBound struct {
		N uint8 `json:"n" validate:"min=-1"`
	}
	mustPanicWith(t, "zinc.badBound.N: min=-1 (the bound must be a whole number", func() {
		New().Post("/", Typed(func(*Context, badBound) (NoContent, error) { return NoContent{}, nil }))
	})
	// Floats compare as floats.
	st := vxBoundType(reflect.TypeFor[float64](), "lt=1.5")
	v := reflect.New(st).Elem()
	v.Field(0).SetFloat(1.4999)
	if errs := rulePlanFor(st).check(v, true, "", nil); errs != nil {
		t.Errorf("1.4999 < 1.5: %v", errs)
	}
	v.Field(0).SetFloat(1.5)
	if errs := rulePlanFor(st).check(v, true, "", nil); errs == nil {
		t.Error("1.5 < 1.5 passed")
	}
}

// The rules mean what they mean to go-playground/validator: a non-nil
// pointer counts as set, omitempty skips only the rules after it, and the
// RFC 4122 forms check the variant.
func TestValidatePlaygroundSemantics(t *testing.T) {
	type flag struct {
		On *bool `json:"on" validate:"required"`
	}
	app := vxApp[flag]()
	if code, fields := vxSend(t, app, `{"on":false}`); code != 204 {
		t.Errorf("non-nil false pointer, required: %d %v", code, fields)
	}
	if code, fields := vxSend(t, app, `{}`); code != 422 || fields["on"] != "is required" {
		t.Errorf("nil pointer, required: %d %v", code, fields)
	}
	type nick struct {
		Nick *string `json:"nick" validate:"omitempty,min=2"`
	}
	app = vxApp[nick]()
	if code, fields := vxSend(t, app, `{"nick":""}`); code != 422 || fields["nick"] != "must be at least 2 characters" {
		t.Errorf("non-nil empty pointer, omitempty,min=2: %d %v", code, fields)
	}
	if code, fields := vxSend(t, app, `{}`); code != 204 {
		t.Errorf("nil pointer, omitempty,min=2: %d %v", code, fields)
	}
	type order struct {
		First  string `json:"first" validate:"required,omitempty"`
		Second string `json:"second" validate:"omitempty,required"`
		Third  string `json:"third" validate:"min=2,omitempty,max=3"`
	}
	app = vxApp[order]()
	code, fields := vxSend(t, app, `{}`)
	if code != 422 || fields["first"] != "is required" || fields["third"] != "must be at least 2 characters" || len(fields) != 2 {
		t.Errorf("rule order: %d %v", code, fields)
	}
	type ids struct {
		RFC  string `json:"rfc" validate:"omitempty,uuid4_rfc4122"`
		V4   string `json:"v4" validate:"omitempty,uuid4"`
		Any  string `json:"any" validate:"omitempty,uuid"`
		AnyR string `json:"any_r" validate:"omitempty,uuid_rfc4122"`
	}
	app = vxApp[ids]()
	for _, tt := range []struct {
		field, id string
		ok        bool
	}{
		{"rfc", "550e8400-e29b-41d4-a716-446655440000", true},
		{"rfc", "550E8400-E29B-41D4-B716-446655440000", true},
		{"rfc", "550e8400-e29b-41d4-0716-446655440000", false},
		{"rfc", "550e8400-e29b-41d4-c716-446655440000", false},
		{"rfc", "550e8400-e29b-31d4-a716-446655440000", false},
		{"v4", "550e8400-e29b-41d4-8716-446655440000", true},
		{"v4", "550e8400-e29b-41d4-0716-446655440000", false},
		{"v4", "550E8400-E29B-41D4-A716-446655440000", false},
		{"any", "550e8400-e29b-41d4-0716-446655440000", true},
		{"any_r", "550E8400-e29b-11d4-0716-446655440000", true},
	} {
		code, fields := vxSend(t, app, fmt.Sprintf(`{%q:%q}`, tt.field, tt.id))
		if (code == 204) != tt.ok {
			t.Errorf("%s %s: %d %v, want valid %v", tt.field, tt.id, code, fields, tt.ok)
		}
	}
}

// The spec states what's enforced: exact integer bounds, an omitempty that
// relaxes only the rules after it, and none of it for a pointer, whose
// non-nil zero value the rules still check.
func TestSchemaAgreesWithValidation(t *testing.T) {
	type in struct {
		Big   uint64  `json:"big" validate:"max=9007199254740993"`
		Low   int64   `json:"low" validate:"gt=-9223372036854775807"`
		Third string  `json:"third" validate:"min=2,omitempty,max=3"`
		Nick  *string `json:"nick" validate:"omitempty,min=2"`
		Ratio float64 `json:"ratio" validate:"lt=0.5"`
	}
	g := newSchemaGen()
	ref := schemaJSON(t, g.inputSchemaFor(reflect.TypeFor[in]()))
	name := strings.TrimPrefix(strings.Trim(ref, `{}"`), `$ref":"#/components/schemas/`)
	want := `{"type":"object","properties":{"big":{"type":"integer","minimum":0,"maximum":9007199254740993},"low":{"type":"integer","format":"int64","exclusiveMinimum":-9223372036854775807},"third":{"type":"string","minLength":2,"maxLength":3},"nick":{"type":["string","null"],"minLength":2},"ratio":{"type":"number","format":"double","exclusiveMaximum":0.5}},"required":["third"]}`
	if got := schemaJSON(t, g.components[name]); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}
