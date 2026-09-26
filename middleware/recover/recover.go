// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package recover

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config controls panic recovery and stack capture.
type Config struct {
	Handler      func(*zinc.Context, *Error) error
	StackSize    int
	DisableStack bool
}

// Error preserves the recovered value and optional current-goroutine stack.
type Error struct {
	Value any
	Stack []byte
}

func (e *Error) Error() string {
	if e == nil {
		return "recover: panic recovered"
	}
	return fmt.Sprintf("recover: panic recovered: %v", e.Value)
}

// defaultConfig captures a four-kilobyte current-goroutine stack.
func defaultConfig() Config {
	return Config{
		Handler:   defaultRecoverHandler,
		StackSize: 4 << 10,
	}
}

// New converts downstream panics into errors. It does not recover
// panics from goroutines started by the application.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("recover", configs)
	cfg := resolveRecoverConfig(config)

	return func(c *zinc.Context) (err error) {
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler {
					panic(value)
				}
				recoverErr := &Error{
					Value: value,
					Stack: captureRecoverStack(cfg),
				}
				err = cfg.Handler(c, recoverErr)
			}
		}()

		return c.Next()
	}
}

func resolveRecoverConfig(config Config) Config {
	cfg := defaultConfig()
	if config.Handler != nil {
		cfg.Handler = config.Handler
	}
	if config.StackSize != 0 {
		cfg.StackSize = config.StackSize
	}
	cfg.DisableStack = config.DisableStack
	if cfg.Handler == nil {
		panic("recover: Handler is required")
	}
	if cfg.StackSize < 0 {
		panic("recover: StackSize must be greater than or equal to zero")
	}
	return cfg
}

func captureRecoverStack(cfg Config) []byte {
	if cfg.DisableStack || cfg.StackSize == 0 {
		return nil
	}
	stack := make([]byte, cfg.StackSize)
	n := runtime.Stack(stack, false)
	return stack[:n]
}

func defaultRecoverHandler(_ *zinc.Context, err *Error) error {
	return errors.Join(zinc.ErrInternalServerError, err)
}
