// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	// err is the first field binding can't fill. A Typed handler panics with
	// it at registration; a binder returns it.
	err error
	// paramOnly are the fields tagged for the path, query, headers or
	// cookies and for no body format. A body decode can still write them,
	// as encoding/json matches keys to field names, so each decode resets
	// them: the body never fills a field the struct says comes from
	// elsewhere.
	paramOnly []bindingField
}

// bodyTags are the struct tags that opt a field into the body. Zinc can't ask
// a configured decoder which tag it reads, so the common ones are listed.
var bodyTags = [...]string{"json", "xml", "form", "yaml", "toml", "msgpack", "cbor", "bson"}

// paramOnly reports whether field is tagged for the path, query, headers or
// cookies and for no body format.
func paramOnly(field reflect.StructField) bool {
	param := false
	for _, tag := range [...]string{"path", "query", "header", "cookie"} {
		if v, ok := field.Tag.Lookup(tag); ok && v != "-" {
			param = true
			break
		}
	}
	if !param {
		return false
	}
	for _, tag := range bodyTags {
		if v, ok := field.Tag.Lookup(tag); ok && v != "-" {
			return false
		}
	}
	return true
}

// keepParamsOutOfBody undoes what a body decode wrote to the fields the plan
// lists as parameter-only, restoring their default or zero value.
func (plan *bindingPlan) keepParamsOutOfBody(val reflect.Value) {
	for _, f := range plan.paramOnly {
		fv := f.value(val)
		fv.SetZero()
		if f.def != nil {
			_ = f.setter.set(fv, f.def)
		}
	}
}

// paramSnapshot holds a binder target's parameter-only fields across a body
// decode, so a single-source body binder, such as Bind().JSON, puts back the
// values earlier binders set instead of letting the body replace them.
// Scalars are held without allocating; other types are copied.
type paramSnapshot struct {
	plan  *bindingPlan
	val   reflect.Value
	saved [8]savedParam
	extra []savedParam
}

type savedParam struct {
	s    string
	i    int64
	u    uint64
	f    float64
	b    bool
	copy reflect.Value // for a type that isn't a plain scalar
}

// snapshotParams records v's parameter-only fields before a body decode. It
// returns false when v has none, so there's nothing to restore.
func snapshotParams(v any) (paramSnapshot, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return paramSnapshot{}, false
	}
	plan := bindingPlanFor(rv.Elem().Type())
	if len(plan.paramOnly) == 0 {
		return paramSnapshot{}, false
	}
	snap := paramSnapshot{plan: plan, val: rv.Elem()}
	for i, f := range plan.paramOnly {
		fv := promotedOrField(rv.Elem(), f)
		var sp savedParam
		if fv.IsValid() {
			switch fv.Kind() {
			case reflect.String:
				sp.s = fv.String()
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				sp.i = fv.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				sp.u = fv.Uint()
			case reflect.Float32, reflect.Float64:
				sp.f = fv.Float()
			case reflect.Bool:
				sp.b = fv.Bool()
			default:
				if !fv.IsZero() {
					sp.copy = detachedCopy(fv)
				}
			}
		}
		if i < len(snap.saved) {
			snap.saved[i] = sp
		} else {
			snap.extra = append(snap.extra, sp)
		}
	}
	return snap, true
}

// detachedCopy copies v so a decode into v can't change the copy: a slice
// gets its own backing array and a pointer its own value, since
// encoding/json writes into both in place.
func detachedCopy(v reflect.Value) reflect.Value {
	c := reflect.New(v.Type()).Elem()
	switch v.Kind() {
	case reflect.Slice:
		c.Set(reflect.MakeSlice(v.Type(), v.Len(), v.Len()))
		reflect.Copy(c, v)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		p.Elem().Set(detachedCopy(v.Elem()))
		c.Set(p)
	default:
		c.Set(v)
	}
	return c
}

