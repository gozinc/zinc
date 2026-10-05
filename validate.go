// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
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
// the elements of a slice or map, and compare a number's value exactly, as
// the field's own type. A pointer that isn't nil counts as set, and
// omitempty skips only the rules after it. Nested structs are validated
// too, including those in slices and maps and those of a recursive type, as
// deep as the value goes, as the spec describes them; go-playground/validator
// needs dive for that.
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

// rulePlan checks a struct type's validate tags, enum values and patterns.
// Enum values, from an enum tag or an EnumProvider type, and pattern tags are
// Zinc's own claims, so they're always checked; validate tags only when Zinc
// is the validator.
type rulePlan struct {
	fields []ruleField
	// recursive is set when the plan can reach itself through its fields,
	// so a check tracks the structs on its path and stops at a cycle.
	recursive bool
	// unsupported lists validate rules outside BuiltinRules, as
	// "Type.Field: rule", for the error when Zinc is the validator.
	unsupported []string
	// invalid lists enum and pattern tags that can't be used, as
	// "Type.Field: why", for the error whatever the validator.
	invalid []string
}

type ruleField struct {
	index    []int
	name     string
	tagRules []rule
	// omitAt is omitempty's position in tagRules: an unset field skips the
	// rules from there on. It's len(tagRules) without omitempty.
	omitAt int
	// nilFails is set when required comes before any omitempty, so a nil
	// pointer fails.
	nilFails bool
	enum     []any
	enumMsg  string
	enumEach bool // the enum applies to each element of a slice, array or map
	pattern  *regexp.Regexp
	nested   *rulePlan
}

type rule struct {
	required bool
	check    func(v reflect.Value) string // "" when v passes
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
	plan := compileRulePlans(t)
	actual, _ := rulePlans.LoadOrStore(t, plan)
	plan, _ = actual.(*rulePlan)
	return plan
}

// ruleCompiler builds the plans for a struct type and the struct types it
// reaches. A type's plan exists before its fields are compiled, so a
// recursive field refers back to it.
type ruleCompiler struct {
	plans       map[reflect.Type]*rulePlan
	all         []*rulePlan
	flagged     map[*rulePlan]bool // has unsupported rules or invalid tags
	unsupported []string
	invalid     []string
}

// compileRulePlans returns the plan for struct type t, or nil when neither
// t nor anything it reaches has something to check.
func compileRulePlans(t reflect.Type) *rulePlan {
	c := &ruleCompiler{plans: map[reflect.Type]*rulePlan{}, flagged: map[*rulePlan]bool{}}
	root := c.compile(t)
	// A plan is live when it checks something itself or reaches a plan that
	// does. Recursion makes that a fixed point.
	live := map[*rulePlan]bool{}
	for changed := true; changed; {
		changed = false
		for _, p := range c.all {
			if !live[p] && c.checksSomething(p, live) {
				live[p], changed = true, true
			}
		}
	}
	if !live[root] {
		return nil
	}
	for _, p := range c.all {
		kept := p.fields[:0]
		for _, f := range p.fields {
			if f.nested != nil && !live[f.nested] {
				f.nested = nil
			}
			if len(f.tagRules) > 0 || len(f.enum) > 0 || f.pattern != nil || f.nested != nil {
				kept = append(kept, f)
			}
		}
		p.fields = kept
	}
	for _, p := range c.all {
		p.recursive = live[p] && reaches(p, p, map[*rulePlan]bool{})
	}
	root.unsupported, root.invalid = c.unsupported, c.invalid
	return root
}

func (c *ruleCompiler) checksSomething(p *rulePlan, live map[*rulePlan]bool) bool {
	if c.flagged[p] {
		return true
	}
	for _, f := range p.fields {
		if len(f.tagRules) > 0 || len(f.enum) > 0 || f.pattern != nil || (f.nested != nil && live[f.nested]) {
			return true
		}
	}
	return false
}

// reaches reports whether target can be reached from p's nested fields.
func reaches(p, target *rulePlan, seen map[*rulePlan]bool) bool {
	for _, f := range p.fields {
		if f.nested == nil {
			continue
		}
		if f.nested == target {
			return true
		}
		if !seen[f.nested] {
			seen[f.nested] = true
			if reaches(f.nested, target, seen) {
				return true
			}
		}
	}
	return false
}

