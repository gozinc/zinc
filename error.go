// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"io"
	"maps"
	"net/http"
	"strconv"
	"unicode/utf8"
)

// ErrorHandler handles an error returned from a Zinc handler.
type ErrorHandler func(*Context, error)

// StatusCoder is implemented by errors that choose their own HTTP status, so
// a handler can return a domain error and let the error handler map it:
//
//	func (ErrUserNotFound) StatusCode() int { return http.StatusNotFound }
//
// For statuses below 500, the error's own message is sent to the client.
type StatusCoder interface {
	StatusCode() int
}

// HTTPError represents an HTTP error with a status code and optional message.
type HTTPError struct {
	Code    int
	Message string
	Cause   error
	// Details are written to the response body by the default error handler.
	Details Map
	Headers http.Header
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Code)
}

// StatusCode implements StatusCoder.
func (e *HTTPError) StatusCode() int {
	return e.Code
}

// Unwrap exposes the underlying cause for errors.Is and errors.As.
func (e *HTTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is matches HTTP errors by status, so errors.Is(err, ErrNotFound) holds for
// any 404, including copies made by the With methods. A target that carries a
// message matches only errors with the same status and message.
func (e *HTTPError) Is(target error) bool {
	t, ok := target.(*HTTPError)
	if !ok || e == nil || t == nil {
		return false
	}
	return e.Code == t.Code && (t.Message == "" || t.Message == e.Message)
}

// NewError creates an HTTPError for code, with an optional client-facing
// message. Without one, the status text is used.
func NewError(code int, message ...string) *HTTPError {
	err := &HTTPError{Code: code}
	if len(message) > 0 {
		err.Message = message[0]
	}
	return err
}

// BadRequest returns a 400 error with a client-facing message.
func BadRequest(message string) *HTTPError { return NewError(StatusBadRequest, message) }

// Unauthorized returns a 401 error with a client-facing message.
func Unauthorized(message string) *HTTPError { return NewError(StatusUnauthorized, message) }

// Forbidden returns a 403 error with a client-facing message.
func Forbidden(message string) *HTTPError { return NewError(StatusForbidden, message) }

// NotFound returns a 404 error with a client-facing message.
func NotFound(message string) *HTTPError { return NewError(StatusNotFound, message) }

// Conflict returns a 409 error with a client-facing message.
func Conflict(message string) *HTTPError { return NewError(StatusConflict, message) }

// Gone returns a 410 error with a client-facing message.
func Gone(message string) *HTTPError { return NewError(StatusGone, message) }

// UnprocessableEntity returns a 422 error with a client-facing message.
func UnprocessableEntity(message string) *HTTPError {
	return NewError(StatusUnprocessableEntity, message)
}

// TooManyRequests returns a 429 error with a client-facing message.
func TooManyRequests(message string) *HTTPError { return NewError(StatusTooManyRequests, message) }

// InternalServerError returns a 500 error with a client-facing message.
func InternalServerError(message string) *HTTPError {
	return NewError(StatusInternalServerError, message)
}

// ServiceUnavailable returns a 503 error with a client-facing message.
func ServiceUnavailable(message string) *HTTPError {
	return NewError(StatusServiceUnavailable, message)
}

// Wrap returns a copy that wraps cause. The cause is for logs and errors.Is;
// it is never sent to the client.
func (e *HTTPError) Wrap(cause error) *HTTPError {
	return e.cloneWith(func(err *HTTPError) {
		err.Cause = cause
	})
}

// WithDetail returns a copy with one detail, written to the response body by
// the default error handler.
func (e *HTTPError) WithDetail(key string, value any) *HTTPError {
	return e.cloneWith(func(err *HTTPError) {
		if err.Details == nil {
			err.Details = Map{}
		}
		err.Details[key] = value
	})
}

// WithHeader returns a copy that adds a response header.
func (e *HTTPError) WithHeader(key, value string) *HTTPError {
	return e.cloneWith(func(err *HTTPError) {
		if err.Headers == nil {
			err.Headers = make(http.Header)
		}
		err.Headers.Add(key, value)
	})
}

// cloneWith keeps package-level HTTP errors immutable and safe to reuse. Maps
// and headers are copied before mutation so derived errors cannot share state.
func (e *HTTPError) cloneWith(apply func(*HTTPError)) *HTTPError {
	if e == nil {
		return nil
	}
	clone := &HTTPError{
		Code:    e.Code,
		Message: e.Message,
		Cause:   e.Cause,
	}
	if len(e.Details) > 0 {
		clone.Details = maps.Clone(e.Details)
	}
	if len(e.Headers) > 0 {
		clone.Headers = e.Headers.Clone()
	}
	if apply != nil {
		apply(clone)
	}
	return clone
}

// ValidationError wraps a failure returned by the configured Validator. The
// default error handler answers it with 422, adding the field messages when
// the validator's error has a Fields() map[string]string method.
type ValidationError struct {
	Err error
}

func (e *ValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "validation failed"
	}
	return "validation failed: " + e.Err.Error()
}

