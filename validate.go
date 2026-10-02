// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"net/mail"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// RuleSetValidator is a Validator that declares the validate-tag rules it
// enforces, such as "required", "min" and "email". The OpenAPI spec claims
// only those rules, and a typed handler whose input or output uses any other
// rule panics when it's registered, so a rule is never silently unenforced.
// A Validator without RuleSet makes no claims: the spec lists no validate
// rules, and nothing is checked at registration.
type RuleSetValidator interface {
	Validator
	RuleSet() []string
}

// BuiltinRules are the validate-tag rules Zinc enforces itself when
// Config.Validator is nil. They mean what they mean to
// go-playground/validator, so a struct's tags work with either:
//
//	required, omitempty, min, max, len, gt, gte, lt, lte, oneof,
//	email, uuid, uuid4, url, uri, http_url
//
// min, max, len, gt, gte, lt and lte count the characters of a string and
// the elements of a slice or map, and compare a number's value. Nested
// structs are validated too, including those in slices and maps, as the
// spec describes them; go-playground/validator needs dive for that.
func BuiltinRules() []string {
	return slices.Clone(builtinRuleNames)
}

var builtinRuleNames = []string{
	"required", "omitempty", "min", "max", "len", "gt", "gte", "lt", "lte", "oneof",
	"email", "uuid", "uuid4", "uuid_rfc4122", "uuid4_rfc4122", "url", "uri", "http_url",
}

