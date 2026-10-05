// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding"
	"fmt"
	"mime/multipart"
	"reflect"
	"strconv"
)

// A fieldSetter is compiled once per field type: it converts the strings a
// source gives into the field's value, whether a scalar, a TextUnmarshaler,
// a pointer, a slice or an uploaded file.

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

// Cache the exact FileHeader types because other structs and pointers are not
// valid multipart targets even when they have a similar shape.
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

var multipartFileHeaderType = reflect.TypeOf(multipart.FileHeader{})

var multipartFileHeaderPtrType = reflect.TypeOf((*multipart.FileHeader)(nil))

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

func (s fieldSetter) usesAllValues() bool {
	if s.kind == fieldSetterPointer {
		return s.elem.usesAllValues()
	}
	return s.kind == fieldSetterSliceString || s.kind == fieldSetterSliceInt
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
