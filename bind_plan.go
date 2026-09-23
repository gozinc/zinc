// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
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
}

// bindingField keeps both the wire name and Go field label: the former locates
// input while the latter makes conversion errors actionable.
type bindingField struct {
	index  int
	name   string
	label  string
	setter fieldSetter
}

// bindFieldError carries source and field attribution through the binder without
// discarding the strconv or reflection error that caused the failure.
type bindFieldError struct {
	Source string
	Field  string
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

	return val, bindingPlanFor(val.Type()), nil
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

func compileBindingPlan(typ reflect.Type) *bindingPlan {
	plan := &bindingPlan{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		// Unexported fields cannot be set through reflection and must never be
		// made writable with unsafe solely for binding convenience.
		if field.PkgPath != "" {
			continue
		}

		// One field may participate in several sources. The plan preserves that
		// intentionally so Bind.All can apply its documented source precedence.
		setter := compileFieldSetter(field.Type)
		if compiled, ok := compileBindingField(i, field, setter, "path"); ok {
			plan.pathFields = append(plan.pathFields, compiled)
		}
		if compiled, ok := compileBindingField(i, field, setter, "query"); ok {
			plan.queryFields = append(plan.queryFields, compiled)
		}
		if compiled, ok := compileBindingField(i, field, setter, "form"); ok {
			if setter.supportsFiles() {
				plan.multipartFileFields = append(plan.multipartFileFields, compiled)
			} else {
				plan.formFields = append(plan.formFields, compiled)
			}
		}
		if compiled, ok := compileBindingField(i, field, setter, "header"); ok {
			plan.headerFields = append(plan.headerFields, compiled)
		}
	}
	return plan
}

func compileBindingField(index int, field reflect.StructField, setter fieldSetter, tag string) (bindingField, bool) {
	name, ok := bindingFieldName(field, tag)
	if !ok {
		return bindingField{}, false
	}
	return bindingField{
		index:  index,
		name:   name,
		label:  field.Name,
		setter: setter,
	}, true
}

func bindingFieldName(field reflect.StructField, tag string) (string, bool) {
	name := field.Tag.Get(tag)
	if name == "-" {
		return "", false
	}
	// Ignore comma options for compatibility with conventional Go struct tags;
	// Zinc currently needs only the name portion.
	if idx := strings.IndexByte(name, ','); idx >= 0 {
		name = name[:idx]
	}
	// Untagged exported fields bind by their lower-cased Go name. A per-source
	// "-" tag is the explicit opt-out.
	if name == "" {
		name = strings.ToLower(field.Name)
	}
	if tag == "header" {
		name = strings.ToLower(name)
	}
	return name, true
}

func compileFieldSetter(typ reflect.Type) fieldSetter {
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
		if err := field.setter.set(val.Field(field.index), inputs); err != nil {
			return &bindFieldError{Field: field.label, Err: err}
		}
	}
	return nil
}

func bindFieldsFromHeader(val reflect.Value, fields []bindingField, header http.Header) error {
	if len(fields) == 0 || len(header) == 0 {
		return nil
	}
	// Header.Values preserves repeated header lines and canonicalizes lookup via
	// net/http rather than duplicating MIME header rules here.
	for _, field := range fields {
		inputs := header.Values(field.name)
		if len(inputs) == 0 {
			continue
		}
		if err := field.setter.set(val.Field(field.index), inputs); err != nil {
			return &bindFieldError{Field: field.label, Err: err}
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
		if err := field.setter.setFiles(val.Field(field.index), inputs); err != nil {
			return &bindFieldError{Source: "form", Field: field.label, Err: err}
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
		if err := field.setter.set(val.Field(field.index), single[:]); err != nil {
			return &bindFieldError{Source: "path", Field: field.label, Err: err}
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
			if c.PathParams[index].start == directParamStart {
				return c.PathParams[index].value, true
			}
			return c.pathParamValueAt(index), true
		}
		return "", false
	}
	if c.paramPath != "" && c.paramCount > 1 {
		c.materializePathParams()
	}
	for i := 0; i < c.paramCount; i++ {
		if c.PathParams[i].key != name {
			continue
		}
		if c.PathParams[i].start == directParamStart {
			return c.PathParams[i].value, true
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
