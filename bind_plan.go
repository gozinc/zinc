// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// bindingPlan is the immutable, type-specific description used by every bind
// operation after the first request for a target type.
type bindingPlan struct {
	pathFields          []bindingField
	queryFields         []bindingField
	formFields          []bindingField
	multipartFileFields []bindingField
	headerFields        []bindingField
	cookieFields        []bindingField
	// hasDefaults reports whether any field has a default tag, so binding
	// skips the defaults pass for types without one.
	hasDefaults bool
	// defaultFields are every field with a default tag, whatever its
	// source: the body, the path or anywhere else. Binding every source
	// sets them first, so a source that leaves a field out keeps its
	// default.
	defaultFields []bindingField
	// rules checks the type's validate tags and enum values; nil when it
	// has none.
	rules *rulePlan
	// err is the first field binding can't fill. A Typed handler panics with
	// it at registration; a binder returns it.
	err error
	// paramOnly are the fields tagged for the path, query, headers or
	// cookies and for no body format. A body decode can still write them,
	// as encoding/json matches keys to field names, so each decode resets
	// them: the body never fills a field the struct says comes from
	// elsewhere.
	paramOnly []bindingField
	// paramNames are the paramOnly fields' Go names in lower case: the names
	// a decoder matches body keys against. A body that mentions none of
	// them can't have filled those fields, so nothing needs resetting.
	// paramNamesFold is set when a name isn't ASCII, whose case folding the
	// scan doesn't attempt; such a plan always resets.
	paramNames     []string
	paramNamesFold bool
	// paramFirst marks the bytes, either case, that start a param name, so
	// the scan is one pass over the body.
	paramFirst [256]bool
}

// bindingField keeps both the wire name and Go field label: the former locates
// input while the latter makes conversion errors actionable.
type bindingField struct {
	// index is the field's position in the struct; path is its index path
	// when it's promoted from an embedded struct, and nil otherwise.
	index      int
	path       []int
	name       string
	headerName string
	label      string
	setter     fieldSetter
	// def is the parsed default tag: the inputs bound when the request leaves
	// the field out. It's nil without a default.
	def []string
	// media lists the content types a file field accepts, from its media
	// tag; nil accepts any.
	media []string
}

// bindFieldError carries source and field attribution through the binder without
// discarding the strconv or reflection error that caused the failure.
type bindFieldError struct {
	Source string
	Field  string
	Name   string
	Reason string
	Err    error
}

func (e *bindFieldError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Source != "" && e.Field != "":
		return fmt.Sprintf("bind %s %s: %v", e.Source, e.Field, e.Err)
	case e.Source != "":
		return fmt.Sprintf("bind %s: %v", e.Source, e.Err)
	case e.Field != "":
		return fmt.Sprintf("bind %s: %v", e.Field, e.Err)
	default:
		return e.Err.Error()
	}
}

func (e *bindFieldError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Binding plans are immutable and safe to share across requests. Compiling
// reflection and conversion decisions once keeps the hot path predictable.
var bindingPlanCache sync.Map

// bindTargetPlan validates the public binding contract before any field is
// touched: the target must be a non-nil pointer to a struct.
func bindTargetPlan(ptr any) (reflect.Value, *bindingPlan, error) {
	if ptr == nil {
		return reflect.Value{}, nil, fmt.Errorf("binding target must not be nil")
	}

	val := reflect.ValueOf(ptr)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return reflect.Value{}, nil, fmt.Errorf("binding target must be a pointer")
	}

	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return reflect.Value{}, nil, fmt.Errorf("binding target must point to a struct")
	}

	plan := bindingPlanFor(val.Type())
	if plan.err != nil {
		return reflect.Value{}, nil, plan.err
	}
	return val, plan, nil
}

func bindingPlanFor(typ reflect.Type) *bindingPlan {
	if cached, ok := bindingPlanCache.Load(typ); ok {
		return cached.(*bindingPlan)
	}

	// Two first requests may compile the same type concurrently. LoadOrStore
	// accepts that small one-time duplication and avoids a global compilation lock.
	plan := compileBindingPlan(typ)
	actual, _ := bindingPlanCache.LoadOrStore(typ, plan)
	return actual.(*bindingPlan)
}