// Unwrap exposes the validator's error.
func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// StatusCode implements StatusCoder.
func (*ValidationError) StatusCode() int {
	return StatusUnprocessableEntity
}

// Fields returns per-field messages from the validator's error, or nil.
func (e *ValidationError) Fields() map[string]string {
	if e == nil {
		return nil
	}
	var fields map[string]string
	walkErrors(e.Err, func(err error) bool {
		if f, ok := err.(interface{ Fields() map[string]string }); ok {
			fields = f.Fields()
			return true
		}
		return false
	})
	return fields
}

// walkErrors calls visit for err and each error it wraps, depth first, until
// visit returns true. Unlike errors.As it takes no pointer target, so looking
// through a chain does not allocate.
func walkErrors(err error, visit func(error) bool) bool {
	for err != nil {
		if visit(err) {
			return true
		}
		switch e := err.(type) {
		case interface{ Unwrap() error }:
			err = e.Unwrap()
		case interface{ Unwrap() []error }:
			for _, inner := range e.Unwrap() {
				if walkErrors(inner, visit) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
	return false
}

// findHTTPError returns the first *HTTPError in err's chain. Errors with an As
// method are handed to errors.As so their custom matching still applies.
func findHTTPError(err error) *HTTPError {
	var found *HTTPError
	walkErrors(err, func(e error) bool {
		if h, ok := e.(*HTTPError); ok {
			found = h
			return true
		}
		if _, ok := e.(interface{ As(any) bool }); ok {
			var target *HTTPError
			if errors.As(e, &target) {
				found = target
				return true
			}
		}
		return false
	})
	return found
}

// findStatusCoder returns the first error in err's chain that chooses a status.
func findStatusCoder(err error) (StatusCoder, error) {
	var coder StatusCoder
	var coderErr error
	walkErrors(err, func(e error) bool {
		if c, ok := e.(StatusCoder); ok {
			coder, coderErr = c, e
			return true
		}
		return false
	})
	return coder, coderErr
}

// StatusCode reports the HTTP status the default error handler sends for err:
// the code of an HTTPError anywhere in its chain, otherwise the status of the
// first StatusCoder, otherwise 500. It returns 0 for a nil error.
func StatusCode(err error) int {
	status, _ := resolveError(err)
	return status
}

// resolveError finds the status for err and the error whose message may be
// shown to the client. An explicit HTTPError wins over other StatusCoders, so
// a BindError caused by an oversized body still reports 413.
func resolveError(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr.Code, httpErr
	}
	if httpErr := findHTTPError(err); httpErr != nil {
		return httpErr.Code, httpErr
	}
	if coder, coderErr := findStatusCoder(err); coder != nil {
		if status := coder.StatusCode(); status >= 400 && status <= 599 {
			return status, coderErr
		}
	}
	return StatusInternalServerError, nil
}

// DefaultErrorHandler writes err as a JSON error body, newline-terminated
// like every Context.JSON response:
//
//	{"error":{"status":404,"message":"user not found"}}
//
// It adds "fields" for binding and validation errors and "details" for
// HTTPError details. Messages from HTTPError and from 4xx StatusCoders are
// sent as written; any other error is reported only by its status text, so
// internal error text never reaches the client.
func DefaultErrorHandler(c *Context, err error) {
	if c == nil || err == nil || c.written {
		return
	}
	resetErrorRepresentation(c)
	status, public := resolveError(err)

	message := http.StatusText(status)
	var fields map[string]string
	var details Map
	switch e := public.(type) {
	case *HTTPError:
		for key, values := range e.Headers {
			for _, value := range values {
				c.AppendHeader(key, value)
			}
		}
		message = e.Error()
		details = e.Details
	case *BindError:
		message, fields = e.clientMessage()
	case *ValidationError:
		message, fields = "validation failed", e.Fields()
	case nil:
	default:
		if status < 500 {
			message = e.Error()
		}
	}
	_ = c.writeErrorJSON(status, message, fields, details)
}

// TextErrors is an ErrorHandler that writes plain-text error bodies, as Zinc
// 0.3 did. Status resolution and message safety match DefaultErrorHandler.
func TextErrors(c *Context, err error) {
	if c == nil || err == nil || c.written {
		return
	}
	resetErrorRepresentation(c)
	status, public := resolveError(err)
	message := http.StatusText(status)
	switch e := public.(type) {
	case *HTTPError:
		for key, values := range e.Headers {
			for _, value := range values {
				c.AppendHeader(key, value)
			}
		}
		message = e.Error()
	case *BindError, *ValidationError, nil:
	default:
		if status < 500 {
			message = e.Error()
		}
	}
	_ = c.Status(status).String(message)
}

// resetErrorRepresentation discards representation headers prepared by a
// helper that failed before the response was committed.
func resetErrorRepresentation(c *Context) {
	header := c.Writer().Header()
	header.Del(HeaderContentType)
	header.Del(HeaderContentLength)
	header.Del(HeaderContentDisposition)
}

type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Status  int               `json:"status"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Details Map               `json:"details,omitempty"`
}

// Error envelopes for 4xx and 5xx statuses are precomputed: the whole body
// when the message is the status text, and the prefix up to the message
// otherwise. Writing an error then takes one or three writes and no encoding.
const errorBodySuffix = "\"}}\n"

var errorEnvelopes = func() (e [200]struct{ full, prefix []byte }) {
	for i := range e {
		status := 400 + i
		prefix := `{"error":{"status":` + strconv.Itoa(status) + `,"message":"`
		e[i].prefix = []byte(prefix)
		if text := http.StatusText(status); text != "" && !jsonNeedsEscape(text) {
			e[i].full = []byte(prefix + text + errorBodySuffix)
		}
	}
	return e
}()

// writeErrorJSON writes the error envelope. The common case, a status and a
// message, is written directly without encoding or allocating; fields and
// details go through the configured JSON codec.
func (c *Context) writeErrorJSON(status int, message string, fields map[string]string, details Map) error {
	if len(fields) > 0 || len(details) > 0 {
		return c.Status(status).JSON(errorBody{Error: errorPayload{
			Status: status, Message: message, Fields: fields, Details: details,
		}})
	}
	c.Status(status)
	if c.baseWriter && !c.written && c.request != nil && c.request.Method != http.MethodHead &&
		status >= 400 && status <= 599 && !jsonNeedsEscape(message) {
		// As in String: the pooled base writer is written directly, without
		// interface dispatch. SetWriter, HEAD, and escaping use the general path.
		writer := &c.response.base
		header := writer.Header()
		if len(header[contentType]) == 0 {
			header[contentType] = []string{jsonType}
		}
		envelope := &errorEnvelopes[status-400]
		if envelope.full != nil && message == http.StatusText(status) {
			_, err := writer.Write(envelope.full)
			return err
		}
		if _, err := writer.Write(envelope.prefix); err != nil {
			return err
		}
		if _, err := writer.WriteString(message); err != nil {
			return err
		}
		_, err := writer.WriteString(errorBodySuffix)
		return err
	}
	writer, writeBody, err := c.prepareResponse(jsonType)
	if err != nil || !writeBody {
		return err
	}
	return writeErrorEnvelope(writer, status, message)
}

// writeErrorEnvelope writes {"error":{"status":N,"message":"..."}} to w.
func writeErrorEnvelope(w io.Writer, status int, message string) error {
	if status < 400 || status > 599 || jsonNeedsEscape(message) {
		return writeErrorEnvelopeSlow(w, status, message)
	}
	envelope := &errorEnvelopes[status-400]
	if envelope.full != nil && message == http.StatusText(status) {
		_, err := w.Write(envelope.full)
		return err
	}
	if _, err := w.Write(envelope.prefix); err != nil {
		return err
	}
	if _, err := io.WriteString(w, message); err != nil {
		return err
	}
	_, err := io.WriteString(w, errorBodySuffix)
	return err
}

// writeErrorEnvelopeSlow handles escaping and statuses outside 4xx and 5xx.
func writeErrorEnvelopeSlow(w io.Writer, status int, message string) error {
	if _, err := io.WriteString(w, `{"error":{"status":`+strconv.Itoa(status)+`,"message":`); err != nil {
		return err
	}
	if err := writeJSONString(w, message); err != nil {
		return err
	}
	_, err := io.WriteString(w, "}}\n")
	return err
}

// writeJSONString writes s as a JSON string. Strings that need no escaping,
// which includes every status text, are written without copying.
func writeJSONString(w io.Writer, s string) error {
	if !jsonNeedsEscape(s) {
		if _, err := io.WriteString(w, `"`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, s); err != nil {
			return err
		}
		_, err := io.WriteString(w, `"`)
		return err
	}
	buf := make([]byte, 0, len(s)+8)
	buf = append(buf, '"')
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			switch {
			case b == '"' || b == '\\':
				buf = append(buf, '\\', b)
			case b == '\n':
				buf = append(buf, '\\', 'n')
			case b == '\r':
				buf = append(buf, '\\', 'r')
			case b == '\t':
				buf = append(buf, '\\', 't')
			case b < 0x20 || b == '<' || b == '>' || b == '&':
				buf = append(buf, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xF])
			default:
				buf = append(buf, b)
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			buf = append(buf, `�`...)
		case r == ' ' || r == ' ':
			buf = append(buf, '\\', 'u', '2', '0', '2', hexDigits[r&0xF])
		default:
			buf = append(buf, s[i:i+size]...)
		}
		i += size
	}
	buf = append(buf, '"')
	_, err := w.Write(buf)
	return err
}