func (c *ruleCompiler) compile(t reflect.Type) *rulePlan {
	if plan, ok := c.plans[t]; ok {
		return plan
	}
	plan := &rulePlan{}
	c.plans[t] = plan
	c.all = append(c.all, plan)
	for _, f := range reflect.VisibleFields(t) {
		if !f.IsExported() || (f.Anonymous && base(f.Type).Kind() == reflect.Struct) {
			continue
		}
		rf := ruleField{index: f.Index, name: wireName(f)}
		elem := base(f.Type)
		if text, ok := f.Tag.Lookup("enum"); ok {
			values, each, err := compileEnum(text, elem)
			if err != nil {
				c.flag(plan, &c.invalid, fmt.Sprintf("%s.%s: %v", t, f.Name, err))
			}
			rf.enum, rf.enumEach = values, each
		} else {
			rf.enum, rf.enumEach = providedEnum(elem)
		}
		if len(rf.enum) > 0 {
			rf.enumMsg = "must be one of: " + enumText(rf.enum)
		}
		if text, ok := f.Tag.Lookup("pattern"); ok {
			re, err := compilePattern(text, elem)
			if err != nil {
				c.flag(plan, &c.invalid, fmt.Sprintf("%s.%s: %v", t, f.Name, err))
			}
			rf.pattern = re
		}
		rf.omitAt = -1
		for _, token := range strings.Split(f.Tag.Get("validate"), ",") {
			token = strings.TrimSpace(token)
			name, param, _ := strings.Cut(token, "=")
			switch name {
			case "":
				continue
			case "omitempty":
				if rf.omitAt < 0 {
					rf.omitAt = len(rf.tagRules)
				}
				continue
			}
			check, err := compileRule(name, param, elem)
			if err != nil {
				entry := fmt.Sprintf("%s.%s: %s", t, f.Name, token)
				if err != errRuleUnsupported {
					entry += " (" + err.Error() + ")"
				}
				c.flag(plan, &c.unsupported, entry)
				continue
			}
			if name == "required" && rf.omitAt < 0 {
				rf.nilFails = true
			}
			rf.tagRules = append(rf.tagRules, rule{required: name == "required", check: check})
		}
		if rf.omitAt < 0 {
			rf.omitAt = len(rf.tagRules)
		}
		if st := nestedStruct(f.Type); st != nil {
			rf.nested = c.compile(st)
		}
		if len(rf.tagRules) > 0 || len(rf.enum) > 0 || rf.pattern != nil || rf.nested != nil {
			plan.fields = append(plan.fields, rf)
		}
	}
	return plan
}

func (c *ruleCompiler) flag(plan *rulePlan, list *[]string, entry string) {
	c.flagged[plan] = true
	*list = append(*list, entry)
}

// compileEnum parses an enum tag's values for a field whose type, pointers
// removed, is t: as t, or as its elements for a slice or array.
func compileEnum(text string, t reflect.Type) (values []any, each bool, err error) {
	et := t
	if (t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Uint8) || t.Kind() == reflect.Array {
		et, each = base(t.Elem()), true
	}
	noun := scalarNoun(et)
	if noun == "" {
		return nil, false, fmt.Errorf("enum applies to strings, numbers and booleans, and slices and arrays of them, not %s", t)
	}
	for _, v := range strings.Split(text, ",") {
		v = strings.TrimSpace(v)
		parsed, ok := parseScalar(v, et, false)
		if !ok {
			return nil, false, fmt.Errorf("enum value %q isn't %s", v, noun)
		}
		values = append(values, parsed)
	}
	return values, each, nil
}

// providedEnum returns the values of an EnumProvider type t, or of the
// elements of a slice, array or map of one.
func providedEnum(t reflect.Type) (values []any, each bool) {
	switch t.Kind() {
	case reflect.Struct:
		return nil, false
	case reflect.Slice, reflect.Array, reflect.Map:
		et := base(t.Elem())
		if et.Kind() == reflect.Struct {
			return nil, false
		}
		values = enumValuesFor(et)
		return values, values != nil
	}
	return enumValuesFor(t), false
}

// scalarNoun names the values of scalar type t for a message, or returns ""
// when t isn't a string, number or boolean.
func scalarNoun(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "an int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return "a uint"
	case reflect.Float32, reflect.Float64:
		return "a number"
	}
	return ""
}

