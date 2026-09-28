// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SchemaProvider lets a type describe itself in the OpenAPI spec when the
// schema Zinc derives from its Go shape would be wrong, such as a type with a
// custom MarshalJSON. OpenAPISchema returns a JSON Schema 2020-12 object,
// such as map[string]any{"type": "string", "format": "date"}.
type SchemaProvider interface {
	OpenAPISchema() map[string]any
}

// schema is a JSON Schema 2020-12 object, as OpenAPI 3.1 uses it. Fields
// marshal in a fixed order, and properties keep their struct order, so the
// spec is byte-for-byte stable.
type schema struct {
	ref                  string
	typ                  []string
	format               string
	description          string
	contentEncoding      string
	contentMediaType     string
	properties           []property
	required             []string
	additionalProperties *schema
	items                *schema
	anyOf                []*schema
	enum                 []any
	examples             []any
	minimum, maximum     *float64
	exclusiveMinimum     *float64
	exclusiveMaximum     *float64
	minLength, maxLength *int
	minItems, maxItems   *int
	// raw replaces everything else: a SchemaProvider's own schema.
	raw map[string]any
}

type property struct {
	name   string
	schema *schema
}

var (
	schemaProviderType = reflect.TypeFor[SchemaProvider]()
	jsonMarshalerType  = reflect.TypeFor[json.Marshaler]()
	textMarshalerType  = reflect.TypeFor[encoding.TextMarshaler]()
	timeType           = reflect.TypeFor[time.Time]()
	durationType       = reflect.TypeFor[time.Duration]()
	rawMessageType     = reflect.TypeFor[json.RawMessage]()
)

// schemaGen turns Go types into schemas. Named structs become components,
// referenced with $ref, so a type used in many places is described once and
// recursive types terminate.
type schemaGen struct {
	// validation reports whether validate tags reach the schema. The spec
	// builder turns it off when the app has no Validator, since nothing would
	// enforce the rules.
	validation bool
	components map[string]*schema
	names      map[reflect.Type]string
	taken      map[string]reflect.Type
}

func newSchemaGen() *schemaGen {
	return &schemaGen{
		validation: true,
		components: map[string]*schema{},
		names:      map[reflect.Type]string{},
		taken:      map[string]reflect.Type{},
	}
}

// schemaFor returns the schema for values of t, as encoding/json writes them.
func (g *schemaGen) schemaFor(t reflect.Type) *schema {
	if s, ok := g.special(t); ok {
		return s
	}
	switch t.Kind() {
	case reflect.Pointer:
		inner := g.schemaFor(t.Elem())
		return nullable(inner)
	case reflect.Bool:
		return &schema{typ: []string{"boolean"}}
	case reflect.String:
		return &schema{typ: []string{"string"}}
	case reflect.Int8, reflect.Int16, reflect.Int32:
		return &schema{typ: []string{"integer"}, format: "int32"}
	case reflect.Uint8, reflect.Uint16:
		return &schema{typ: []string{"integer"}, format: "int32", minimum: ptr(0.0)}
	case reflect.Int, reflect.Int64:
		return &schema{typ: []string{"integer"}, format: "int64"}
	case reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &schema{typ: []string{"integer"}, format: "int64", minimum: ptr(0.0)}
	case reflect.Float32:
		return &schema{typ: []string{"number"}, format: "float"}
	case reflect.Float64:
		return &schema{typ: []string{"number"}, format: "double"}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 && !reflect.PointerTo(t.Elem()).Implements(textMarshalerType) {
			return &schema{typ: []string{"string"}, contentEncoding: "base64"}
		}
		return &schema{typ: []string{"array"}, items: g.schemaFor(t.Elem())}
	case reflect.Array:
		n := t.Len()
		return &schema{typ: []string{"array"}, items: g.schemaFor(t.Elem()), minItems: &n, maxItems: &n}
	case reflect.Map:
		return &schema{typ: []string{"object"}, additionalProperties: g.schemaFor(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return g.structSchema(t, false)
		}
		return &schema{ref: "#/components/schemas/" + g.component(t, false)}
	default:
		// Interfaces can hold anything; channels, funcs and complex numbers
		// don't encode, and struct fields of those kinds are skipped.
		return &schema{}
	}
}

// bodySchemaFor is schemaFor for a request body: a struct's path, query,
// header and form fields are parameters, not body properties, so they're left
// out. Binding reads the URL after the body, so a body key for one of them
// would be ignored anyway.
func (g *schemaGen) bodySchemaFor(t reflect.Type) *schema {
	if t.Kind() != reflect.Struct || !hasParamFields(t) {
		return g.schemaFor(t)
	}
	if t.Name() == "" {
		return g.structSchema(t, true)
	}
	return &schema{ref: "#/components/schemas/" + g.component(t, true)}
}

// special handles types whose JSON form isn't their Go shape.
func (g *schemaGen) special(t reflect.Type) (*schema, bool) {
	if provider := schemaProviderFor(t); provider != nil {
		return &schema{raw: provider.OpenAPISchema()}, true
	}
	switch {
	case t == timeType:
		return &schema{typ: []string{"string"}, format: "date-time"}, true
	case t == rawMessageType:
		return &schema{}, true
	case t == durationType:
		// time.Duration encodes as its integer nanoseconds.
		return &schema{typ: []string{"integer"}, format: "int64", description: "nanoseconds"}, true
	}
	if t.Kind() != reflect.Pointer && (t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType)) {
		return &schema{}, true // custom JSON: unknown shape without a SchemaProvider
	}
	if t.Kind() != reflect.Pointer && (t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)) {
		return &schema{typ: []string{"string"}}, true
	}
	return nil, false
}