const hexDigits = "0123456789abcdef"

// jsonNeedsEscape matches encoding/json's default escaping, including the HTML
// characters and line separators it escapes for safe embedding.
func jsonNeedsEscape(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < 0x20 || b == '"' || b == '\\' || b == '<' || b == '>' || b == '&' || b >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

// Reusable HTTP errors. Modifier methods return independent copies and never
// mutate these package-level values.
var (
	ErrBadRequest                    = NewError(StatusBadRequest)
	ErrUnauthorized                  = NewError(StatusUnauthorized)
	ErrPaymentRequired               = NewError(StatusPaymentRequired)
	ErrForbidden                     = NewError(StatusForbidden)
	ErrNotFound                      = NewError(StatusNotFound)
	ErrMethodNotAllowed              = NewError(StatusMethodNotAllowed)
	ErrNotAcceptable                 = NewError(StatusNotAcceptable)
	ErrProxyAuthRequired             = NewError(StatusProxyAuthRequired)
	ErrRequestTimeout                = NewError(StatusRequestTimeout)
	ErrConflict                      = NewError(StatusConflict)
	ErrGone                          = NewError(StatusGone)
	ErrLengthRequired                = NewError(StatusLengthRequired)
	ErrPreconditionFailed            = NewError(StatusPreconditionFailed)
	ErrRequestEntityTooLarge         = NewError(StatusRequestEntityTooLarge)
	ErrRequestURITooLong             = NewError(StatusRequestURITooLong)
	ErrUnsupportedMediaType          = NewError(StatusUnsupportedMediaType)
	ErrRequestedRangeNotSatisfiable  = NewError(StatusRequestedRangeNotSatisfiable)
	ErrExpectationFailed             = NewError(StatusExpectationFailed)
	ErrTeapot                        = NewError(StatusTeapot)
	ErrMisdirectedRequest            = NewError(StatusMisdirectedRequest)
	ErrUnprocessableEntity           = NewError(StatusUnprocessableEntity)
	ErrLocked                        = NewError(StatusLocked)
	ErrFailedDependency              = NewError(StatusFailedDependency)
	ErrTooEarly                      = NewError(StatusTooEarly)
	ErrUpgradeRequired               = NewError(StatusUpgradeRequired)
	ErrPreconditionRequired          = NewError(StatusPreconditionRequired)
	ErrTooManyRequests               = NewError(StatusTooManyRequests)
	ErrRequestHeaderFieldsTooLarge   = NewError(StatusRequestHeaderFieldsTooLarge)
	ErrUnavailableForLegalReasons    = NewError(StatusUnavailableForLegalReasons)
	ErrInternalServerError           = NewError(StatusInternalServerError)
	ErrNotImplemented                = NewError(StatusNotImplemented)
	ErrBadGateway                    = NewError(StatusBadGateway)
	ErrServiceUnavailable            = NewError(StatusServiceUnavailable)
	ErrGatewayTimeout                = NewError(StatusGatewayTimeout)
	ErrHTTPVersionNotSupported       = NewError(StatusHTTPVersionNotSupported)
	ErrVariantAlsoNegotiates         = NewError(StatusVariantAlsoNegotiates)
	ErrInsufficientStorage           = NewError(StatusInsufficientStorage)
	ErrLoopDetected                  = NewError(StatusLoopDetected)
	ErrNotExtended                   = NewError(StatusNotExtended)
	ErrNetworkAuthenticationRequired = NewError(StatusNetworkAuthenticationRequired)
)