// enforcedRules returns the rules the app's validator enforces, or nil when
// it doesn't say.
func enforcedRules(v Validator) map[string]bool {
	var names []string
	switch v := v.(type) {
	case nil:
		names = builtinRuleNames
	case RuleSetValidator:
		names = v.RuleSet()
	default:
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// invalidFields are invalid fields, by name, and what's wrong with each.
type invalidFields map[string]string

func (e invalidFields) Error() string {
	names := make([]string, 0, len(e))
	for name := range e {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + " " + e[name]
	}
	return strings.Join(parts, "; ")
}

// Fields returns the messages by field name.
func (e invalidFields) Fields() map[string]string { return e }

// rulePlan checks a struct type's validate tags and enum values. Enum
// values, from an enum tag or an EnumProvider type, are Zinc's own claims,
// so they're always checked; validate tags only when Zinc is the validator.
type rulePlan struct {
	fields []ruleField
	// unsupported lists validate rules outside BuiltinRules, as
	// "Type.Field: rule", for the error when Zinc is the validator.
	unsupported []string
}

type ruleField struct {
	index    []int
	name     string
	typ      reflect.Type // the field's type, pointers included
	tagRules []rule
	omit     bool // omitempty: a zero value skips tagRules
	enum     []any
	nested   *rulePlan
}

type rule struct {
	name  string
	check func(v reflect.Value) string // "" when v passes
}

var rulePlans sync.Map // reflect.Type -> *rulePlan (nil when nothing to check)

// rulePlanFor returns the plan for struct type t, or nil when t has nothing
// to check.
func rulePlanFor(t reflect.Type) *rulePlan {
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	if cached, ok := rulePlans.Load(t); ok {
		plan, _ := cached.(*rulePlan)
		return plan
	}
	plan := compileRulePlan(t, map[reflect.Type]bool{})
	actual, _ := rulePlans.LoadOrStore(t, plan)
	plan, _ = actual.(*rulePlan)
	return plan
}

func compileRulePlan(t reflect.Type, visiting map[reflect.Type]bool) *rulePlan {
	if visiting[t] {
		return nil // a recursive type is checked to the depth its values have
	}
	visiting[t] = true
	defer delete(visiting, t)
	plan := &rulePlan{}
	for _, f := range reflect.VisibleFields(t) {
		if !f.IsExported() || (f.Anonymous && base(f.Type).Kind() == reflect.Struct) {
			continue
		}
		rf := ruleField{index: f.Index, name: wireName(f), typ: f.Type}
		elem := base(f.Type)
		if text, ok := f.Tag.Lookup("enum"); ok {
			for _, v := range strings.Split(text, ",") {
				if parsed, ok := parseScalar(strings.TrimSpace(v), elem, false); ok {
					rf.enum = append(rf.enum, parsed)
				}
			}
		} else if provider, ok := reflect.New(elem).Elem().Interface().(EnumProvider); ok && elem.Kind() != reflect.Struct {
			rf.enum = provider.Enum()
		}
		for _, token := range strings.Split(f.Tag.Get("validate"), ",") {
			name, param, _ := strings.Cut(strings.TrimSpace(token), "=")
			switch name {
			case "":
				continue
			case "omitempty":
				rf.omit = true
				continue
			}
			check, ok := compileRule(name, param, elem)
			if !ok {
				plan.unsupported = append(plan.unsupported, fmt.Sprintf("%s.%s: %s", t, f.Name, strings.TrimSpace(token)))
				continue
			}
			rf.tagRules = append(rf.tagRules, rule{name: name, check: check})
		}
		if st := nestedStruct(f.Type); st != nil {
			if nested := compileRulePlan(st, visiting); nested != nil {
				rf.nested = nested
				plan.unsupported = append(plan.unsupported, nested.unsupported...)
			}
		}
		if len(rf.tagRules) > 0 || len(rf.enum) > 0 || rf.nested != nil {
			plan.fields = append(plan.fields, rf)
		}
	}
	if len(plan.fields) == 0 && len(plan.unsupported) == 0 {
		return nil
	}
	return plan
}

// wireName is the name a client knows a field by: its JSON name, or else
// the name it's bound from, or else its Go name.
func wireName(f reflect.StructField) string {
	for _, tag := range [...]string{"json", "path", "query", "header", "cookie", "form", "xml"} {
		if name, _, _ := strings.Cut(f.Tag.Get(tag), ","); name != "" && name != "-" {
			return name
		}
	}
	return f.Name
}

// check validates v, a struct value. tags is set when Zinc is the
// validator, so validate tags apply as well as enums.
func (p *rulePlan) check(v reflect.Value, tags bool, prefix string, errs invalidFields) invalidFields {
	for i := range p.fields {
		f := &p.fields[i]
		fv, ok := fieldByIndex(v, f.index)
		if !ok {
			continue
		}
		name := prefix + f.name
		if tags && len(f.tagRules) > 0 {
			if msg := f.checkTags(fv); msg != "" {
				if errs == nil {
					errs = invalidFields{}
				}
				errs[name] = msg
				continue
			}
		}
		for fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				break
			}
			fv = fv.Elem()
		}
		if fv.Kind() == reflect.Pointer {
			continue // nil
		}
		// A zero value is a field left out; required says whether that's
		// allowed.
		if len(f.enum) > 0 && !fv.IsZero() && !enumContains(f.enum, fv) {
			if errs == nil {
				errs = invalidFields{}
			}
			errs[name] = "must be one of: " + enumText(f.enum)
			continue
		}
		if f.nested != nil {
			errs = f.nested.checkWithin(fv, tags, name, errs)
		}
	}
	return errs
}

// checkWithin checks the structs in v: v itself, or the elements of a
// slice, array or map, through pointers, named name[i] or name[key].
func (p *rulePlan) checkWithin(v reflect.Value, tags bool, name string, errs invalidFields) invalidFields {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return errs
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		return p.check(v, tags, name+".", errs)
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			errs = p.checkWithin(v.Index(i), tags, name+"["+strconv.Itoa(i)+"]", errs)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			errs = p.checkWithin(iter.Value(), tags, name+"["+fmt.Sprint(iter.Key().Interface())+"]", errs)
		}
	}
	return errs
}

// checkTags runs the field's validate rules, returning the first failure.
func (f *ruleField) checkTags(fv reflect.Value) string {
	for fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			if slices.ContainsFunc(f.tagRules, func(r rule) bool { return r.name == "required" }) {
				return "is required"
			}
			return ""
		}
		fv = fv.Elem()
	}
	if f.omit && fv.IsZero() {
		return ""
	}
	for _, r := range f.tagRules {
		if msg := r.check(fv); msg != "" {
			return msg
		}
	}
	return ""
}