// schemaProviderFor returns a zero value of t as a SchemaProvider, or nil.
// Pointer types return nil: they're described as their element plus null.
func schemaProviderFor(t reflect.Type) SchemaProvider {
	var v reflect.Value
	switch {
	case t.Kind() == reflect.Pointer:
		return nil
	case t.Implements(schemaProviderType):
		v = reflect.New(t).Elem()
	case reflect.PointerTo(t).Implements(schemaProviderType):
		v = reflect.New(t) // pointer-receiver methods need an addressable value
	default:
		return nil
	}
	provider, _ := v.Interface().(SchemaProvider)
	return provider
}

// component names t, building its schema the first time it's seen. The name
// is reserved before building, so a recursive type refers to itself.
func (g *schemaGen) component(t reflect.Type, body bool) string {
	key := t
	if body {
		// The body variant of an input struct is a different schema, so it
		// gets its own name: a type standing for itself can't be a map key
		// twice, so key it by a pointer-to-pointer type nobody else uses.
		key = reflect.PointerTo(reflect.PointerTo(t))
	}
	if name, ok := g.names[key]; ok {
		return name
	}
	name := g.uniqueName(t, body)
	g.names[key] = name
	g.taken[name] = key
	g.components[name] = g.structSchema(t, body)
	return name
}

// uniqueName is the type's name, sanitized for a component key, with its
// package added when another type already has the name.
func (g *schemaGen) uniqueName(t reflect.Type, body bool) string {
	suffix := ""
	if body {
		suffix = "Body"
	}
	name := sanitizeComponentName(t.Name()) + suffix
	if _, used := g.taken[name]; !used {
		return name
	}
	pkg := t.PkgPath()
	if i := strings.LastIndexByte(pkg, '/'); i >= 0 {
		pkg = pkg[i+1:]
	}
	name = sanitizeComponentName(pkg+"."+t.Name()) + suffix
	if _, used := g.taken[name]; !used {
		return name
	}
	base := sanitizeComponentName(t.PkgPath()+"."+t.Name()) + suffix
	name = base
	for i := 2; ; i++ {
		if _, used := g.taken[name]; !used {
			return name
		}
		name = base + strconv.Itoa(i)
	}
}