// restore puts back the recorded values, undoing what the body decode wrote.
func (s *paramSnapshot) restore() {
	for i, f := range s.plan.paramOnly {
		sp := &s.extra
		var saved savedParam
		if i < len(s.saved) {
			saved = s.saved[i]
		} else {
			saved = (*sp)[i-len(s.saved)]
		}
		fv := promotedOrField(s.val, f)
		if !fv.IsValid() {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(saved.s)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(saved.i)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			fv.SetUint(saved.u)
		case reflect.Float32, reflect.Float64:
			fv.SetFloat(saved.f)
		case reflect.Bool:
			fv.SetBool(saved.b)
		default:
			if saved.copy.IsValid() {
				fv.Set(saved.copy)
			} else {
				fv.SetZero()
			}
		}
	}
}

// promotedOrField returns the field without allocating a nil embedded
// pointer on the way: an invalid Value when one is nil.
func promotedOrField(v reflect.Value, f bindingField) reflect.Value {
	if f.path == nil {
		return v.Field(f.index)
	}
	for _, i := range f.path[:len(f.path)-1] {
		v = v.Field(i)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
		}
	}
	return v.Field(f.path[len(f.path)-1])
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

// fieldSetter precompiles the reflection decisions needed to convert input. It
// deliberately contains data only, so cached plans remain safe for concurrent use.
type fieldSetter struct {
	kind             fieldSetterKind
	elem             *fieldSetter
	bits             int
	elemBits         int
	unsupportedKind  reflect.Kind
	unsupportedSlice reflect.Type
}

// fieldSetterKind selects a conversion path without repeating reflect.Kind
// switches for every request.
type fieldSetterKind uint8

const (
	fieldSetterString fieldSetterKind = iota
	fieldSetterText
	fieldSetterPointer
	fieldSetterBool
	fieldSetterInt
	fieldSetterUint
	fieldSetterFloat
	fieldSetterSliceString
	fieldSetterSliceInt
	fieldSetterFileHeaderValue
	fieldSetterFileHeaderPtr
	fieldSetterSliceFileHeaderValue
	fieldSetterSliceFileHeaderPtr
	fieldSetterUnsupportedKind
	fieldSetterUnsupportedSlice
)

// Binding plans are immutable and safe to share across requests. Compiling
// reflection and conversion decisions once keeps the hot path predictable.
var bindingPlanCache sync.Map

// Cache the exact FileHeader types because other structs and pointers are not
// valid multipart targets even when they have a similar shape.
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