func enumContains(values []any, v reflect.Value) bool {
	for _, e := range values {
		ev := reflect.ValueOf(e)
		switch {
		case ev.CanInt() && v.CanInt():
			if ev.Int() == v.Int() {
				return true
			}
		case ev.CanUint() && v.CanUint():
			if ev.Uint() == v.Uint() {
				return true
			}
		case ev.CanFloat() && v.CanFloat():
			if ev.Float() == v.Float() {
				return true
			}
		case ev.Kind() == reflect.String && v.Kind() == reflect.String:
			if ev.String() == v.String() {
				return true
			}
		case ev.Kind() == reflect.Bool && v.Kind() == reflect.Bool:
			if ev.Bool() == v.Bool() {
				return true
			}
		}
	}
	return false
}

func enumText(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, ", ")
}

// compileRule returns the check for one validate rule on a field whose
// type, pointers removed, is t; false when Zinc doesn't support it there.
func compileRule(name, param string, t reflect.Type) (func(reflect.Value) string, bool) {
	kind := measureKind(t)
	switch name {
	case "required":
		return func(v reflect.Value) string {
			if v.IsZero() {
				return "is required"
			}
			return ""
		}, true
	case "min", "max", "len", "gt", "gte", "lt", "lte":
		if kind == "" {
			return nil, false
		}
		n, err := strconv.ParseFloat(param, 64)
		if err != nil {
			return nil, false
		}
		message := boundMessage(name, kind, param)
		return func(v reflect.Value) string {
			if boundHolds(name, measure(v), n) {
				return ""
			}
			return message
		}, true
	case "oneof":
		values := strings.Fields(param)
		if len(values) == 0 || (kind != "chars" && kind != "number") {
			return nil, false
		}
		var allowed []any
		for _, text := range values {
			parsed, ok := parseScalar(text, t, false)
			if !ok {
				return nil, false
			}
			allowed = append(allowed, parsed)
		}
		message := "must be one of: " + strings.Join(values, ", ")
		return func(v reflect.Value) string {
			if enumContains(allowed, v) {
				return ""
			}
			return message
		}, true
	case "email", "uuid", "uuid4", "uuid_rfc4122", "uuid4_rfc4122", "url", "uri", "http_url":
		if t.Kind() != reflect.String {
			return nil, false
		}
		valid, message := formatRule(name)
		return func(v reflect.Value) string {
			if valid(v.String()) {
				return ""
			}
			return message
		}, true
	}
	return nil, false
}

// measureKind says what min, max and the others compare for type t:
// characters, items or a number; "" when they don't apply.
func measureKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "chars"
	case reflect.Slice, reflect.Array, reflect.Map:
		return "items"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	}
	return ""
}

func measure(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.String:
		return float64(utf8.RuneCountInString(v.String()))
	case reflect.Slice, reflect.Array, reflect.Map:
		return float64(v.Len())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	}
	return v.Float()
}

func boundHolds(name string, got, n float64) bool {
	switch name {
	case "min", "gte":
		return got >= n
	case "max", "lte":
		return got <= n
	case "len":
		return got == n
	case "gt":
		return got > n
	}
	return got < n // lt
}

func boundMessage(name, kind, n string) string {
	relation := map[string]string{
		"min": "at least", "gte": "at least", "max": "at most", "lte": "at most",
		"len": "exactly", "gt": "more than", "lt": "fewer than",
	}[name]
	switch kind {
	case "chars":
		if name == "lt" {
			relation = "fewer than"
		}
		return "must be " + relation + " " + n + " characters"
	case "items":
		return "must have " + relation + " " + n + " items"
	}
	switch name {
	case "gt":
		relation = "greater than"
	case "lt":
		relation = "less than"
	case "len":
		return "must be " + n
	}
	return "must be " + relation + " " + n
}

