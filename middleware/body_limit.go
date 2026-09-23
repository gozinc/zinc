// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"errors"
	"fmt"
	"io"

	"github.com/0mjs/zinc"
)

const (
	// B, KB, MB, and GB are binary byte-size helpers.
	B  int64 = 1
	KB       = 1024 * B
	MB       = 1024 * KB
	GB       = 1024 * MB
)

// ErrBodyLimitExceeded identifies requests that exceed the configured limit.
var ErrBodyLimitExceeded = errors.New("zincbodylimit: request body exceeded configured limit")

// BodyLimitSource identifies whether rejection used metadata or observed bytes.
type BodyLimitSource string

const (
	BodyLimitSourceContentLength BodyLimitSource = "content_length"
	BodyLimitSourceBodyRead      BodyLimitSource = "body_read"
)

// BodyLimitError records the configured limit and observed request size.
type BodyLimitError struct {
	Limit    int64
	Observed int64
	Source   BodyLimitSource
}

func (e *BodyLimitError) Error() string {
	if e == nil {
		return ErrBodyLimitExceeded.Error()
	}
	switch e.Source {
	case BodyLimitSourceContentLength:
		return fmt.Sprintf("zincbodylimit: content length %d exceeds limit %d", e.Observed, e.Limit)
	case BodyLimitSourceBodyRead:
		return fmt.Sprintf("zincbodylimit: body read %d exceeds limit %d", e.Observed, e.Limit)
	default:
		return fmt.Sprintf("zincbodylimit: request body exceeds limit %d", e.Limit)
	}
}

func (e *BodyLimitError) Is(target error) bool {
	return target == ErrBodyLimitExceeded || target == zinc.ErrRequestEntityTooLarge
}

func (e *BodyLimitError) Unwrap() error {
	return zinc.ErrRequestEntityTooLarge
}

// BodyLimitConfig controls request-body size enforcement.
type BodyLimitConfig struct {
	Skipper func(*zinc.Context) bool
	Limit   int64
}

// BodyLimit enforces limit against request body bytes.
func BodyLimit(limit int64) zinc.Middleware {
	return BodyLimitWithConfig(BodyLimitConfig{Limit: limit})
}

// BodyLimitWithConfig rejects known oversized bodies early and wraps the body
// reader so chunked or dishonest requests cannot bypass enforcement.
func BodyLimitWithConfig(config BodyLimitConfig) zinc.Middleware {
	cfg := resolveBodyLimitConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		req := c.Request()
		if req == nil || req.Body == nil {
			return c.Next()
		}

		if req.ContentLength > cfg.Limit {
			return &BodyLimitError{
				Limit:    cfg.Limit,
				Observed: req.ContentLength,
				Source:   BodyLimitSourceContentLength,
			}
		}

		req.Body = &bodyLimitReadCloser{
			reader: req.Body,
			limit:  cfg.Limit,
		}

		return c.Next()
	}
}

func resolveBodyLimitConfig(config BodyLimitConfig) BodyLimitConfig {
	if config.Limit <= 0 {
		panic("zincbodylimit: Limit must be greater than zero")
	}
	return config
}

type bodyLimitReadCloser struct {
	reader io.ReadCloser
	limit  int64
	read   int64
}

func (r *bodyLimitReadCloser) Read(p []byte) (int, error) {
	if r.read >= r.limit {
		return 0, &BodyLimitError{
			Limit:    r.limit,
			Observed: r.read,
			Source:   BodyLimitSourceBodyRead,
		}
	}

	n, err := r.reader.Read(p)
	if n <= 0 {
		return n, err
	}

	remaining := r.limit - r.read
	if int64(n) <= remaining {
		r.read += int64(n)
		return n, err
	}

	r.read += int64(n)
	allowed := int(remaining)
	if allowed < 0 {
		allowed = 0
	}

	return allowed, &BodyLimitError{
		Limit:    r.limit,
		Observed: r.read,
		Source:   BodyLimitSourceBodyRead,
	}
}

func (r *bodyLimitReadCloser) Close() error {
	return r.reader.Close()
}
