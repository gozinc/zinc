// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// RequestBinder controls how request data is mapped into application structs.
type RequestBinder interface {
	Bind(*Context, any) error
	BindBody(*Context, any) error
	BindQuery(*Context, any) error
	BindForm(*Context, any) error
	BindHeader(*Context, any) error
	BindPath(*Context, any) error
}

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
	// Source is "path", "query", "header", "form", or "body".
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

// JSONCodec allows applications to replace Zinc's JSON encoder and decoder.
type JSONCodec interface {
	Encode(w io.Writer, v any, indent string) error
	Decode(r io.Reader, v any) error
}

type defaultBinder struct {
	codec JSONCodec
}

type jsonBytesDecoder interface {
	DecodeBytes(body []byte, v any) error
}

func (b defaultBinder) Bind(c *Context, v any) error {
	// General binding is intentionally deterministic: path values are applied
	// first, then query values, then the body. Later sources may overwrite
	// earlier fields before validation runs once at the end.
	mediaType := requestMediaType(c.Header(HeaderContentType))
	if mediaType == "text/plain" {
		return bindPlainTextBody(c, v, false)
	}
	// Scalar/map YAML and TOML remain body-only. Struct targets consistently
	// merge path, query, then body just like JSON/XML.
	typ := reflect.TypeOf(v)
	structTarget := typ != nil && typ.Kind() == reflect.Pointer && typ.Elem().Kind() == reflect.Struct
	if !structTarget {
		if isYAMLMediaType(mediaType) {
			return bindYAMLBody(c, v, false)
		}
		if isTOMLMediaType(mediaType) {
			return bindTOMLBody(c, v, false)
		}
	}

	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if err := bindFieldsFromPath(val, plan.pathFields, c); err != nil {
		return wrapBindError("path", err)
	}
	req := c.Request()
	if len(plan.queryFields) > 0 && req != nil && req.URL != nil && req.URL.RawQuery != "" {
		if err := bindFieldsFromQuery(val, plan.queryFields, c); err != nil {
			return wrapBindError("query", err)
		}
	}
	if req == nil || req.Body == nil {
		return c.Validate(v)
	}
	if isYAMLMediaType(mediaType) {
		return bindYAMLBody(c, v, false)
	}
	if isTOMLMediaType(mediaType) {
		return bindTOMLBody(c, v, false)
	}

	switch mediaType {
	case "", "application/json":
		bodyLen, readErr, decodeErr := c.readAndCacheJSONBody(b.codec, v)
		if readErr != nil {
			return wrapBindError("body", readErr)
		}
		if bodyLen == 0 {
			return c.Validate(v)
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
			return c.Validate(v)
		}
		if decodeErr != nil {
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
	case "text/plain":
		return bindPlainTextBody(c, v, false)
	default:
		return wrapBindError("body", fmt.Errorf("unsupported content type: %s", mediaType))
	}
	return c.Validate(v)
}

func (b defaultBinder) BindBody(c *Context, v any) error {
	mediaType := requestMediaType(c.Header(HeaderContentType))
	switch mediaType {
	case "", "application/json":
		bodyLen, readErr, decodeErr := c.readAndCacheJSONBody(b.codec, v)
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
		return b.BindForm(c, v)
	case "text/plain":
		return bindPlainTextBody(c, v, true)
	default:
		if isYAMLMediaType(mediaType) {
			return bindYAMLBody(c, v, true)
		}
		if isTOMLMediaType(mediaType) {
			return bindTOMLBody(c, v, true)
		}
		return wrapBindError("body", fmt.Errorf("unsupported content type: %s", mediaType))
	}
	return c.Validate(v)
}

func (b defaultBinder) BindQuery(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if err := bindFieldsFromValues(val, plan.queryFields, c.QueryValues()); err != nil {
		return wrapBindError("query", err)
	}
	return c.Validate(v)
}

func (b defaultBinder) BindForm(c *Context, v any) error {
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
	if requestMediaType(c.Header(HeaderContentType)) == "multipart/form-data" {
		if err := bindMultipartForm(val, plan, req); err != nil {
			return wrapBindError("form", err)
		}
		return c.Validate(v)
	}

	if err := req.ParseForm(); err != nil {
		return wrapBindError("form", fmt.Errorf("parse form: %w", err))
	}
	if err := bindFieldsFromValues(val, plan.formFields, req.Form); err != nil {
		return wrapBindError("form", err)
	}
	return c.Validate(v)
}

func (b defaultBinder) BindHeader(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if err := bindFieldsFromHeader(val, plan.headerFields, c.Request().Header); err != nil {
		return wrapBindError("header", err)
	}
	return c.Validate(v)
}

func (b defaultBinder) BindPath(c *Context, v any) error {
	val, plan, err := bindTargetPlan(v)
	if err != nil {
		return err
	}
	if err := bindFieldsFromPath(val, plan.pathFields, c); err != nil {
		return wrapBindError("path", err)
	}
	return c.Validate(v)
}

// Bind returns the request binder facade for this context.
func (c *Context) Bind() *Bind {
	return &Bind{c: c}
}

// All binds path, query, and body data, then validates v.
func (b *Bind) All(v any) error {
	return b.c.app.config.RequestBinder.Bind(b.c, v)
}

// Body binds the request body according to its Content-Type.
func (b *Bind) Body(v any) error {
	return b.c.app.config.RequestBinder.BindBody(b.c, v)
}

// JSON decodes and validates a non-empty JSON request body.
func (b *Bind) JSON(v any) error {
	if b == nil || b.c == nil {
		return errors.New("context is nil")
	}
	codec := b.c.app.config.JSONCodec
	bodyLen, readErr, decodeErr := b.c.readAndCacheJSONBody(codec, v)
	if readErr != nil {
		return wrapBindError("body", readErr)
	}
	if bodyLen == 0 {
		return wrapBindError("body", errEmptyBody)
	}
	if decodeErr != nil {
		return classifyJSONDecodeError(decodeErr)
	}
	return b.c.Validate(v)
}

// Text decodes and validates a non-empty plain-text request body.
func (b *Bind) Text(v any) error {
	if b == nil || b.c == nil {
		return errors.New("context is nil")
	}
	return bindPlainTextBody(b.c, v, true)
}

// YAML decodes and validates a non-empty YAML request body.
func (b *Bind) YAML(v any) error {
	if b == nil || b.c == nil {
		return errors.New("context is nil")
	}
	return bindYAMLBody(b.c, v, true)
}

// TOML decodes and validates a non-empty TOML request body.
func (b *Bind) TOML(v any) error {
	if b == nil || b.c == nil {
		return errors.New("context is nil")
	}
	return bindTOMLBody(b.c, v, true)
}

// XML decodes and validates a non-empty XML request body.
func (b *Bind) XML(v any) error {
	bodyLen, readErr, decodeErr := b.c.readAndCacheBody(func(r io.Reader) error {
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
	return b.c.Validate(v)
}

// Form binds URL-encoded or multipart form values, then validates v.
func (b *Bind) Form(v any) error {
	return b.c.app.config.RequestBinder.BindForm(b.c, v)
}

// Query binds query values, then validates v.
func (b *Bind) Query(v any) error {
	return b.c.app.config.RequestBinder.BindQuery(b.c, v)
}

// Header binds request headers, then validates v.
func (b *Bind) Header(v any) error {
	return b.c.app.config.RequestBinder.BindHeader(b.c, v)
}

// Path binds route parameters, then validates v.
func (b *Bind) Path(v any) error {
	return b.c.app.config.RequestBinder.BindPath(b.c, v)
}

// Validate invokes the configured Validator, or succeeds when none is set. A
// failure is returned as a *ValidationError, which the default error handler
// answers with 422, unless the validator's error already carries a status.
func (c *Context) Validate(v any) error {
	if c.app == nil || c.app.config.Validator == nil {
		return nil
	}
	err := c.app.config.Validator.Validate(v)
	if err == nil {
		return nil
	}
	if coder, _ := findStatusCoder(err); coder != nil {
		return err
	}
	return &ValidationError{Err: err}
}

func setFieldValue(value reflect.Value, inputs []string) error {
	return compileFieldSetter(value.Type()).set(value, inputs)
}

type defaultJSONCodec struct{}

func (defaultJSONCodec) Encode(w io.Writer, v any, indent string) error {
	enc := json.NewEncoder(w)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	return enc.Encode(v)
}

func (defaultJSONCodec) Decode(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	return dec.Decode(v)
}

func (defaultJSONCodec) DecodeBytes(body []byte, v any) error {
	return json.Unmarshal(body, v)
}

func decodeJSONBody(codec JSONCodec, body []byte, v any) error {
	if decoder, ok := codec.(jsonBytesDecoder); ok {
		return decoder.DecodeBytes(body, v)
	}
	return codec.Decode(bytes.NewReader(body), v)
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

func bindYAMLBody(c *Context, v any, requireBody bool) error {
	body, readErr := readRequiredBody(c, requireBody)
	if readErr != nil {
		return wrapBindError("body", readErr)
	}
	if len(body) == 0 {
		return c.Validate(v)
	}
	if err := yaml.Unmarshal(body, v); err != nil {
		return wrapBindError("body", err)
	}
	return c.Validate(v)
}

func bindTOMLBody(c *Context, v any, requireBody bool) error {
	body, readErr := readRequiredBody(c, requireBody)
	if readErr != nil {
		return wrapBindError("body", readErr)
	}
	if len(body) == 0 {
		return c.Validate(v)
	}
	if err := toml.Unmarshal(body, v); err != nil {
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

func isYAMLMediaType(mediaType string) bool {
	switch mediaType {
	case "application/x-yaml", "application/yaml", "text/yaml":
		return true
	default:
		return false
	}
}

func isTOMLMediaType(mediaType string) bool {
	switch mediaType {
	case "application/toml", "text/toml":
		return true
	default:
		return false
	}
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

// Opaque decoder errors may be application/codec failures. Classify known
// malformed JSON and type errors without exposing decoder details to clients.
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
	return err
}
