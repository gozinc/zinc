// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"reflect"
	"unicode/utf8"
)

// A body never fills a field tagged only for the path, query, headers or
// cookies. A decode can still write one, since encoding/json matches keys to
// field names, so binders reset those fields after it, or put back values an
// earlier binder set.

// bodyMentionsParams reports whether body could have filled a
// parameter-only field: whether any of their names appear in it, ignoring
// ASCII case, as encoding/json matches them. A key can also reach a field
// spelled another way: escaped ("\u0072ole" is "role") or folded from a
// non-ASCII letter ("ſort" matches Sort, the Kelvin sign matches K). The
// scan can't see through either, so a body with a backslash or a non-ASCII
// byte counts as mentioning them.
func (plan *bindingPlan) bodyMentionsParams(body []byte) bool {
	if plan.paramNamesFold {
		return true
	}
	for i, b := range body {
		if b == '\\' || b >= utf8.RuneSelf {
			return true
		}
		if !plan.paramFirst[b] {
			continue
		}
		for _, name := range plan.paramNames {
			if len(name) <= len(body)-i && hasFoldPrefixASCII(body[i:], name) {
				return true
			}
		}
	}
	return false
}

// hasFoldPrefixASCII reports whether s starts with name, a lower-case ASCII
// string, ignoring ASCII case.
func hasFoldPrefixASCII(s []byte, name string) bool {
	for j := 0; j < len(name); j++ {
		b := s[j]
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if b != name[j] {
			return false
		}
	}
	return true
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
// returns nil when there's nothing to restore: v has none, or the body
// doesn't mention them, so the decode can't touch them. A configured decoder
// (custom) can fill a field from any key, so for one the body is never
// scanned and the fields are always recorded. It also returns v's binding
// plan, nil unless v points to a struct, for validating v after.
func snapshotParams(c *Context, v any, custom bool) (*paramSnapshot, *bindingPlan) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return nil, nil
	}
	plan := bindingPlanFor(rv.Elem().Type())
	if len(plan.paramOnly) == 0 {
		return nil, plan
	}
	if body, err := c.readAndCacheBodyBytes(); !custom && err == nil && !plan.bodyMentionsParams(body) {
		return nil, plan
	}
	snap := &paramSnapshot{plan: plan, val: rv.Elem()}
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
	return snap, plan
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
