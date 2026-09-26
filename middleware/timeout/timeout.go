// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package timeout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// ErrExceeded identifies a downstream context deadline.
var ErrExceeded = errors.New("timeout: request context deadline exceeded")

// Info describes the request deadline installed by middleware.
type Info struct {
	Timeout  time.Duration
	Deadline time.Time
}

// Error preserves deadline metadata and the underlying error.
type Error struct {
	Info  Info
	Cause error
}

func (e *Error) Error() string {
	if e == nil {
		return ErrExceeded.Error()
	}
	if e.Info.Timeout > 0 {
		return fmt.Sprintf("timeout: request exceeded timeout %s", e.Info.Timeout)
	}
	return ErrExceeded.Error()
}

func (e *Error) Unwrap() error {
	if e == nil || e.Cause == nil {
		return context.DeadlineExceeded
	}
	return e.Cause
}

func (e *Error) Is(target error) bool {
	return target == ErrExceeded || target == context.DeadlineExceeded
}

// Config controls request deadline installation and mapping.
type Config struct {
	Timeout      time.Duration
	ErrorHandler func(*zinc.Context, *Error) error
}

type contextTimeoutContextKey int

const contextTimeoutInfoContextKey contextTimeoutContextKey = iota

// New installs Config.Timeout as the downstream request deadline. It does
// not run the handler in a detached goroutine, so cancellation remains idiomatic.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("timeout", configs)
	cfg := resolveContextTimeoutConfig(config)

	return func(c *zinc.Context) error {
		baseCtx := c.Context()
		timeoutCtx, cancel := context.WithTimeout(baseCtx, cfg.Timeout)
		defer cancel()

		c.SetContext(timeoutCtx)

		info := Info{Timeout: cfg.Timeout}
		if deadline, ok := timeoutCtx.Deadline(); ok {
			info.Deadline = deadline
		}
		c.Set(contextTimeoutInfoContextKey, info)

		err := c.Next()
		if err == nil {
			return nil
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		timeoutErr := &Error{
			Info:  info,
			Cause: err,
		}
		return cfg.ErrorHandler(c, timeoutErr)
	}
}

// Get returns installed deadline metadata.
func Get(c *zinc.Context) (Info, bool) {
	if c == nil {
		return Info{}, false
	}
	value, ok := c.Get(contextTimeoutInfoContextKey)
	if !ok {
		return Info{}, false
	}
	info, ok := value.(Info)
	return info, ok
}

// MustGet returns deadline metadata or panics when absent.
func MustGet(c *zinc.Context) Info {
	info, ok := Get(c)
	if !ok {
		panic("timeout: timeout info not found")
	}
	return info
}

// Remaining returns time until the installed deadline.
func Remaining(c *zinc.Context) (time.Duration, bool) {
	info, ok := Get(c)
	if !ok || info.Deadline.IsZero() {
		return 0, false
	}
	remaining := time.Until(info.Deadline)
	if remaining < 0 {
		return 0, true
	}
	return remaining, true
}

func resolveContextTimeoutConfig(config Config) Config {
	if config.Timeout <= 0 {
		panic("timeout: Timeout must be greater than zero")
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = func(_ *zinc.Context, err *Error) error {
			return errors.Join(zinc.ErrServiceUnavailable, err)
		}
	}
	return config
}
