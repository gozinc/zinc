// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/0mjs/zinc"
)

// RecoverHandler maps a recovered panic to a handler error.
type RecoverHandler func(*zinc.Context, *RecoverError) error

// RecoverConfig controls panic recovery and stack capture.
type RecoverConfig struct {
	Skipper      func(*zinc.Context) bool
	Handler      RecoverHandler
	StackSize    int
	DisableStack bool
}

// RecoverError preserves the recovered value and optional current-goroutine stack.
type RecoverError struct {
	Value any
	Stack []byte
}

func (e *RecoverError) Error() string {
	if e == nil {
		return "zincrecover: panic recovered"
	}
	return fmt.Sprintf("zincrecover: panic recovered: %v", e.Value)
}

// DefaultRecoverConfig captures a four-kilobyte current-goroutine stack.
func DefaultRecoverConfig() RecoverConfig {
	return RecoverConfig{
		Handler:   defaultRecoverHandler,
		StackSize: 4 << 10,
	}
}

// Recover converts downstream panics into internal-server errors.
func Recover() zinc.Middleware {
	return RecoverWithConfig(DefaultRecoverConfig())
}

// RecoverWithConfig converts downstream panics into errors. It does not recover
// panics from goroutines started by the application.
func RecoverWithConfig(config RecoverConfig) zinc.Middleware {
	cfg := resolveRecoverConfig(config)

	return func(c *zinc.Context) (err error) {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		defer func() {
			if value := recover(); value != nil {
				recoverErr := &RecoverError{
					Value: value,
					Stack: captureRecoverStack(cfg),
				}
				err = cfg.Handler(c, recoverErr)
			}
		}()

		return c.Next()
	}
}

func resolveRecoverConfig(config RecoverConfig) RecoverConfig {
	cfg := DefaultRecoverConfig()
	cfg.Skipper = config.Skipper
	if config.Handler != nil {
		cfg.Handler = config.Handler
	}
	if config.StackSize != 0 {
		cfg.StackSize = config.StackSize
	}
	cfg.DisableStack = config.DisableStack
	if cfg.Handler == nil {
		panic("zincrecover: Handler is required")
	}
	if cfg.StackSize < 0 {
		panic("zincrecover: StackSize must be greater than or equal to zero")
	}
	return cfg
}

func captureRecoverStack(cfg RecoverConfig) []byte {
	if cfg.DisableStack || cfg.StackSize == 0 {
		return nil
	}
	stack := make([]byte, cfg.StackSize)
	n := runtime.Stack(stack, false)
	return stack[:n]
}

func defaultRecoverHandler(_ *zinc.Context, err *RecoverError) error {
	return errors.Join(zinc.ErrInternalServerError, err)
}
