// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package casbin

import (
	"errors"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/basicauth"
	"github.com/0mjs/zinc/middleware/internal/shared"
	"github.com/0mjs/zinc/middleware/keyauth"
)

// Enforcer is the subset of a Casbin enforcer required by this middleware.
type Enforcer interface {
	Enforce(args ...any) (bool, error)
}

// ValueFunc derives a subject, object, or action from a request.
type ValueFunc func(*zinc.Context) any

// Config maps request state to a Casbin enforcement tuple.
type Config struct {
	Enforcer       Enforcer
	Subject        ValueFunc
	Object         ValueFunc
	Action         ValueFunc
	SuccessHandler zinc.HandlerFunc
	ErrorHandler   func(*zinc.Context, error) error
}

// ErrRejected identifies a valid policy decision that denied access.
var ErrRejected = errors.New("casbin: request rejected")

// New enforces Casbin policy on subject, object, and action, in that order.
// Config.Enforcer and Config.Subject are required.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("casbin", configs)
	cfg := resolveCasbinAuthConfig(config)

	return func(c *zinc.Context) error {
		ok, err := cfg.Enforcer.Enforce(cfg.Subject(c), cfg.Object(c), cfg.Action(c))
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}
		if !ok {
			return cfg.ErrorHandler(c, ErrRejected)
		}
		return cfg.SuccessHandler(c)
	}
}

// SubjectFromBasicAuth uses the authenticated Basic username.
func SubjectFromBasicAuth() ValueFunc {
	return func(c *zinc.Context) any {
		identity, _ := basicauth.Get(c)
		return identity.Username
	}
}

// SubjectFromKeyAuth uses the authenticated key.
func SubjectFromKeyAuth() ValueFunc {
	return func(c *zinc.Context) any {
		state, ok := keyauth.Get(c)
		if !ok {
			return ""
		}
		return state.Key
	}
}

// SubjectFromContext reads a request-scoped value.
func SubjectFromContext(key any) ValueFunc {
	return func(c *zinc.Context) any {
		value, _ := c.Get(key)
		return value
	}
}

// ObjectPath uses the current URL path as the policy object.
func ObjectPath() ValueFunc {
	return func(c *zinc.Context) any {
		return c.Path()
	}
}

// ActionMethod uses the request method as the policy action.
func ActionMethod() ValueFunc {
	return func(c *zinc.Context) any {
		return c.Method()
	}
}

func resolveCasbinAuthConfig(config Config) Config {
	cfg := Config{
		Object: ObjectPath(),
		Action: ActionMethod(),
		SuccessHandler: func(c *zinc.Context) error {
			return c.Next()
		},
		ErrorHandler: func(_ *zinc.Context, err error) error {
			if errors.Is(err, ErrRejected) {
				return zinc.ErrForbidden
			}
			return err
		},
	}
	if config.Enforcer == nil {
		panic("casbin: Enforcer is required")
	}
	cfg.Enforcer = config.Enforcer
	if config.Subject == nil {
		panic("casbin: Subject is required")
	}
	cfg.Subject = config.Subject
	if config.Object != nil {
		cfg.Object = config.Object
	}
	if config.Action != nil {
		cfg.Action = config.Action
	}
	if config.SuccessHandler != nil {
		cfg.SuccessHandler = config.SuccessHandler
	}
	if config.ErrorHandler != nil {
		cfg.ErrorHandler = config.ErrorHandler
	}
	return cfg
}
