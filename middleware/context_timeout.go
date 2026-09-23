// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0mjs/zinc"
)

// ErrContextTimeout identifies a downstream context deadline.
var ErrContextTimeout = errors.New("zinccontexttimeout: request context deadline exceeded")

// ContextTimeoutInfo describes the request deadline installed by middleware.
type ContextTimeoutInfo struct {
	Timeout  time.Duration
	Deadline time.Time
}

// ContextTimeoutError preserves deadline metadata and the underlying error.
type ContextTimeoutError struct {
	Info  ContextTimeoutInfo
	Cause error
}

func (e *ContextTimeoutError) Error() string {
	if e == nil {
		return ErrContextTimeout.Error()
	}
	if e.Info.Timeout > 0 {
		return fmt.Sprintf("zinccontexttimeout: request exceeded timeout %s", e.Info.Timeout)
	}
	return ErrContextTimeout.Error()
}

func (e *ContextTimeoutError) Unwrap() error {
	if e == nil || e.Cause == nil {
		return context.DeadlineExceeded
	}
	return e.Cause
}

func (e *ContextTimeoutError) Is(target error) bool {
	return target == ErrContextTimeout || target == context.DeadlineExceeded
}

// ContextTimeoutErrorHandler maps an elapsed deadline to a handler error.
type ContextTimeoutErrorHandler func(*zinc.Context, *ContextTimeoutError) error

// ContextTimeoutConfig controls request deadline installation and mapping.
type ContextTimeoutConfig struct {
	Skipper      func(*zinc.Context) bool
	Timeout      time.Duration
	ErrorHandler ContextTimeoutErrorHandler
}

type contextTimeoutContextKey int

const contextTimeoutInfoContextKey contextTimeoutContextKey = iota

// ContextTimeout installs timeout as the downstream request deadline.
func ContextTimeout(timeout time.Duration) zinc.Middleware {
	return ContextTimeoutWithConfig(ContextTimeoutConfig{Timeout: timeout})
}

// ContextTimeoutWithConfig derives a timed context for downstream work. It does
// not run the handler in a detached goroutine, so cancellation remains idiomatic.
func ContextTimeoutWithConfig(config ContextTimeoutConfig) zinc.Middleware {
	cfg := resolveContextTimeoutConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		baseCtx := c.Context()
		timeoutCtx, cancel := context.WithTimeout(baseCtx, cfg.Timeout)
		defer cancel()

		c.SetContext(timeoutCtx)

		info := ContextTimeoutInfo{Timeout: cfg.Timeout}
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

		timeoutErr := &ContextTimeoutError{
			Info:  info,
			Cause: err,
		}
		return cfg.ErrorHandler(c, timeoutErr)
	}
}

// ContextTimeoutCurrent returns installed deadline metadata.
func ContextTimeoutCurrent(c *zinc.Context) (ContextTimeoutInfo, bool) {
	if c == nil {
		return ContextTimeoutInfo{}, false
	}
	value, ok := c.Get(contextTimeoutInfoContextKey)
	if !ok {
		return ContextTimeoutInfo{}, false
	}
	info, ok := value.(ContextTimeoutInfo)
	return info, ok
}

// MustContextTimeoutCurrent returns deadline metadata or panics when absent.
func MustContextTimeoutCurrent(c *zinc.Context) ContextTimeoutInfo {
	info, ok := ContextTimeoutCurrent(c)
	if !ok {
		panic("zinccontexttimeout: timeout info not found")
	}
	return info
}

// ContextTimeoutRemaining returns time until the installed deadline.
func ContextTimeoutRemaining(c *zinc.Context) (time.Duration, bool) {
	info, ok := ContextTimeoutCurrent(c)
	if !ok || info.Deadline.IsZero() {
		return 0, false
	}
	remaining := time.Until(info.Deadline)
	if remaining < 0 {
		return 0, true
	}
	return remaining, true
}

func resolveContextTimeoutConfig(config ContextTimeoutConfig) ContextTimeoutConfig {
	if config.Timeout <= 0 {
		panic("zinccontexttimeout: Timeout must be greater than zero")
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = func(_ *zinc.Context, err *ContextTimeoutError) error {
			return errors.Join(zinc.ErrServiceUnavailable, err)
		}
	}
	return config
}
