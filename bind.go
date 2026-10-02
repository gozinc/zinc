// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

// Validator validates a value after binding.
type Validator interface {
	Validate(any) error
}

// Renderer renders named templates into a response writer.
type Renderer interface {
	Render(w io.Writer, name string, data any, c *Context) error
}

// Bind exposes source-specific binding for a Context.
type Bind struct {
	c *Context
}

// BindError identifies the request source and struct field that failed.
type BindError struct {
	// Source is "path", "query", "header", "cookie", "form", or "body".
	Source string
	// Field is the Go struct field, for logs.
	Field string
	// Name is the request-facing name: the tag value, or the JSON field path.
	Name string
	// Reason is a client-safe description of what the value must be, such as
	// "must be an integer". It is empty when there is nothing safe to say.
	Reason string
	Err    error
}

// errEmptyBody reports a missing body to a binder that requires one.
var errEmptyBody = errors.New("request body is empty")

// StatusCode implements StatusCoder: a binding failure is the client's error.
func (*BindError) StatusCode() int {
	return StatusBadRequest
}

var bindSourceNouns = map[string]string{
	"path":   "path parameter",
	"query":  "query parameter",
	"header": "header",
	"cookie": "cookie",
	"form":   "form field",
}

// clientMessage describes the failure without decoder text or Go names.
func (e *BindError) clientMessage() (string, map[string]string) {
	if errors.Is(e.Err, errEmptyBody) {
		return "request body is empty", nil
	}
	message := "invalid request body"
	if noun, ok := bindSourceNouns[e.Source]; ok {
		message = "invalid " + noun
	}
	if e.Name == "" {
		return message, nil
	}
	reason := e.Reason
	if reason == "" {
		reason = "invalid value"
	}
	return message, map[string]string{e.Name: reason}
}

// Error formats the binding source, field, and underlying error.
func (e *BindError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field != "" {
		return fmt.Sprintf("bind %s %s: %v", e.Source, e.Field, e.Err)
	}
	return fmt.Sprintf("bind %s: %v", e.Source, e.Err)
}