var multipartFileHeaderType = reflect.TypeOf(multipart.FileHeader{})
var multipartFileHeaderPtrType = reflect.TypeOf((*multipart.FileHeader)(nil))

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
	plan := &bindingPlan{}
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
			plan.err = fmt.Errorf("zinc: %s.%s has a binding tag, but binding can't fill a %s; use a string, number or bool, a type with UnmarshalText, a pointer to one, or a slice of them", root, field.Name, field.Type)
		}
		def := compileDefault(root, field, setter)
		if def != nil {
			plan.hasDefaults = true
		}
		if paramOnly(field) {
			plan.paramOnly = append(plan.paramOnly, bindingField{index: i, path: path, setter: setter, def: def})
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

// unsupported reports whether binding can't fill the field at all.
func (s fieldSetter) unsupported() bool {
	switch s.kind {
	case fieldSetterUnsupportedKind, fieldSetterUnsupportedSlice:
		return true
	case fieldSetterPointer:
		return s.elem.unsupported()
	}
	return false
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

func compileFieldSetter(typ reflect.Type) fieldSetter { return compileFieldSetterDepth(typ, 0) }

func compileFieldSetterDepth(typ reflect.Type, depth int) fieldSetter {
	if depth >= 16 {
		return fieldSetter{kind: fieldSetterUnsupportedKind, unsupportedKind: typ.Kind()}
	}
	if typ.Kind() != reflect.Interface && (typ.Implements(textUnmarshalerType) || (typ.Kind() != reflect.Pointer && reflect.PointerTo(typ).Implements(textUnmarshalerType))) {
		return fieldSetter{kind: fieldSetterText}
	}

	switch typ.Kind() {
	case reflect.String:
		return fieldSetter{kind: fieldSetterString}
	case reflect.Bool:
		return fieldSetter{kind: fieldSetterBool}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fieldSetter{kind: fieldSetterInt, bits: int(typ.Bits())}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fieldSetter{kind: fieldSetterUint, bits: int(typ.Bits())}
	case reflect.Float32, reflect.Float64:
		return fieldSetter{kind: fieldSetterFloat, bits: int(typ.Bits())}
	case reflect.Struct:
		if typ == multipartFileHeaderType {
			return fieldSetter{kind: fieldSetterFileHeaderValue}
		}
	case reflect.Pointer:
		if typ == multipartFileHeaderPtrType {
			return fieldSetter{kind: fieldSetterFileHeaderPtr}
		}
		elem := compileFieldSetterDepth(typ.Elem(), depth+1)
		return fieldSetter{kind: fieldSetterPointer, elem: &elem}
	case reflect.Slice:
		if typ.Elem() == multipartFileHeaderType {
			return fieldSetter{kind: fieldSetterSliceFileHeaderValue}
		}
		if typ.Elem() == multipartFileHeaderPtrType {
			return fieldSetter{kind: fieldSetterSliceFileHeaderPtr}
		}
		switch typ.Elem().Kind() {
		case reflect.String:
			return fieldSetter{kind: fieldSetterSliceString}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return fieldSetter{kind: fieldSetterSliceInt, elemBits: int(typ.Elem().Bits())}
		default:
			return fieldSetter{kind: fieldSetterUnsupportedSlice, unsupportedSlice: typ.Elem()}
		}
	default:
		return fieldSetter{kind: fieldSetterUnsupportedKind, unsupportedKind: typ.Kind()}
	}
	// Preserve unsupported types in the plan instead of failing compilation.
	// They should error only when request input actually targets that field.
	return fieldSetter{kind: fieldSetterUnsupportedKind, unsupportedKind: typ.Kind()}
}

// bindError records the failing field under both its Go name, for logs, and
// its request name with a client-safe reason, for the error response.
func (f bindingField) bindError(source string, err error) *bindFieldError {
	return &bindFieldError{Source: source, Field: f.label, Name: f.name, Reason: f.setter.reason(), Err: err}
}

// reason describes, for clients, the value a setter accepts.
func (s fieldSetter) reason() string {
	switch s.kind {
	case fieldSetterBool:
		return "must be a boolean"
	case fieldSetterInt, fieldSetterSliceInt:
		return "must be an integer"
	case fieldSetterUint:
		return "must be a non-negative integer"
	case fieldSetterFloat:
		return "must be a number"
	case fieldSetterPointer:
		if s.elem != nil {
			return s.elem.reason()
		}
	}
	return ""
}

func (s fieldSetter) supportsFiles() bool {
	switch s.kind {
	case fieldSetterFileHeaderValue, fieldSetterFileHeaderPtr, fieldSetterSliceFileHeaderValue, fieldSetterSliceFileHeaderPtr:
		return true
	default:
		return false
	}
}

func bindFieldsFromValues(val reflect.Value, fields []bindingField, values url.Values) error {
	if len(fields) == 0 || len(values) == 0 {
		return nil
	}
	// Scalar setters consume the first value; slice setters retain every value
	// in transport order.
	for _, field := range fields {
		inputs, ok := values[field.name]
		if !ok || len(inputs) == 0 {
			continue
		}
		if err := field.setter.set(field.value(val), inputs); err != nil {
			return field.bindError("", err)
		}
	}
	return nil
}

// Short query strings can be bound directly without allocating a map for
// parameters that the target does not use. Larger inputs keep net/url's
// parser so binding cost does not grow with the product of fields and pairs.
func bindFieldsFromQuery(val reflect.Value, fields []bindingField, c *Context) error {
	if len(fields) == 0 || c == nil || c.request == nil || c.request.URL == nil {
		return nil
	}
	if c.queryParams != nil {
		return bindFieldsFromValues(val, fields, c.queryParams)
	}
	raw := c.request.URL.RawQuery
	if raw == "" {
		return nil
	}
	if len(raw) > 128 || len(fields) > 8 {
		return bindFieldsFromValues(val, fields, c.QueryValues())
	}
	var first [8]string
	var found [8]bool
	var repeated [8][]string
	for raw != "" {
		pair, remaining, _ := strings.Cut(raw, "&")
		raw = remaining
		if pair == "" || strings.Contains(pair, ";") {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		if strings.ContainsAny(key, "%+") {
			decoded, err := url.QueryUnescape(key)
			if err != nil {
				continue
			}
			key = decoded
		}
		if strings.ContainsAny(value, "%+") {
			decoded, err := url.QueryUnescape(value)
			if err != nil {
				continue
			}
			value = decoded
		}
		for i, field := range fields {
			if field.name != key {
				continue
			}
			if !found[i] {
				first[i], found[i] = value, true
			} else if field.setter.usesAllValues() {
				if repeated[i] == nil {
					repeated[i] = []string{first[i]}
				}
				repeated[i] = append(repeated[i], value)
			}
		}
	}
	for i, field := range fields {
		if !found[i] {
			continue
		}
		inputs := repeated[i]
		if inputs == nil {
			single := [1]string{first[i]}
			inputs = single[:]
		}
		if err := field.setter.set(field.value(val), inputs); err != nil {
			return field.bindError("", err)
		}
	}
	return nil
}

func (s fieldSetter) usesAllValues() bool {
	if s.kind == fieldSetterPointer {
		return s.elem.usesAllValues()
	}
	return s.kind == fieldSetterSliceString || s.kind == fieldSetterSliceInt
}

func bindFieldsFromHeader(val reflect.Value, fields []bindingField, header http.Header) error {
	if len(fields) == 0 || len(header) == 0 {
		return nil
	}
	// Names were canonicalized while compiling the immutable binding plan.
	// Direct lookup preserves repeated header lines without per-request work.
	for _, field := range fields {
		inputs := header[field.headerName]
		if len(inputs) == 0 {
			continue
		}
		if err := field.setter.set(field.value(val), inputs); err != nil {
			return field.bindError("", err)
		}
	}
	return nil
}

func bindFieldsFromCookies(val reflect.Value, fields []bindingField, req *http.Request) error {
	if len(fields) == 0 || req == nil || len(req.Header["Cookie"]) == 0 {
		return nil
	}
	for _, field := range fields {
		cookie, err := req.Cookie(field.name)
		if err != nil {
			continue
		}
		single := [1]string{cookie.Value}
		if err := field.setter.set(field.value(val), single[:]); err != nil {
			return field.bindError("", err)
		}
	}
	return nil
}

func bindFieldsFromMultipartFiles(val reflect.Value, fields []bindingField, files map[string][]*multipart.FileHeader) error {
	if len(fields) == 0 || len(files) == 0 {
		return nil
	}
	// File fields are kept separate from textual form fields so a filename can
	// never be coerced through a string setter by accident.
	for _, field := range fields {
		inputs := files[field.name]
		if len(inputs) == 0 {
			continue
		}
		if err := field.setter.setFiles(field.value(val), inputs); err != nil {
			return field.bindError("form", err)
		}
	}
	return nil
}

func bindFieldsFromPath(val reflect.Value, fields []bindingField, c *Context) error {
	if len(fields) == 0 || c == nil || c.paramCount == 0 {
		return nil
	}
	// Resolve parameters through Context so lazy router offsets remain an
	// internal optimization rather than leaking into binding.
	for _, field := range fields {
		input, ok := c.lookupPathParam(field.name)
		if !ok {
			continue
		}
		single := [1]string{input}
		if err := field.setter.set(field.value(val), single[:]); err != nil {
			return field.bindError("path", err)
		}
	}
	return nil
}

func (s fieldSetter) set(value reflect.Value, inputs []string) error {
	if !value.CanSet() || len(inputs) == 0 {
		return nil
	}

	// Conversion is strict: strconv bit sizes match the destination exactly and
	// overflow is returned instead of truncating data.
	switch s.kind {
	case fieldSetterText:
		target := value
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				target = reflect.New(value.Type().Elem())
				if err := target.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(inputs[0])); err != nil {
					return err
				}
				value.Set(target)
				return nil
			}
		} else {
			target = value.Addr()
		}
		return target.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(inputs[0]))
	case fieldSetterPointer:
		if value.IsNil() {
			target := reflect.New(value.Type().Elem())
			if err := s.elem.set(target.Elem(), inputs); err != nil {
				return err
			}
			value.Set(target)
			return nil
		}
		return s.elem.set(value.Elem(), inputs)

	case fieldSetterString:
		value.SetString(inputs[0])
	case fieldSetterBool:
		parsed, err := strconv.ParseBool(inputs[0])
		if err != nil {
			return err
		}
		value.SetBool(parsed)
	case fieldSetterInt:
		parsed, err := strconv.ParseInt(inputs[0], 10, s.bits)
		if err != nil {
			return err
		}
		value.SetInt(parsed)
	case fieldSetterUint:
		parsed, err := strconv.ParseUint(inputs[0], 10, s.bits)
		if err != nil {
			return err
		}
		value.SetUint(parsed)
	case fieldSetterFloat:
		parsed, err := strconv.ParseFloat(inputs[0], s.bits)
		if err != nil {
			return err
		}
		value.SetFloat(parsed)
	case fieldSetterSliceString:
		slice := reflect.MakeSlice(value.Type(), len(inputs), len(inputs))
		for i, input := range inputs {
			slice.Index(i).SetString(input)
		}
		value.Set(slice)
	case fieldSetterSliceInt:
		slice := reflect.MakeSlice(value.Type(), len(inputs), len(inputs))
		for i, input := range inputs {
			parsed, err := strconv.ParseInt(input, 10, s.elemBits)
			if err != nil {
				return err
			}
			slice.Index(i).SetInt(parsed)
		}
		value.Set(slice)
	case fieldSetterFileHeaderValue, fieldSetterFileHeaderPtr, fieldSetterSliceFileHeaderValue, fieldSetterSliceFileHeaderPtr:
		return fmt.Errorf("multipart files must be bound from multipart file data")
	case fieldSetterUnsupportedSlice:
		return fmt.Errorf("unsupported slice element type %s", s.unsupportedSlice)
	case fieldSetterUnsupportedKind:
		return fmt.Errorf("unsupported kind %s", s.unsupportedKind)
	}
	return nil
}