// compilePattern compiles a pattern tag for a field whose type, pointers
// removed, is t. Like JSON Schema's pattern, it matches anywhere in the
// value unless anchored with ^ and $.
func compilePattern(text string, t reflect.Type) (*regexp.Regexp, error) {
	if t.Kind() != reflect.String {
		return nil, fmt.Errorf("pattern applies only to strings, not %s", t)
	}
	re, err := regexp.Compile(text)
	if err != nil {
		return nil, fmt.Errorf("pattern %q: %w", text, err)
	}
	return re, nil
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

// ruleWalk is one check's state. path holds the structs and maps of
// recursive plans on the way down to the current value; one met again is a
// cycle in memory, already being checked, so the walk ends. Others are
// checked wherever they're reached.
type ruleWalk struct {
	tags bool // validate tags apply: Zinc is the validator
	errs invalidFields
	path []ruleVisit
	// onPath indexes a deep path, so each step stays cheap.
	onPath map[ruleVisit]bool
}

type ruleVisit struct {
	plan *rulePlan
	addr uintptr
}

const ruleWalkIndexDepth = 32

// enter records a visit to addr for plan p, returning false when it's
// already on the path.
func (w *ruleWalk) enter(p *rulePlan, addr uintptr) bool {
	v := ruleVisit{p, addr}
	if w.onPath != nil {
		if w.onPath[v] {
			return false
		}
		w.onPath[v] = true
	} else if slices.Contains(w.path, v) {
		return false
	}
	w.path = append(w.path, v)
	if w.onPath == nil && len(w.path) > ruleWalkIndexDepth {
		w.onPath = make(map[ruleVisit]bool, 2*len(w.path))
		for _, seen := range w.path {
			w.onPath[seen] = true
		}
	}
	return true
}

func (w *ruleWalk) leave() {
	last := len(w.path) - 1
	if w.onPath != nil {
		delete(w.onPath, w.path[last])
	}
	w.path = w.path[:last]
}

func (w *ruleWalk) fail(name *ruleName, msg string) {
	if w.errs == nil {
		w.errs = invalidFields{}
	}
	w.errs[name.String()] = msg
}

// ruleName is a value's name as a chain from the root, written out only
// when the value fails, so naming a deep value costs nothing until then.
type ruleName struct {
	parent *ruleName
	field  string // a field's name, or the root's text
	key    string // a map value's key, when index is -1
	index  int    // an element's index, when field is unset
}

func (n *ruleName) String() string { return string(n.append(nil)) }

func (n *ruleName) append(b []byte) []byte {
	if n.parent == nil {
		return append(b, n.field...)
	}
	b = n.parent.append(b)
	switch {
	case n.field != "":
		if len(b) > 0 {
			b = append(b, '.')
		}
		return append(b, n.field...)
	case n.index < 0:
		return append(append(append(b, '['), n.key...), ']')
	}
	return append(strconv.AppendInt(append(b, '['), int64(n.index), 10), ']')
}

// check validates v, a struct value, naming its fields after prefix. tags
// is set when Zinc is the validator, so validate tags apply as well as
// enums.
func (p *rulePlan) check(v reflect.Value, tags bool, prefix string, errs invalidFields) invalidFields {
	w := ruleWalk{tags: tags, errs: errs}
	root := ruleName{field: strings.TrimSuffix(prefix, ".")}
	w.structValue(p, v, &root)
	return w.errs
}

// checkWithin checks the structs in v: v itself, or the elements of a
// slice, array or map, through pointers, named name[i] or name[key].
func (p *rulePlan) checkWithin(v reflect.Value, tags bool, name string, errs invalidFields) invalidFields {
	w := ruleWalk{tags: tags, errs: errs}
	root := ruleName{field: name}
	w.within(p, v, &root)
	return w.errs
}

func (w *ruleWalk) structValue(p *rulePlan, v reflect.Value, at *ruleName) {
	if p.recursive && v.CanAddr() {
		if !w.enter(p, v.UnsafeAddr()) {
			return
		}
		defer w.leave()
	}
	for i := range p.fields {
		f := &p.fields[i]
		fv, ok := fieldByIndex(v, f.index)
		if !ok {
			continue
		}
		name := ruleName{parent: at, field: f.name}
		if w.tags && len(f.tagRules) > 0 {
			if msg := f.checkTags(fv); msg != "" {
				w.fail(&name, msg)
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
		if f.enumEach {
			w.enumElements(f, fv, &name)
			continue
		}
		// A zero value is a field left out; required says whether that's
		// allowed.
		if len(f.enum) > 0 && !fv.IsZero() && !enumContains(f.enum, fv) {
			w.fail(&name, f.enumMsg)
			continue
		}
		if f.pattern != nil && !fv.IsZero() && !f.pattern.MatchString(fv.String()) {
			w.fail(&name, "must match the pattern "+f.pattern.String())
			continue
		}
		if f.nested != nil {
			w.within(f.nested, fv, &name)
		}
	}
}

// within checks the structs in v for plan p: v itself, or the elements of
// a slice, array or map, through pointers, named name[i] or name[key].
func (w *ruleWalk) within(p *rulePlan, v reflect.Value, at *ruleName) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		w.structValue(p, v, at)
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			w.within(p, v.Index(i), &ruleName{parent: at, index: i})
		}
	case reflect.Map:
		// Map values are copies, so a cycle through a map is caught at the
		// map.
		if p.recursive && v.Len() > 0 {
			if !w.enter(p, v.Pointer()) {
				return
			}
			defer w.leave()
		}
		iter := v.MapRange()
		for iter.Next() {
			w.within(p, iter.Value(), &ruleName{parent: at, key: fmt.Sprint(iter.Key().Interface()), index: -1})
		}
	}
}