// Unwrap exposes the underlying conversion or decoding error.
func (e *BindError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// bindAll binds every source a typed handler binds, then validates v.
func bindAll(c *Context, v any) error {
	return bindRequest(c, v)
}

// bindRequest fills v from the body first, then headers, cookies, query
// values and path parameters, and validates once at the end. Default tags
// fill their fields before any source. A field tagged only for the path,
// query, headers or cookies is never filled from the body: encoding/json
// matches keys to field names case-insensitively, so each body decode resets
// those fields. A GET or HEAD request's body isn't read, as it has no meaning
// there and the spec never documents one.
func bindRequest(c *Context, v any) error {
	mediaType := requestMediaType(c.Header(HeaderContentType))
	if mediaType == "text/plain" {
		return bindPlainTextBody(c, v, false)
	}
	// A configured decoder fills a non-struct target, such as a map, from the
	// body alone. Struct targets merge the body, query, then path like JSON.
	decode := c.decoderFor(mediaType)
	if decode != nil {
		typ := reflect.TypeOf(v)
		if typ == nil || typ.Kind() != reflect.Pointer || typ.Elem().Kind() != reflect.Struct {
			return decodeBody(c, decode, v, false)
		}
	}

	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if plan.hasDefaults {
		applyDefaults(val, plan.formFields)
		applyDefaults(val, plan.queryFields)
		applyDefaults(val, plan.headerFields)
		applyDefaults(val, plan.cookieFields)
	}
	if method := c.Method(); method != MethodGet && method != MethodHead {
		if err := bindRequestBody(c, v, val, plan, mediaType, decode); err != nil {
			return err
		}
		// Only a decoded body can name these fields; a form binds by tag.
		if len(plan.paramOnly) > 0 && c.bodyRead && plan.bodyMentionsParams(c.body) {
			plan.keepParamsOutOfBody(val)
		}
	}
	req := c.Request()
	if len(plan.headerFields) > 0 && req != nil {
		if err := bindFieldsFromHeader(val, plan.headerFields, req.Header); err != nil {
			return wrapBindError("header", err)
		}
	}
	if len(plan.cookieFields) > 0 {
		if err := bindFieldsFromCookies(val, plan.cookieFields, req); err != nil {
			return wrapBindError("cookie", err)
		}
	}
	if len(plan.queryFields) > 0 && req != nil && req.URL != nil && req.URL.RawQuery != "" {
		if err := bindFieldsFromQuery(val, plan.queryFields, c); err != nil {
			return wrapBindError("query", err)
		}
	}
	if err := bindFieldsFromPath(val, plan.pathFields, c); err != nil {
		return wrapBindError("path", err)
	}
	if c.app == nil {
		return nil
	}
	return c.validate(v, plan.rules)
}

// bindRequestBody decodes an optional body into v without validating it. A
// missing or empty body is not an error.
func bindRequestBody(c *Context, v any, val reflect.Value, plan *bindingPlan, mediaType string, decode Decoder) error {
	req := c.Request()
	if req == nil || req.Body == nil {
		return nil
	}
	if decode != nil {
		return decodeBodyOnly(c, decode, v, false)
	}

	switch mediaType {
	case "", "application/json":
		bodyLen, readErr, decodeErr := c.readAndCacheJSONBody(v)
		if readErr != nil {
			return wrapBindError("body", readErr)
		}
		if bodyLen > 0 && decodeErr != nil {
			return classifyJSONDecodeError(decodeErr)
		}
	case "application/xml", "text/xml":
		bodyLen, readErr, decodeErr := c.readAndCacheBody(func(r io.Reader) error {
			return xml.NewDecoder(r).Decode(v)
		})
		if readErr != nil {
			return wrapBindError("body", readErr)
		}
		if bodyLen > 0 && decodeErr != nil {
			return wrapBindError("body", decodeErr)
		}
	case "application/x-www-form-urlencoded":
		if err := c.limitFormBody(); err != nil {
			return wrapBindError("form", err)
		}
		if err := req.ParseForm(); err != nil {
			return wrapBindError("form", fmt.Errorf("parse form: %w", err))
		}
		if err := bindFieldsFromValues(val, plan.formFields, req.Form); err != nil {
			return wrapBindError("form", err)
		}
	case "multipart/form-data":
		if err := c.limitFormBody(); err != nil {
			return wrapBindError("form", err)
		}
		if err := bindMultipartForm(val, plan, req); err != nil {
			return wrapBindError("form", err)
		}
	default:
		return wrapBindError("body", fmt.Errorf("unsupported content type: %s", mediaType))
	}
	return nil
}

// bindBody binds the request body according to its Content-Type.
func bindBody(c *Context, v any) error {
	mediaType := requestMediaType(c.Header(HeaderContentType))
	if decode := c.decoderFor(mediaType); decode != nil {
		return decodeBody(c, decode, v, true)
	}
	snap, plan := snapshotParams(c, v)
	if snap != nil {
		defer snap.restore()
	}
	switch mediaType {
	case "", "application/json":
		bodyLen, readErr, decodeErr := c.readAndCacheJSONBody(v)
		if readErr != nil {
			return wrapBindError("body", readErr)
		}
		if bodyLen == 0 {
			return wrapBindError("body", errEmptyBody)
		}
		if decodeErr != nil {
			return classifyJSONDecodeError(decodeErr)
		}
	case "application/xml", "text/xml":
		bodyLen, readErr, decodeErr := c.readAndCacheBody(func(r io.Reader) error {
			return xml.NewDecoder(r).Decode(v)
		})
		if readErr != nil {
			return wrapBindError("body", readErr)
		}
		if bodyLen == 0 {
			return wrapBindError("body", errEmptyBody)
		}
		if decodeErr != nil {
			return wrapBindError("body", decodeErr)
		}
	case "application/x-www-form-urlencoded", "multipart/form-data":
		return bindForm(c, v)
	case "text/plain":
		return bindPlainTextBody(c, v, true)
	default:
		return wrapBindError("body", fmt.Errorf("unsupported content type: %s", mediaType))
	}
	// Put the parameter fields back before validating, not after.
	if snap != nil {
		snap.restore()
	}
	return c.validateWith(v, plan)
}

// bindQuery binds query values, then validates v.
func bindQuery(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if plan.hasDefaults {
		applyDefaults(val, plan.queryFields)
	}
	if err := bindFieldsFromValues(val, plan.queryFields, c.QueryValues()); err != nil {
		return wrapBindError("query", err)
	}
	return c.validateWith(v, plan)
}

// bindForm binds URL-encoded or multipart form values, then validates v.
func bindForm(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}

	req := c.Request()
	if req == nil {
		return wrapBindError("form", errors.New("request is nil"))
	}
	if err := c.limitFormBody(); err != nil {
		return wrapBindError("form", err)
	}
	if plan.hasDefaults {
		applyDefaults(val, plan.formFields)
	}
	if requestMediaType(c.Header(HeaderContentType)) == "multipart/form-data" {
		if err := bindMultipartForm(val, plan, req); err != nil {
			return wrapBindError("form", err)
		}
		return c.validateWith(v, plan)
	}

	if err := req.ParseForm(); err != nil {
		return wrapBindError("form", fmt.Errorf("parse form: %w", err))
	}
	if err := bindFieldsFromValues(val, plan.formFields, req.Form); err != nil {
		return wrapBindError("form", err)
	}
	return c.validateWith(v, plan)
}