func (s fieldSetter) setFiles(value reflect.Value, files []*multipart.FileHeader) error {
	if !value.CanSet() || len(files) == 0 {
		return nil
	}

	// Pointer targets reference FileHeaders owned by the parsed multipart form;
	// value targets receive copies of those headers.
	switch s.kind {
	case fieldSetterFileHeaderValue:
		value.Set(reflect.ValueOf(*files[0]).Convert(value.Type()))
	case fieldSetterFileHeaderPtr:
		value.Set(reflect.ValueOf(files[0]))
	case fieldSetterSliceFileHeaderValue:
		slice := reflect.MakeSlice(value.Type(), len(files), len(files))
		for i, file := range files {
			slice.Index(i).Set(reflect.ValueOf(*file).Convert(value.Type().Elem()))
		}
		value.Set(slice)
	case fieldSetterSliceFileHeaderPtr:
		slice := reflect.MakeSlice(value.Type(), len(files), len(files))
		for i, file := range files {
			slice.Index(i).Set(reflect.ValueOf(file))
		}
		value.Set(slice)
	default:
		return fmt.Errorf("unsupported multipart file target")
	}
	return nil
}

func (c *Context) lookupPathParam(name string) (string, bool) {
	// Registered radix routes carry a name-to-index table. Fall back to a linear
	// scan only for manually populated or otherwise non-indexed parameters.
	if route := c.paramRoute; route != nil {
		if c.paramPath != "" && c.paramCount > 1 && len(route.paramIndices) > 0 {
			c.materializePathParams()
		}
		if index, ok := route.paramIndex(name); ok {
			if index >= c.paramCount {
				return "", false
			}
			if c.pathParams[index].start == directParamStart {
				return c.pathParams[index].value, true
			}
			return c.pathParamValueAt(index), true
		}
		return "", false
	}
	if c.paramPath != "" && c.paramCount > 1 {
		c.materializePathParams()
	}
	for i := 0; i < c.paramCount; i++ {
		if c.pathParams[i].key != name {
			continue
		}
		if c.pathParams[i].start == directParamStart {
			return c.pathParams[i].value, true
		}
		return c.pathParamValueAt(i), true
	}
	return "", false
}

func requestMediaType(header string) string {
	// Binding dispatch needs only the media type. Charset and boundary parameters
	// remain available on the request for the format-specific parser.
	base, _, _ := strings.Cut(header, ";")
	return strings.TrimSpace(base)
}
