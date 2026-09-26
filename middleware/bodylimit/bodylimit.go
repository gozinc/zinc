// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package bodylimit

import (
	"errors"
	"fmt"
	"io"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

const (
	// B, KB, MB, and GB are binary byte-size helpers.
	B  int64 = 1
	KB       = 1024 * B
	MB       = 1024 * KB
	GB       = 1024 * MB
)

// ErrExceeded identifies requests that exceed the configured limit.
var ErrExceeded = errors.New("bodylimit: request body exceeded configured limit")

// Source identifies whether rejection used metadata or observed bytes.
type Source string

const (
	SourceContentLength Source = "content_length"
	SourceBodyRead      Source = "body_read"
)

// Error records the configured limit and observed request size.
type Error struct {
	Limit    int64
	Observed int64
	Source   Source
}

func (e *Error) Error() string {
	if e == nil {
		return ErrExceeded.Error()
	}
	switch e.Source {
	case SourceContentLength:
		return fmt.Sprintf("bodylimit: content length %d exceeds limit %d", e.Observed, e.Limit)
	case SourceBodyRead:
		return fmt.Sprintf("bodylimit: body read %d exceeds limit %d", e.Observed, e.Limit)
	default:
		return fmt.Sprintf("bodylimit: request body exceeds limit %d", e.Limit)
	}
}

func (e *Error) Is(target error) bool {
	return target == ErrExceeded || target == zinc.ErrRequestEntityTooLarge
}

func (e *Error) Unwrap() error {
	return zinc.ErrRequestEntityTooLarge
}

// Config controls request-body size enforcement.
type Config struct {
	Limit int64
}

// New rejects known oversized bodies early and wraps the body reader so
// chunked or dishonest requests cannot bypass enforcement.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("bodylimit", configs)
	cfg := resolveBodyLimitConfig(config)

	return func(c *zinc.Context) error {
		req := c.Request()
		if req == nil || req.Body == nil {
			return c.Next()
		}

		if req.ContentLength > cfg.Limit {
			return &Error{
				Limit:    cfg.Limit,
				Observed: req.ContentLength,
				Source:   SourceContentLength,
			}
		}

		req.Body = &bodyLimitReadCloser{
			reader: req.Body,
			limit:  cfg.Limit,
		}

		return c.Next()
	}
}

func resolveBodyLimitConfig(config Config) Config {
	if config.Limit <= 0 {
		panic("bodylimit: Limit must be greater than zero")
	}
	return config
}

type bodyLimitReadCloser struct {
	reader io.ReadCloser
	limit  int64
	read   int64
}

func (r *bodyLimitReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	limitError := func() error {
		return &Error{Limit: r.limit, Observed: r.read, Source: SourceBodyRead}
	}
	if r.read > r.limit {
		return 0, limitError()
	}
	if r.read == r.limit {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			r.read += int64(n)
			return 0, limitError()
		}
		return 0, err
	}
	if remaining := r.limit - r.read; int64(len(p)) > remaining {
		p = p[:int(remaining)]
	}
	n, err := r.reader.Read(p)
	r.read += int64(n)
	return n, err
}

func (r *bodyLimitReadCloser) Close() error {
	return r.reader.Close()
}