// bindHeader binds request headers, then validates v.
func bindHeader(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if plan.hasDefaults {
		applyDefaults(val, plan.headerFields)
	}
	if err := bindFieldsFromHeader(val, plan.headerFields, c.Request().Header); err != nil {
		return wrapBindError("header", err)
	}
	return c.validateWith(v, plan)
}

// bindCookie binds request cookies, then validates v.
func bindCookie(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if plan.hasDefaults {
		applyDefaults(val, plan.cookieFields)
	}
	if err := bindFieldsFromCookies(val, plan.cookieFields, c.Request()); err != nil {
		return wrapBindError("cookie", err)
	}
	return c.validateWith(v, plan)
}

// bindPath binds route parameters, then validates v.
func bindPath(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if err := bindFieldsFromPath(val, plan.pathFields, c); err != nil {
		return wrapBindError("path", err)
	}
	if c.app == nil {
		return nil
	}
	return c.validate(v, plan.rules)
}

// Bind returns the request binder facade for this context.
func (c *Context) Bind() *Bind {
	return &Bind{c: c}
}

// All binds the body, then query values, then path parameters, and validates
// v. Later sources win, so a path or query value is never replaced by a body
// key that matches the same field.
func (b *Bind) All(v any) error {
	return bindAll(b.c, v)
}

// Body binds the request body according to its Content-Type.
func (b *Bind) Body(v any) error {
	return bindBody(b.c, v)
}

// JSON decodes and validates a non-empty JSON request body.
func (b *Bind) JSON(v any) error {
	if b == nil || b.c == nil {
		return errors.New("context is nil")
	}
	snap, plan := snapshotParams(b.c, v)
	bodyLen, readErr, decodeErr := b.c.readAndCacheJSONBody(v)
	if snap != nil {
		snap.restore()
	}
	if readErr != nil {
		return wrapBindError("body", readErr)
	}
	if bodyLen == 0 {
		return wrapBindError("body", errEmptyBody)
	}
	if decodeErr != nil {
		return classifyJSONDecodeError(decodeErr)
	}
	return b.c.validateWith(v, plan)
}

// Text decodes and validates a non-empty plain-text request body.
func (b *Bind) Text(v any) error {
	if b == nil || b.c == nil {
		return errors.New("context is nil")
	}
	return bindPlainTextBody(b.c, v, true)
}