// sanitizeComponentName keeps the characters OpenAPI allows in component
// keys. Generic type arguments lose their package paths:
// Page[example.com/shop.User] becomes Page_shop.User.
func sanitizeComponentName(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '/':
			// Keep only the last path element of a qualified type argument.
			s := b.String()
			cut := strings.LastIndexAny(s, "_[")
			b.Reset()
			b.WriteString(s[:cut+1])
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
			b.WriteByte(c)
		default:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
				b.WriteByte('_')
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

// structSchema describes a struct's JSON object. With body set, fields bound
// from the path, query, headers or a form are left out.
func (g *schemaGen) structSchema(t reflect.Type, body bool) *schema {
	s := &schema{typ: []string{"object"}}
	for _, f := range jsonFields(t) {
		if body && f.param {
			continue
		}
		var fs *schema
		switch {
		case f.asString:
			fs = &schema{typ: []string{"string"}}
		default:
			fs = g.schemaFor(f.typ)
		}
		required := g.applyFieldTags(fs, f)
		if fs.ref != "" && (fs.description != "" || len(fs.examples) > 0) {
			// Keep the component clean: annotations sit beside the $ref.
			fs = &schema{ref: fs.ref, description: fs.description, examples: fs.examples}
		}
		s.properties = append(s.properties, property{name: f.name, schema: fs})
		if required {
			s.required = append(s.required, f.name)
		}
	}
	return s
}

// jsonField is a struct field as encoding/json sees it.
type jsonField struct {
	name     string
	typ      reflect.Type
	tag      reflect.StructTag
	asString bool
	param    bool
	depth    int
	tagged   bool
	index    []int
}

// jsonFields lists t's encoded fields with encoding/json's rules: exported
// fields, tag names, "-" skipped, embedded structs promoted, and at equal
// depth a tagged field beats untagged ones while a tie drops the name.
func jsonFields(t reflect.Type) []jsonField {
	var all []jsonField
	var walk func(t reflect.Type, depth int, index []int, visited map[reflect.Type]bool)
	walk = func(t reflect.Type, depth int, index []int, visited map[reflect.Type]bool) {
		if visited[t] {
			return
		}
		visited[t] = true
		defer delete(visited, t)
		for i := range t.NumField() {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			ft := f.Type
			if f.Anonymous {
				et := ft
				if et.Kind() == reflect.Pointer {
					et = et.Elem()
				}
				if name == "" && et.Kind() == reflect.Struct {
					walk(et, depth+1, append(slices.Clone(index), i), visited)
					continue
				}
				if !f.IsExported() && et.Kind() != reflect.Struct {
					continue
				}
			} else if !f.IsExported() {
				continue
			}
			switch ft.Kind() {
			case reflect.Chan, reflect.Func, reflect.Complex64, reflect.Complex128, reflect.UnsafePointer:
				continue
			}
			jf := jsonField{
				name:   name,
				typ:    ft,
				tag:    f.Tag,
				depth:  depth,
				tagged: name != "",
				index:  append(slices.Clone(index), i),
				// Binding reads only top-level fields, so a tagged field in
				// an embedded struct is still a body field.
				param: depth == 0 && isParamField(f.Tag),
			}
			if jf.name == "" {
				jf.name = f.Name
			}
			if hasOption(opts, "string") {
				switch base(ft).Kind() {
				case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
					reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
					reflect.Float32, reflect.Float64, reflect.String:
					jf.asString = true
				}
			}
			all = append(all, jf)
		}
	}
	walk(t, 0, nil, map[reflect.Type]bool{})

	// Resolve name conflicts the way encoding/json does.
	byName := map[string][]int{}
	for i, f := range all {
		byName[f.name] = append(byName[f.name], i)
	}
	keep := make([]bool, len(all))
	for _, idxs := range byName {
		best := all[idxs[0]].depth
		for _, i := range idxs {
			best = min(best, all[i].depth)
		}
		var top []int
		for _, i := range idxs {
			if all[i].depth == best {
				top = append(top, i)
			}
		}
		if len(top) == 1 {
			keep[top[0]] = true
			continue
		}
		var tagged []int
		for _, i := range top {
			if all[i].tagged {
				tagged = append(tagged, i)
			}
		}
		if len(tagged) == 1 {
			keep[tagged[0]] = true
		}
	}
	var out []jsonField
	for i, f := range all {
		if keep[i] {
			out = append(out, f)
		}
	}
	// encoding/json writes fields in index order.
	slices.SortStableFunc(out, func(a, b jsonField) int { return slices.Compare(a.index, b.index) })
	return out
}

func base(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func hasOption(opts, want string) bool {
	for opts != "" {
		var o string
		o, opts, _ = strings.Cut(opts, ",")
		if o == want {
			return true
		}
	}
	return false
}

// isParamField reports whether binding reads the field from outside the
// body. A tag value of "-" means no binding.
func isParamField(tag reflect.StructTag) bool {
	for _, key := range []string{"path", "query", "header", "form"} {
		if v, ok := tag.Lookup(key); ok && v != "-" {
			return true
		}
	}
	return false
}

func hasParamFields(t reflect.Type) bool {
	for _, f := range jsonFields(t) {
		if f.param {
			return true
		}
	}
	return false
}

// applyFieldTags adds what the doc, example and validate tags say to fs, and
// reports whether the field is required. Validate tags count only when g
// includes validation.
func (g *schemaGen) applyFieldTags(fs *schema, f jsonField) bool {
	if doc := f.tag.Get("doc"); doc != "" {
		fs.description = doc
	}
	target := fs
	if len(fs.anyOf) == 2 {
		target = fs.anyOf[0] // constraints apply to the non-null branch
	}
	if ex, ok := f.tag.Lookup("example"); ok {
		if v, ok := parseExample(ex, f.typ, f.asString); ok {
			fs.examples = []any{v}
		}
	}
	if !g.validation {
		return false
	}
	return applyValidateTag(target, f.tag.Get("validate"), base(f.typ), f.asString)
}

// applyValidateTag reads the go-playground validator tokens that have a
// JSON Schema meaning. Zinc doesn't depend on the validator; unknown tokens
// are ignored, and anything after "dive" describes elements, so it stops.
func applyValidateTag(s *schema, tag string, t reflect.Type, asString bool) (required bool) {
	if tag == "" {
		return false
	}
	// A $ref or a provider's schema is shared or fixed, so only "required",
	// which belongs to the parent, applies to it.
	constrain := s.ref == "" && s.raw == nil
	kind := "number"
	switch {
	case asString || t.Kind() == reflect.String:
		kind = "string"
	case t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map:
		kind = "items"
	case t.Kind() == reflect.Struct || t.Kind() == reflect.Bool || t.Kind() == reflect.Interface:
		kind = ""
	}
	for _, token := range strings.Split(tag, ",") {
		key, value, _ := strings.Cut(strings.TrimSpace(token), "=")
		if key == "dive" {
			return required
		}
		if key == "required" {
			required = true
		}
		if !constrain {
			continue
		}
		switch key {
		case "email":
			s.format = "email"
		case "uuid", "uuid4", "uuid_rfc4122", "uuid4_rfc4122":
			s.format = "uuid"
		case "url", "uri", "http_url":
			s.format = "uri"
		case "oneof":
			for _, v := range strings.Fields(value) {
				if parsed, ok := parseScalar(v, t, asString); ok {
					s.enum = append(s.enum, parsed)
				}
			}
		case "min", "max", "len", "gte", "lte", "gt", "lt":
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			applyBound(s, kind, key, n)
		}
	}
	return required
}

func applyBound(s *schema, kind, key string, n float64) {
	i := int(n)
	switch kind {
	case "string":
		switch key {
		case "min", "gte":
			s.minLength = &i
		case "max", "lte":
			s.maxLength = &i
		case "len":
			s.minLength, s.maxLength = &i, &i
		}
	case "items":
		if s.typ != nil && s.typ[0] == "object" {
			return // maps: minProperties is out of scope
		}
		switch key {
		case "min", "gte":
			s.minItems = &i
		case "max", "lte":
			s.maxItems = &i
		case "len":
			s.minItems, s.maxItems = &i, &i
		}
	case "number":
		switch key {
		case "min", "gte":
			s.minimum = &n
		case "max", "lte":
			s.maximum = &n
		case "gt":
			s.exclusiveMinimum = &n
		case "lt":
			s.exclusiveMaximum = &n
		case "len":
			s.minimum, s.maximum = &n, &n
		}
	}
}

// parseExample turns an example tag into a value of the field's JSON type.
func parseExample(text string, t reflect.Type, asString bool) (any, bool) {
	if v, ok := parseScalar(text, base(t), asString); ok {
		return v, true
	}
	var v any
	if json.Unmarshal([]byte(text), &v) == nil {
		return v, true
	}
	return nil, false
}

func parseScalar(text string, t reflect.Type, asString bool) (any, bool) {
	if asString {
		return text, true
	}
	switch t.Kind() {
	case reflect.String:
		return text, true
	case reflect.Bool:
		b, err := strconv.ParseBool(text)
		return b, err == nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(text, 10, 64)
		return n, err == nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := strconv.ParseUint(text, 10, 64)
		return n, err == nil
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(text, 64)
		return n, err == nil
	}
	return nil, false
}

// nullable allows null alongside s: in the type list, or with anyOf for a
// $ref or a schema without a type.
func nullable(s *schema) *schema {
	switch {
	case s.ref != "" || s.raw != nil || len(s.anyOf) > 0:
		return &schema{anyOf: []*schema{s, {typ: []string{"null"}}}}
	case len(s.typ) == 0:
		return s // {} already allows null
	}
	if !slices.Contains(s.typ, "null") {
		s.typ = append(s.typ, "null")
	}
	return s
}

func ptr[T any](v T) *T { return &v }

// MarshalJSON writes the schema with a fixed key order.
func (s *schema) MarshalJSON() ([]byte, error) {
	if s.raw != nil {
		return json.Marshal(s.raw)
	}
	var b bytes.Buffer
	b.WriteByte('{')
	first := true
	field := func(key string, v any) {
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(strconv.Quote(key))
		b.WriteByte(':')
		enc, _ := json.Marshal(v)
		b.Write(enc)
	}
	if s.ref != "" {
		field("$ref", s.ref)
	}
	switch len(s.typ) {
	case 0:
	case 1:
		field("type", s.typ[0])
	default:
		field("type", s.typ)
	}
	if s.format != "" {
		field("format", s.format)
	}
	if s.contentEncoding != "" {
		field("contentEncoding", s.contentEncoding)
	}
	if s.contentMediaType != "" {
		field("contentMediaType", s.contentMediaType)
	}
	if s.description != "" {
		field("description", s.description)
	}
	if len(s.anyOf) > 0 {
		field("anyOf", s.anyOf)
	}
	if len(s.enum) > 0 {
		field("enum", s.enum)
	}
	if s.minimum != nil {
		field("minimum", *s.minimum)
	}
	if s.exclusiveMinimum != nil {
		field("exclusiveMinimum", *s.exclusiveMinimum)
	}
	if s.maximum != nil {
		field("maximum", *s.maximum)
	}
	if s.exclusiveMaximum != nil {
		field("exclusiveMaximum", *s.exclusiveMaximum)
	}
	if s.minLength != nil {
		field("minLength", *s.minLength)
	}
	if s.maxLength != nil {
		field("maxLength", *s.maxLength)
	}
	if s.items != nil {
		field("items", s.items)
	}
	if s.minItems != nil {
		field("minItems", *s.minItems)
	}
	if s.maxItems != nil {
		field("maxItems", *s.maxItems)
	}
	if s.properties != nil {
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(`"properties":{`)
		for i, p := range s.properties {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(p.name))
			b.WriteByte(':')
			enc, err := p.schema.MarshalJSON()
			if err != nil {
				return nil, err
			}
			b.Write(enc)
		}
		b.WriteByte('}')
	}
	if len(s.required) > 0 {
		field("required", s.required)
	}
	if s.additionalProperties != nil {
		field("additionalProperties", s.additionalProperties)
	}
	if len(s.examples) > 0 {
		field("examples", s.examples)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