func formatRule(name string) (func(string) bool, string) {
	switch name {
	case "email":
		return func(s string) bool {
			addr, err := mail.ParseAddress(s)
			return err == nil && addr.Address == s && addr.Name == ""
		}, "must be an email address"
	case "uuid", "uuid_rfc4122":
		return func(s string) bool { return isUUID(s, 0) }, "must be a UUID"
	case "uuid4", "uuid4_rfc4122":
		return func(s string) bool { return isUUID(s, '4') }, "must be a version 4 UUID"
	case "uri":
		return func(s string) bool { _, err := url.ParseRequestURI(s); return err == nil }, "must be a URI"
	case "http_url":
		return func(s string) bool {
			u, err := url.Parse(s)
			return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
		}, "must be an http or https URL"
	}
	return func(s string) bool {
		u, err := url.Parse(s)
		return err == nil && u.Scheme != "" && (u.Host != "" || u.Opaque != "" || u.Path != "")
	}, "must be a URL"
}

// isUUID reports whether s is a UUID in its 36-character form, with
// version when it isn't 0.
func isUUID(s string, version byte) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
				return false
			}
		}
	}
	return version == 0 || s[14] == version
}

// checkRuleSupport panics when a typed handler's input or output uses a
// validate rule the app's validator doesn't declare, so no rule in a tag is
// silently unenforced. A validator that declares nothing isn't checked.
func checkRuleSupport(types handlerTypes, cfg *Config) {
	var v Validator
	if cfg != nil {
		v = cfg.Validator
	}
	enforced := enforcedRules(v)
	if enforced == nil {
		return
	}
	for _, t := range []reflect.Type{types.in, types.out} {
		if t == nil {
			continue
		}
		var missing []string
		for _, ft := range ruleTypes(t) {
			for _, f := range reflect.VisibleFields(ft) {
				for _, token := range strings.Split(f.Tag.Get("validate"), ",") {
					name, _, _ := strings.Cut(strings.TrimSpace(token), "=")
					if name != "" && !enforced[name] {
						missing = append(missing, fmt.Sprintf("%s.%s: %s", ft, f.Name, strings.TrimSpace(token)))
					}
				}
			}
		}
		// Zinc's own rules can also fail on a field type, such as min on a bool.
		if v == nil {
			if st := nestedStruct(t); st != nil && rulePlanFor(st) != nil {
				plan := rulePlanFor(st)
				for _, u := range plan.unsupported {
					if !slices.Contains(missing, u) {
						missing = append(missing, u)
					}
				}
			}
		}
		if len(missing) > 0 {
			who := "Zinc's built-in validator"
			if v != nil {
				who = fmt.Sprintf("the Validator (%T)", v)
			}
			panic(fmt.Sprintf("zinc: %s doesn't enforce these validate rules: %s; use rules it declares, or a Validator that declares them with RuleSet", who, strings.Join(missing, ", ")))
		}
	}
}

// ruleTypes lists t and the struct types nested in it, which validation
// reaches.
func ruleTypes(t reflect.Type) []reflect.Type {
	t = nestedStruct(t)
	if t == nil {
		return nil
	}
	var out []reflect.Type
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		if slices.Contains(out, t) {
			return
		}
		out = append(out, t)
		for _, f := range reflect.VisibleFields(t) {
			if f.IsExported() && !f.Anonymous {
				if et := nestedStruct(f.Type); et != nil {
					walk(et)
				}
			}
		}
	}
	walk(t)
	return out
}

// validateOutput checks a typed handler's output for Config.ValidateResponses.
// A failure is the server's, so it's a plain error, answered with 500.
func (c *Context) validateOutput(out any) error {
	v := reflect.ValueOf(out)
	if !v.IsValid() {
		return nil
	}
	custom := c.app.config.Validator
	if plan := rulePlanFor(nestedStruct(v.Type())); plan != nil {
		if errs := plan.checkWithin(v, custom == nil, "response", nil); errs != nil {
			return fmt.Errorf("zinc: %s %s: the response breaks its contract: %w", c.Method(), c.FullPath(), errs)
		}
	}
	if custom == nil || base(v.Type()).Kind() != reflect.Struct {
		return nil
	}
	ptr := reflect.New(base(v.Type()))
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	ptr.Elem().Set(v)
	if err := custom.Validate(ptr.Interface()); err != nil {
		return fmt.Errorf("zinc: %s %s: the response breaks its contract: %w", c.Method(), c.FullPath(), err)
	}
	return nil
}