// XML decodes and validates a non-empty XML request body.
func (b *Bind) XML(v any) error {
	if decode := b.c.decoderFor("application/xml"); decode != nil {
		return decodeBody(b.c, decode, v, true)
	}
	snap, plan := snapshotParams(b.c, v)
	bodyLen, readErr, decodeErr := b.c.readAndCacheBody(func(r io.Reader) error {
		return xml.NewDecoder(r).Decode(v)
	})
	if snap != nil {
		snap.restore()
	}
	if readErr != nil {
		return wrapBindError("body", readErr)
	}
	if bodyLen == 0 {
		return wrapBindError("body", errEmptyBody)
	}
	if decodeErr != nil {
		return wrapBindError("body", decodeErr)
	}
	return b.c.validateWith(v, plan)
}

// Form binds URL-encoded or multipart form values, then validates v.
func (b *Bind) Form(v any) error {
	return bindForm(b.c, v)
}

// Query binds query values, then validates v.
func (b *Bind) Query(v any) error {
	return bindQuery(b.c, v)
}

// Header binds request headers, then validates v.
func (b *Bind) Header(v any) error {
	return bindHeader(b.c, v)
}

// Cookie binds request cookies into fields tagged cookie, then validates v.
func (b *Bind) Cookie(v any) error {
	return bindCookie(b.c, v)
}

// Path binds route parameters, then validates v.
func (b *Bind) Path(v any) error {
	return bindPath(b.c, v)
}

// Validate checks v, a pointer to a struct, as binding does. Enum values,
// from an enum tag or an EnumProvider type, are always checked. Then the
// configured Validator runs, or, when there's none, Zinc checks v's validate
// tags with BuiltinRules. A failure is a *ValidationError, which the default
// error handler answers with 422, unless the validator's error already
// carries a status.
func (c *Context) Validate(v any) error {
	if c.app == nil {
		return nil
	}
	return c.validate(v, rulePlanOf(v))
}

// validate is Validate with v's rule plan already found.
func (c *Context) validate(v any, plan *rulePlan) error {
	custom := c.app.config.Validator
	if plan != nil {
		if custom == nil && len(plan.unsupported) > 0 {
			return fmt.Errorf("zinc: validate rules Zinc doesn't enforce: %s; set Config.Validator to a validator that does", strings.Join(plan.unsupported, ", "))
		}
		if errs := plan.check(reflect.ValueOf(v).Elem(), custom == nil, "", nil); errs != nil {
			return &ValidationError{Err: errs}
		}
	}
	if custom == nil {
		return nil
	}
	err := custom.Validate(v)
	if err == nil {
		return nil
	}
	if coder, _ := findStatusCoder(err); coder != nil {
		return err
	}
	return &ValidationError{Err: err}
}

// validateWith validates v whose binding plan, which may be nil, is known,
// so the rule plan needs no lookup.
func (c *Context) validateWith(v any, plan *bindingPlan) error {
	if c.app == nil {
		return nil
	}
	var rules *rulePlan
	if plan != nil {
		rules = plan.rules
	}
	return c.validate(v, rules)
}

// rulePlanOf returns the rule plan for v, a pointer to a struct, or nil.
func rulePlanOf(v any) *rulePlan {
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct || reflect.ValueOf(v).IsNil() {
		return nil
	}
	return rulePlanFor(t.Elem())
}

func setFieldValue(value reflect.Value, inputs []string) error {
	return compileFieldSetter(value.Type()).set(value, inputs)
}

func bindPlainTextBody(c *Context, v any, requireBody bool) error {
	if c == nil {
		return errors.New("context is nil")
	}

	body, readErr := c.readAndCacheBodyBytes()
	if readErr != nil {
		return wrapBindError("body", readErr)
	}
	if len(body) == 0 {
		if requireBody {
			return wrapBindError("body", errEmptyBody)
		}
		return c.Validate(v)
	}
	if err := decodeTextBody(v, body); err != nil {
		return wrapBindError("body", err)
	}
	return c.Validate(v)
}

