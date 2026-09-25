// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
)

// Param returns the named route parameter parsed as T. A value that does not
// parse is a *BindError, which the default error handler answers with 400
// and the parameter's name:
//
//	id, err := zinc.Param[int64](c, "id")
//	if err != nil {
//		return err
//	}
//
// T may be a string, bool, integer, or float type, a type implementing
// encoding.TextUnmarshaler, or a pointer to one of those.
func Param[T any](c *Context, name string) (T, error) {
	// c.Param is the router's fast path. Only an empty result needs the
	// slower lookup to tell a missing parameter from an empty catch-all.
	// c.Param is the router's fast path. Only an empty result needs the
	// slower lookup to tell a missing parameter from an empty catch-all.
	value := c.Param(name)
	present := value != ""
	if !present {
		_, present = c.lookupPathParam(name)
	}
	return parseRequestValue[T]("path", name, value, present)
}

// Query returns the first query value for name parsed as T. A missing or
// unparsable value is a *BindError, answered with 400. Use QueryOr for an
// optional value.
func Query[T any](c *Context, name string) (T, error) {
	value, ok := c.firstQueryValue(name)
	return parseRequestValue[T]("query", name, value, ok)
}

// QueryOr returns the first query value for name parsed as T, or fallback when
// the value is missing or does not parse. T is inferred from fallback:
//
//	page := zinc.QueryOr(c, "page", 1)
func QueryOr[T any](c *Context, name string, fallback T) T {
	value, ok := c.firstQueryValue(name)
	if !ok {
		return fallback
	}
	if parsed, err := parseValue[T](value); err == nil {
		return parsed
	}
	return fallback
}

// Form returns the named form value parsed as T, with the semantics of
// FormValue. A missing or unparsable value is a *BindError, answered with 400.
func Form[T any](c *Context, name string) (T, error) {
	value, ok := c.formValue(name)
	return parseRequestValue[T]("form", name, value, ok)
}

// FormOr returns the named form value parsed as T, or fallback when the value
// is missing or does not parse.
func FormOr[T any](c *Context, name string, fallback T) T {
	value, ok := c.formValue(name)
	if !ok {
		return fallback
	}
	if parsed, err := parseValue[T](value); err == nil {
		return parsed
	}
	return fallback
}

// Value returns the request-scoped value stored under key by Set, if it has
// type T.
//
//	user, ok := zinc.Value[*User](c, userKey)
func Value[T any](c *Context, key any) (T, bool) {
	value, ok := c.store[key].(T)
	return value, ok
}

// MustValue returns the request-scoped value stored under key, and panics
// when it is missing or has another type. Use it for values an earlier
// middleware always sets.
func MustValue[T any](c *Context, key any) T {
	stored, present := c.store[key]
	value, ok := stored.(T)
	if !ok {
		var zero T
		if !present {
			panic(fmt.Sprintf("zinc: no request value for key %v", key))
		}
		panic(fmt.Sprintf("zinc: request value for key %v is %T, not %T", key, stored, zero))
	}
	return value
}

// firstQueryValue matches Query, and also reports whether name was present.
func (c *Context) firstQueryValue(name string) (string, bool) {
	if c.queryParams != nil {
		values := c.queryParams[name]
		if len(values) == 0 {
			return "", false
		}
		return values[0], true
	}
	if c.request == nil || c.request.URL == nil {
		return "", false
	}
	return lookupRawQuery(c.request.URL.RawQuery, name)
}

func (c *Context) formValue(name string) (string, bool) {
	if c.request == nil || c.limitFormBody() != nil {
		return "", false
	}
	if c.request.Form == nil {
		_ = c.request.ParseMultipartForm(defaultMultipartMemory)
	}
	values := c.request.Form[name]
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

// errMissingValue is the cause of a BindError for an absent required value.
var errMissingValue = errors.New("value is required")

// parseRequestValue parses a present value, and reports a missing or invalid
// one as a BindError naming its source and name.
func parseRequestValue[T any](source, name, value string, present bool) (T, error) {
	if !present {
		var zero T
		return zero, &BindError{Source: source, Name: name, Reason: "is required", Err: errMissingValue}
	}
	parsed, err := parseValue[T](value)
	if err != nil {
		return parsed, &BindError{Source: source, Name: name, Reason: reasonFor[T](), Err: err}
	}
	return parsed, nil
}

// parseValue converts value to T. The common scalar types are handled by a
// type switch with no reflection or allocation; everything else goes through
// parseValueSlow, kept separate so its escaping values cannot force the fast
// path onto the heap.
func parseValue[T any](value string) (T, error) {
	var out T
	switch p := any(&out).(type) {
	case *int:
		n, err := strconv.Atoi(value)
		*p = n
		return out, err
	case *string:
		*p = value
	case *int64:
		n, err := strconv.ParseInt(value, 10, 64)
		*p = n
		return out, err
	case *int32:
		n, err := strconv.ParseInt(value, 10, 32)
		*p = int32(n)
		return out, err
	case *uint:
		n, err := strconv.ParseUint(value, 10, 0)
		*p = uint(n)
		return out, err
	case *uint64:
		n, err := strconv.ParseUint(value, 10, 64)
		*p = n
		return out, err
	case *uint32:
		n, err := strconv.ParseUint(value, 10, 32)
		*p = uint32(n)
		return out, err
	case *float64:
		n, err := strconv.ParseFloat(value, 64)
		*p = n
		return out, err
	case *float32:
		n, err := strconv.ParseFloat(value, 32)
		*p = float32(n)
		return out, err
	case *bool:
		b, err := strconv.ParseBool(value)
		*p = b
		return out, err
	default:
		return parseValueSlow[T](value)
	}
	return out, nil
}

// parseValueSlow handles encoding.TextUnmarshaler, pointers, and the remaining
// integer widths through the setters used by struct binding.
func parseValueSlow[T any](value string) (T, error) {
	var out T
	if u, ok := any(&out).(encoding.TextUnmarshaler); ok {
		return out, u.UnmarshalText([]byte(value))
	}
	setter := setterFor(reflect.TypeFor[T]())
	if setter.kind == fieldSetterUnsupportedKind || setter.kind == fieldSetterUnsupportedSlice ||
		setter.supportsFiles() {
		return out, fmt.Errorf("zinc: cannot parse a request value into %s", reflect.TypeFor[T]())
	}
	return out, setter.set(reflect.ValueOf(&out).Elem(), []string{value})
}

var setterCache sync.Map

func setterFor(typ reflect.Type) fieldSetter {
	if cached, ok := setterCache.Load(typ); ok {
		return cached.(fieldSetter)
	}
	setter := compileFieldSetter(typ)
	setterCache.Store(typ, setter)
	return setter
}

// reasonFor describes, for clients, the values T accepts.
func reasonFor[T any]() string {
	return setterFor(reflect.TypeFor[T]()).reason()
}