// enumElements checks each element of v, a slice, array or map, against
// the field's enum. Unlike a field, an element that's its zero value was
// sent, so it's checked too.
func (w *ruleWalk) enumElements(f *ruleField, v reflect.Value, at *ruleName) {
	element := func(e reflect.Value) bool {
		for e.Kind() == reflect.Pointer {
			if e.IsNil() {
				return true
			}
			e = e.Elem()
		}
		return enumContains(f.enum, e)
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if !element(v.Index(i)) {
				w.fail(&ruleName{parent: at, index: i}, f.enumMsg)
			}
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if !element(iter.Value()) {
				w.fail(&ruleName{parent: at, key: fmt.Sprint(iter.Key().Interface()), index: -1}, f.enumMsg)
			}
		}
	}
}

// checkTags runs the field's validate rules in order, returning the first
// failure. As in go-playground/validator, a pointer that isn't nil is set,
// whatever it points to, and omitempty skips only the rules after it.
func (f *ruleField) checkTags(fv reflect.Value) string {
	viaPointer := false
	for fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			if f.nilFails {
				return "is required"
			}
			return ""
		}
		fv, viaPointer = fv.Elem(), true
	}
	for i := range f.tagRules {
		if i == f.omitAt && !hasValue(fv, viaPointer) {
			return ""
		}
		r := &f.tagRules[i]
		if r.required {
			if !hasValue(fv, viaPointer) {
				return "is required"
			}
			continue
		}
		if msg := r.check(fv); msg != "" {
			return msg
		}
	}
	return ""
}

// hasValue is go-playground/validator's test for required and omitempty: a
// slice, map or pointer that isn't nil, a value reached through a pointer,
// or a value that isn't its zero value.
func hasValue(v reflect.Value, viaPointer bool) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func:
		return !v.IsNil()
	}
	return viaPointer || !v.IsZero()
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

// errRuleUnsupported is compileRule's error for a rule Zinc doesn't have, or
// doesn't have for the field's type.
var errRuleUnsupported = errors.New("unsupported rule")

// compileRule returns the check for one validate rule on a field whose
// type, pointers removed, is t. The error is errRuleUnsupported when Zinc
// doesn't support the rule there, or says why its parameter can't be used.
func compileRule(name, param string, t reflect.Type) (func(reflect.Value) string, error) {
	kind := measureKind(t)
	switch name {
	case "required":
		return func(v reflect.Value) string {
			if v.IsZero() {
				return "is required"
			}
			return ""
		}, nil
	case "min", "max", "len", "gt", "gte", "lt", "lte":
		if kind == "" {
			return nil, errRuleUnsupported
		}
		return compileBound(name, param, t, kind)
	case "oneof":
		values := strings.Fields(param)
		if len(values) == 0 || (kind != "chars" && kind != "number") {
			return nil, errRuleUnsupported
		}
		var allowed []any
		for _, text := range values {
			parsed, ok := parseScalar(text, t, false)
			if !ok {
				return nil, errRuleUnsupported
			}
			allowed = append(allowed, parsed)
		}
		message := "must be one of: " + strings.Join(values, ", ")
		return func(v reflect.Value) string {
			if enumContains(allowed, v) {
				return ""
			}
			return message
		}, nil
	case "email", "uuid", "uuid4", "uuid_rfc4122", "uuid4_rfc4122", "url", "uri", "http_url":
		if t.Kind() != reflect.String {
			return nil, errRuleUnsupported
		}
		valid, message := formatRule(name)
		return func(v reflect.Value) string {
			if valid(v.String()) {
				return ""
			}
			return message
		}, nil
	}
	return nil, errRuleUnsupported
}

// boundValue is a min, max or other bound parsed in its field's domain: a
// length or signed integer, an unsigned integer, or a float.
type boundValue struct {
	i int64
	u uint64
	f float64
}