// compileBindingPlan panics on a default tag the field can't hold, so the
// mistake shows at registration for a Typed handler.
func compileBindingPlan(typ reflect.Type) *bindingPlan {
	plan := &bindingPlan{rules: rulePlanFor(typ)}
	plan.compileFields(typ, typ, nil, 0)
	return plan
}

// bindingTags are the tags that bind a field from outside the body.
var bindingTags = [...]string{"path", "query", "form", "header", "cookie"}

// hasBindingTag reports whether a binding tag opts the field in.
func hasBindingTag(field reflect.StructField) bool {
	for _, tag := range bindingTags {
		if v, ok := field.Tag.Lookup(tag); ok && v != "-" {
			return true
		}
	}
	return false
}

// compileFields adds t's tagged fields to the plan. The fields of an untagged
// embedded struct are promoted, as Go promotes them; path is t's index path
// within root.
func (plan *bindingPlan) compileFields(root, t reflect.Type, path []int, depth int) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous && !hasBindingTag(field) {
			et, isPtr := field.Type, field.Type.Kind() == reflect.Pointer
			if isPtr {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct && depth < 8 {
				if isPtr && field.PkgPath != "" {
					// Binding can't allocate an unexported embedded pointer.
					if plan.err == nil && hasTaggedFields(et) {
						plan.err = fmt.Errorf("zinc: %s embeds *%s, whose tagged fields binding can't fill: the pointer is unexported; embed the struct itself, or export it", root, et.Name())
					}
					continue
				}
				plan.compileFields(root, et, append(slices.Clone(path), i), depth+1)
				continue
			}
		}
		// Unexported fields cannot be set through reflection and must never be
		// made writable with unsafe solely for binding convenience.
		if field.PkgPath != "" {
			continue
		}
		var fieldPath []int
		if path != nil {
			fieldPath = append(slices.Clone(path), i)
		}
		plan.compileField(root, i, fieldPath, field)
	}
}

// bindAdvice says which types binding can fill, in terms of t.
func bindAdvice(t reflect.Type) string {
	if t.Kind() == reflect.Slice {
		return "a slice's elements must be a string, number or bool, or a type with UnmarshalText, not a pointer, slice, map or struct"
	}
	return "use a string, number or bool, a type with UnmarshalText, a pointer to one, or a slice of strings, numbers, bools or UnmarshalText types"
}

// hasTaggedFields reports whether t, or a struct it embeds, has a field with
// a binding tag.
func hasTaggedFields(t reflect.Type) bool {
	for i := range t.NumField() {
		f := t.Field(i)
		if hasBindingTag(f) {
			return true
		}
		if et := base(f.Type); f.Anonymous && et.Kind() == reflect.Struct && et != t && hasTaggedFields(et) {
			return true
		}
	}
	return false
}

func (plan *bindingPlan) compileField(root reflect.Type, i int, path []int, field reflect.StructField) {
	{
		// One field may participate in several sources. The plan preserves that
		// intentionally so Bind.All can apply its documented source precedence.
		setter := compileFieldSetter(field.Type)
		if plan.err == nil && setter.unsupported() && hasBindingTag(field) {
			plan.err = fmt.Errorf("zinc: %s.%s has a binding tag, but binding can't fill a %s; %s", root, field.Name, field.Type, bindAdvice(field.Type))
		}
		def := compileDefault(root, field, setter)
		if def != nil {
			plan.hasDefaults = true
			plan.defaultFields = append(plan.defaultFields, bindingField{index: i, path: path, setter: setter, def: def})
		}
		if paramOnly(field) {
			plan.paramOnly = append(plan.paramOnly, bindingField{index: i, path: path, setter: setter, def: def})
			name := strings.ToLower(field.Name)
			plan.paramNames = append(plan.paramNames, name)
			if name != "" && name[0] < utf8.RuneSelf {
				plan.paramFirst[name[0]] = true
				if 'a' <= name[0] && name[0] <= 'z' {
					plan.paramFirst[name[0]-'a'+'A'] = true
				}
			}
			for i := 0; i < len(name); i++ {
				if name[i] >= utf8.RuneSelf {
					plan.paramNamesFold = true
				}
			}
		}
		add := func(list *[]bindingField, compiled bindingField) {
			compiled.path = path
			*list = append(*list, compiled)
		}
		if compiled, ok := compileBindingField(i, field, setter, "path"); ok {
			add(&plan.pathFields, compiled)
		}
		if compiled, ok := compileBindingField(i, field, setter, "query"); ok {
			compiled.def = def
			add(&plan.queryFields, compiled)
		}
		if compiled, ok := compileBindingField(i, field, setter, "form"); ok {
			if setter.supportsFiles() {
				compiled.media = mediaTag(field)
				add(&plan.multipartFileFields, compiled)
			} else {
				compiled.def = def
				add(&plan.formFields, compiled)
			}
		}
		if compiled, ok := compileBindingField(i, field, setter, "header"); ok {
			compiled.def = def
			add(&plan.headerFields, compiled)
		}
		if compiled, ok := compileBindingField(i, field, setter, "cookie"); ok {
			compiled.def = def
			add(&plan.cookieFields, compiled)
		}
	}
}

