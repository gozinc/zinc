// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// outputHeader is a field of a typed handler's output that's sent as a
// response header rather than in the body.
type outputHeader struct {
	index  []int
	name   string // canonical header name
	field  string // Go field name, for messages
	t      reflect.Type
	cookie bool // *http.Cookie or []*http.Cookie, sent as Set-Cookie
}

var (
	cookieType      = reflect.TypeFor[*http.Cookie]()
	cookieSliceType = reflect.TypeFor[[]*http.Cookie]()
)

var outputHeaderCache sync.Map // reflect.Type -> []outputHeader

// outputHeadersFor returns the header fields of output type t, a struct or
// a pointer to one, including those of embedded structs, or nil when it
// has none. A field is a header when it's tagged header and json:"-". It
// panics when such a field can't be written as a header.
func outputHeadersFor(t reflect.Type) []outputHeader {
	if cached, ok := outputHeaderCache.Load(t); ok {
		return cached.([]outputHeader)
	}
	st := t
	if st.Kind() == reflect.Pointer {
		st = st.Elem()
	}
	var headers []outputHeader
	if st.Kind() == reflect.Struct {
		headers = collectOutputHeaders(st, nil, map[reflect.Type]bool{})
	}
	outputHeaderCache.Store(t, headers)
	return headers
}

func collectOutputHeaders(st reflect.Type, prefix []int, seen map[reflect.Type]bool) []outputHeader {
	if seen[st] {
		return nil
	}
	seen[st] = true
	defer delete(seen, st)
	var headers []outputHeader
	for i := range st.NumField() {
		f := st.Field(i)
		index := append(append([]int(nil), prefix...), i)
		name, tagged := f.Tag.Lookup("header")
		if !tagged {
			if f.Anonymous {
				et := f.Type
				if et.Kind() == reflect.Pointer {
					et = et.Elem()
				}
				if et.Kind() == reflect.Struct && et != timeType {
					headers = append(headers, collectOutputHeaders(et, index, seen)...)
				}
			}
			continue
		}
		// A header field that's also in the body is an input field of a
		// type that's echoed back, so it stays in the body; App.Validate
		// reports it.
		if name == "" || name == "-" || !f.IsExported() || f.Tag.Get("json") != "-" {
			continue
		}
		h := outputHeader{index: index, name: http.CanonicalHeaderKey(name), field: st.Name() + "." + f.Name, t: f.Type}
		h.cookie = f.Type == cookieType || f.Type == cookieSliceType
		switch {
		case h.cookie && h.name != "Set-Cookie":
			panic(fmt.Sprintf(`zinc: output field %s.%s is a cookie, so its tag must be header:"Set-Cookie"`, st, f.Name))
		case !h.cookie && h.name == "Set-Cookie":
			panic(fmt.Sprintf("zinc: output field %s.%s sets Set-Cookie, so it must be a *http.Cookie or []*http.Cookie", st, f.Name))
		case !h.cookie && !headerValueType(f.Type):
			panic(fmt.Sprintf("zinc: output field %s.%s can't be the %s header: %s isn't a string, number, bool, time.Time, or a slice or pointer of one", st, f.Name, h.name, f.Type))
		}
		headers = append(headers, h)
	}
	return headers
}

// headerValueType reports whether a field of type t can be written as
// header values.
func headerValueType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return false
		}
		e := t.Elem()
		return e.Kind() != reflect.Pointer && e.Kind() != reflect.Slice && headerValueType(e)
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return t == timeType
}

// writeOutputHeaders sets the header fields of out on the response. An
// empty string, a nil pointer or slice and a zero time.Time send nothing;
// a slice sends one value per element, and a number or bool is always sent.
func (c *Context) writeOutputHeaders(headers []outputHeader, out reflect.Value) {
	if out.Kind() == reflect.Pointer {
		if out.IsNil() {
			return
		}
		out = out.Elem()
	}
	for _, h := range headers {
		v, ok := fieldByIndex(out, h.index)
		if !ok {
			continue
		}
		if h.cookie {
			if v.Kind() == reflect.Slice {
				for i := range v.Len() {
					if cookie := v.Index(i).Interface().(*http.Cookie); cookie != nil {
						c.SetCookie(cookie)
					}
				}
			} else if cookie, _ := v.Interface().(*http.Cookie); cookie != nil {
				c.SetCookie(cookie)
			}
			continue
		}
		switch v.Kind() {
		case reflect.Slice:
			for i := range v.Len() {
				c.AppendHeader(h.name, headerText(v.Index(i)))
			}
		case reflect.Pointer:
			if !v.IsNil() {
				c.SetHeader(h.name, headerText(v.Elem()))
			}
		default:
			if (v.Kind() == reflect.String || v.Type() == timeType) && v.IsZero() {
				continue
			}
			c.SetHeader(h.name, headerText(v))
		}
	}
}

// fieldByIndex is reflect.Value.FieldByIndex that reports false, rather
// than panicking, at a nil embedded pointer.
func fieldByIndex(v reflect.Value, index []int) (reflect.Value, bool) {
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, true
}

func headerText(v reflect.Value) string {
	if v.Type() == timeType {
		return v.Interface().(time.Time).UTC().Format(http.TimeFormat)
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'g', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	}
	return strings.TrimSpace(fmt.Sprint(v.Interface()))
}
