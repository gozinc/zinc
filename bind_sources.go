// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
)

// Each source fills the fields tagged for it: form values, the query, headers,
// cookies, uploaded files and path parameters.

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
		if len(field.media) > 0 {
			for _, file := range inputs {
				if got := requestMediaType(file.Header.Get(HeaderContentType)); !slices.Contains(field.media, got) {
					want := strings.Join(field.media, " or ")
					return &bindFieldError{Source: "form", Field: field.label, Name: field.name, Reason: "must be " + want,
						Err: fmt.Errorf("file %q is %q, want %s", file.Filename, got, want)}
				}
			}
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
	// Media types are case-insensitive: Application/JSON is JSON.
	base, _, _ := strings.Cut(header, ";")
	return strings.ToLower(strings.TrimSpace(base))
}

// mediaTag returns a file field's accepted content types, from a tag such
// as media:"image/png,image/jpeg".
func mediaTag(field reflect.StructField) []string {
	text, ok := field.Tag.Lookup("media")
	if !ok || text == "" {
		return nil
	}
	var out []string
	for _, mt := range strings.Split(text, ",") {
		if mt = strings.TrimSpace(mt); mt != "" {
			out = append(out, mt)
		}
	}
	return out
}