// value returns the field within v, the bound struct. A nil embedded
// pointer on the way is allocated, as a value is about to be set.
func (f bindingField) value(v reflect.Value) reflect.Value {
	if f.path == nil {
		return v.Field(f.index)
	}
	return promotedField(v, f.path)
}

func promotedField(v reflect.Value, path []int) reflect.Value {
	for _, i := range path[:len(path)-1] {
		v = v.Field(i)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
	}
	return v.Field(path[len(path)-1])
}

// structField returns the field's declaration within t, the bound struct.
func (f bindingField) structField(t reflect.Type) reflect.StructField {
	if f.path == nil {
		return t.Field(f.index)
	}
	return t.FieldByIndex(f.path)
}

// compileDefault parses a field's default tag into the inputs a request
// would carry: one value, or comma-separated values for a slice.
func compileDefault(typ reflect.Type, field reflect.StructField, setter fieldSetter) []string {
	text, ok := field.Tag.Lookup("default")
	if !ok {
		return nil
	}
	def := []string{text}
	if setter.usesAllValues() {
		def = strings.Split(text, ",")
	}
	if err := setter.set(reflect.New(field.Type).Elem(), def); err != nil {
		panic(fmt.Sprintf("zinc: default tag %q on %s.%s: %v", text, typ.Name(), field.Name, err))
	}
	return def
}

// applyDefaults sets the fields that have a default, before a source binds
// over them.
func applyDefaults(val reflect.Value, fields []bindingField) {
	for _, field := range fields {
		if field.def != nil {
			// The value was checked when the plan compiled.
			_ = field.setter.set(field.value(val), field.def)
		}
	}
}

func compileBindingField(index int, field reflect.StructField, setter fieldSetter, tag string) (bindingField, bool) {
	name, ok := bindingFieldName(field, tag)
	if !ok {
		return bindingField{}, false
	}
	headerName := ""
	if tag == "header" {
		// Keep the source name for map binding, but precompute the canonical
		// key used by net/http request headers.
		headerName = http.CanonicalHeaderKey(name)
	}
	return bindingField{
		index:      index,
		name:       name,
		headerName: headerName,
		label:      field.Name,
		setter:     setter,
	}, true
}

func bindingFieldName(field reflect.StructField, tag string) (string, bool) {
	// Request sources bind only fields that opt in with their tag. Binding
	// untagged fields would let any query parameter or header set a field the
	// struct's author never exposed, such as one hidden from JSON with
	// `json:"-"`.
	name, ok := field.Tag.Lookup(tag)
	if !ok || name == "-" {
		return "", false
	}
	// Ignore comma options for compatibility with conventional Go struct tags;
	// Zinc currently needs only the name portion.
	if idx := strings.IndexByte(name, ','); idx >= 0 {
		name = name[:idx]
	}
	// A tag with options but no name, such as `query:",omitempty"`, opts in
	// under the lower-cased Go name.
	if name == "" {
		name = strings.ToLower(field.Name)
	}
	if tag == "header" {
		name = strings.ToLower(name)
	}
	return name, true
}

// bindError records the failing field under both its Go name, for logs, and
// its request name with a client-safe reason, for the error response.
func (f bindingField) bindError(source string, err error) *bindFieldError {
	return &bindFieldError{Source: source, Field: f.label, Name: f.name, Reason: f.setter.reason(), Err: err}
}