// parseBound parses a bound for a field of type t, which kind measures.
// Lengths and integers must be whole numbers the domain holds, and floats
// finite, so no bound is rounded into another.
func parseBound(param string, t reflect.Type, kind string) (boundValue, error) {
	var b boundValue
	var err error
	switch {
	case kind == "chars" || kind == "items" || isSigned(t):
		if b.i, err = strconv.ParseInt(param, 10, 64); err != nil {
			return b, fmt.Errorf("the bound must be a whole number from %d to %d", math.MinInt64, math.MaxInt64)
		}
	case isUnsigned(t):
		if b.u, err = strconv.ParseUint(param, 10, 64); err != nil {
			return b, fmt.Errorf("the bound must be a whole number from 0 to %d", uint64(math.MaxUint64))
		}
	default:
		if b.f, err = strconv.ParseFloat(param, t.Bits()); err != nil || math.IsNaN(b.f) || math.IsInf(b.f, 0) {
			return b, errors.New("the bound must be a finite number")
		}
	}
	return b, nil
}

// compileBound returns the check for min, max, len, gt, gte, lt or lte,
// comparing in the field's own domain: lengths and signed integers as
// int64, unsigned integers as uint64, floats as float64.
func compileBound(name, param string, t reflect.Type, kind string) (func(reflect.Value) string, error) {
	b, err := parseBound(param, t, kind)
	if err != nil {
		return nil, err
	}
	// allow says which outcomes of comparing the value with the bound pass:
	// less, equal, greater.
	allow := map[string][3]bool{
		"min": {false, true, true}, "gte": {false, true, true},
		"max": {true, true, false}, "lte": {true, true, false},
		"len": {false, true, false},
		"gt":  {false, false, true}, "lt": {true, false, false},
	}[name]
	message := boundMessage(name, kind, param)
	switch {
	case kind == "chars":
		return func(v reflect.Value) string {
			if allow[cmp.Compare(int64(utf8.RuneCountInString(v.String())), b.i)+1] {
				return ""
			}
			return message
		}, nil
	case kind == "items":
		return func(v reflect.Value) string {
			if allow[cmp.Compare(int64(v.Len()), b.i)+1] {
				return ""
			}
			return message
		}, nil
	case isSigned(t):
		return func(v reflect.Value) string {
			if allow[cmp.Compare(v.Int(), b.i)+1] {
				return ""
			}
			return message
		}, nil
	case isUnsigned(t):
		return func(v reflect.Value) string {
			if allow[cmp.Compare(v.Uint(), b.u)+1] {
				return ""
			}
			return message
		}, nil
	}
	return func(v reflect.Value) string {
		if f := v.Float(); !math.IsNaN(f) && allow[cmp.Compare(f, b.f)+1] {
			return ""
		}
		return message
	}, nil
}

func isSigned(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	}
	return false
}

func isUnsigned(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
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
		return func(s string) bool { return isUUID(s, false, false) }, "must be a UUID"
	case "uuid4":
		return func(s string) bool { return isUUID(s, true, true) }, "must be a version 4 UUID"
	case "uuid4_rfc4122":
		return func(s string) bool { return isUUID(s, true, false) }, "must be a version 4 UUID"
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

// isUUID reports whether s is a UUID in its 36-character form. With v4, it
// must be version 4 with the RFC 4122 variant; with lower, in lower case,
// as go-playground/validator's uuid4 requires.
func isUUID(s string, v4, lower bool) bool {
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
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || !lower && 'A' <= c && c <= 'F') {
				return false
			}
		}
	}
	if !v4 {
		return true
	}
	return s[14] == '4' && strings.IndexByte("89abAB", s[19]) >= 0
}

// ruleSupportError reports a typed handler's input or output with an enum
// or pattern tag that can't be used, or using a validate
// rule the app's validator doesn't declare, so no rule in a tag is silently
// unenforced. A validator that declares nothing isn't checked for rules.
func ruleSupportError(types handlerTypes, cfg *Config) error {
	// Enum and pattern tags are checked whatever the validator, so a bad one
	// is always an error.
	for _, t := range []reflect.Type{types.in, types.out} {
		if plan := rulePlanFor(nestedStruct(t)); plan != nil && len(plan.invalid) > 0 {
			return fmt.Errorf("zinc: these enum and pattern tags can't be used: %s", strings.Join(plan.invalid, ", "))
		}
	}
	var v Validator
	if cfg != nil {
		v = cfg.Validator
	}
	enforced := enforcedRules(v)
	if enforced == nil {
		return nil
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
			if plan := rulePlanFor(nestedStruct(t)); plan != nil {
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
			return fmt.Errorf("zinc: %s doesn't enforce these validate rules: %s; use rules it declares, or a Validator that declares them with RuleSet", who, strings.Join(missing, ", "))
		}
	}
	return nil
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