func readRequiredBody(c *Context, requireBody bool) ([]byte, error) {
	if c == nil {
		return nil, errors.New("context is nil")
	}
	body, readErr := c.readAndCacheBodyBytes()
	if readErr != nil {
		return nil, readErr
	}
	if len(body) == 0 && requireBody {
		return nil, errEmptyBody
	}
	return body, nil
}

func decodeTextBody(v any, body []byte) error {
	if v == nil {
		return errors.New("binding target must not be nil")
	}
	if unmarshaler, ok := v.(encoding.TextUnmarshaler); ok {
		return unmarshaler.UnmarshalText(body)
	}

	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return errors.New("binding target must be a pointer")
	}

	elem := val.Elem()
	if elem.Kind() == reflect.Slice && elem.Type().Elem().Kind() == reflect.Uint8 {
		copied := append([]byte(nil), body...)
		elem.Set(reflect.ValueOf(copied).Convert(elem.Type()))
		return nil
	}
	if elem.Kind() == reflect.String {
		elem.SetString(string(body))
		return nil
	}

	return setFieldValue(elem, []string{string(body)})
}

func bindMultipartForm(val reflect.Value, plan *bindingPlan, req *http.Request) error {
	if err := req.ParseMultipartForm(defaultMultipartMemory); err != nil {
		return fmt.Errorf("parse multipart form: %w", err)
	}
	form := req.MultipartForm
	if form == nil {
		return nil
	}
	if err := bindFieldsFromValues(val, plan.formFields, form.Value); err != nil {
		return err
	}
	if err := bindFieldsFromMultipartFiles(val, plan.multipartFileFields, form.File); err != nil {
		return err
	}
	return nil
}

func wrapBindError(source string, err error) error {
	if err == nil {
		return nil
	}
	var invalid *json.InvalidUnmarshalError
	if errors.As(err, &invalid) {
		return err
	}
	var bindErr *BindError
	if errors.As(err, &bindErr) {
		return err
	}
	bindErr = &BindError{Source: source, Err: err}
	var fieldErr *bindFieldError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &fieldErr):
		bindErr.Field, bindErr.Name, bindErr.Reason = fieldErr.Field, fieldErr.Name, fieldErr.Reason
	case errors.As(err, &typeErr):
		bindErr.Field, bindErr.Name, bindErr.Reason = typeErr.Field, typeErr.Field, jsonTypeReason(typeErr.Type)
	}
	return bindErr
}

// jsonTypeReason describes the JSON value a Go type expects.
func jsonTypeReason(typ reflect.Type) string {
	if typ == nil {
		return ""
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.String:
		return "must be a string"
	case reflect.Bool:
		return "must be a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "must be an integer"
	case reflect.Float32, reflect.Float64:
		return "must be a number"
	case reflect.Slice, reflect.Array:
		return "must be an array"
	case reflect.Struct, reflect.Map:
		return "must be an object"
	default:
		return ""
	}
}

// classifyJSONDecodeError turns a JSON decoding failure into a *BindError
// without exposing decoder details to clients. An error that carries its own
// status, such as one from a custom UnmarshalJSON method, keeps it.
func classifyJSONDecodeError(err error) error {
	// The default codec returns these concrete errors directly. Avoid walking
	// the error chain three times on the common malformed-request path while
	// keeping the wrapped-error fallback for custom codecs.
	switch typed := err.(type) {
	case *json.SyntaxError:
		return &BindError{Source: "body", Err: err}
	case *json.UnmarshalTypeError:
		return &BindError{Source: "body", Field: typed.Field, Name: typed.Field, Reason: jsonTypeReason(typed.Type), Err: err}
	}
	var syntax *json.SyntaxError
	var mismatch *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &mismatch) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return wrapBindError("body", err)
	}
	if coder, _ := findStatusCoder(err); coder != nil {
		return err
	}
	return wrapBindError("body", err)
}
