// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"net/http"
)

// ErrorHandler handles an error returned from a Zinc handler.
type ErrorHandler func(*Context, error)

// HTTPError represents an HTTP error with a status code and optional message.
type HTTPError struct {
	Code    int
	Message string
	Cause   error
	Meta    Map
	Headers http.Header
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Code)
}

// Unwrap exposes the underlying cause for errors.Is and errors.As.
func (e *HTTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewError creates an HTTPError for status code.
func NewError(code int) *HTTPError {
	return &HTTPError{Code: code}
}

// WithMessage returns a copy with a client-facing message.
func (e *HTTPError) WithMessage(message string) *HTTPError {
	return e.cloneWith(func(err *HTTPError) {
		err.Message = message
	})
}

// WithCause returns a copy that wraps cause.
func (e *HTTPError) WithCause(cause error) *HTTPError {
	return e.cloneWith(func(err *HTTPError) {
		err.Cause = cause
	})
}

// WithMeta returns a copy with one metadata value. Metadata is available to
// custom error handlers and is not emitted by the default handler.
func (e *HTTPError) WithMeta(key string, value any) *HTTPError {
	return e.cloneWith(func(err *HTTPError) {
		if err.Meta == nil {
			err.Meta = Map{}
		}
		err.Meta[key] = value
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
	if len(e.Meta) > 0 {
		clone.Meta = make(Map, len(e.Meta))
		for key, value := range e.Meta {
			clone.Meta[key] = value
		}
	}
	if len(e.Headers) > 0 {
		clone.Headers = e.Headers.Clone()
	}
	if apply != nil {
		apply(clone)
	}
	return clone
}

func defaultErrorHandler(c *Context, err error) {
	if c == nil || err == nil || c.written {
		return
	}

	if httpErr, ok := err.(*HTTPError); ok {
		writeHTTPError(c, httpErr)
		return
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		writeHTTPError(c, httpErr)
		return
	}

	_ = c.Status(http.StatusInternalServerError).String(http.StatusText(http.StatusInternalServerError))
}

func writeHTTPError(c *Context, httpErr *HTTPError) {
	if c == nil || httpErr == nil {
		return
	}
	for key, values := range httpErr.Headers {
		for _, value := range values {
			c.AppendHeader(key, value)
		}
	}
	_ = c.Status(httpErr.Code).String(httpErr.Error())
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
