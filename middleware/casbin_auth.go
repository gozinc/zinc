// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"errors"

	"github.com/0mjs/zinc"
)

// CasbinEnforcer is the subset of a Casbin enforcer required by this middleware.
type CasbinEnforcer interface {
	Enforce(args ...any) (bool, error)
}

// CasbinValueFunc derives a subject, object, or action from a request.
type CasbinValueFunc func(*zinc.Context) any

// CasbinErrorHandler maps policy errors and denials.
type CasbinErrorHandler func(*zinc.Context, error) error

// CasbinAuthConfig maps request state to a Casbin enforcement tuple.
type CasbinAuthConfig struct {
	Skipper        func(*zinc.Context) bool
	Enforcer       CasbinEnforcer
	Subject        CasbinValueFunc
	Object         CasbinValueFunc
	Action         CasbinValueFunc
	SuccessHandler zinc.RouteHandler
	ErrorHandler   CasbinErrorHandler
}

// ErrCasbinAuthRejected identifies a valid policy decision that denied access.
var ErrCasbinAuthRejected = errors.New("zinccasbin: request rejected")

// CasbinAuth enforces subject against the request path and method.
func CasbinAuth(enforcer CasbinEnforcer, subject CasbinValueFunc) zinc.Middleware {
	return CasbinAuthWithConfig(CasbinAuthConfig{
		Enforcer: enforcer,
		Subject:  subject,
	})
}

// CasbinAuthWithConfig enforces subject, object, and action in that order.
func CasbinAuthWithConfig(config CasbinAuthConfig) zinc.Middleware {
	cfg := resolveCasbinAuthConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		ok, err := cfg.Enforcer.Enforce(cfg.Subject(c), cfg.Object(c), cfg.Action(c))
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}
		if !ok {
			return cfg.ErrorHandler(c, ErrCasbinAuthRejected)
		}
		return cfg.SuccessHandler(c)
	}
}

// CasbinSubjectFromBasicAuth uses the authenticated Basic username.
func CasbinSubjectFromBasicAuth() CasbinValueFunc {
	return func(c *zinc.Context) any {
		username, _ := BasicAuthUsername(c)
		return username
	}
}

// CasbinSubjectFromKeyAuth uses the authenticated key.
func CasbinSubjectFromKeyAuth() CasbinValueFunc {
	return func(c *zinc.Context) any {
		state, ok := KeyAuthCurrent(c)
		if !ok {
			return ""
		}
		return state.Key
	}
}

// CasbinSubjectFromContext reads a request-scoped value.
func CasbinSubjectFromContext(key any) CasbinValueFunc {
	return func(c *zinc.Context) any {
		value, _ := c.Get(key)
		return value
	}
}

// CasbinObjectPath uses the current URL path as the policy object.
func CasbinObjectPath() CasbinValueFunc {
	return func(c *zinc.Context) any {
		return c.Path()
	}
}

// CasbinActionMethod uses the request method as the policy action.
func CasbinActionMethod() CasbinValueFunc {
	return func(c *zinc.Context) any {
		return c.Method()
	}
}

func resolveCasbinAuthConfig(config CasbinAuthConfig) CasbinAuthConfig {
	cfg := CasbinAuthConfig{
		Object: CasbinObjectPath(),
		Action: CasbinActionMethod(),
		SuccessHandler: func(c *zinc.Context) error {
			return c.Next()
		},
		ErrorHandler: func(_ *zinc.Context, err error) error {
			if errors.Is(err, ErrCasbinAuthRejected) {
				return zinc.ErrForbidden
			}
			return err
		},
	}
	cfg.Skipper = config.Skipper
	if config.Enforcer == nil {
		panic("zinccasbin: Enforcer is required")
	}
	cfg.Enforcer = config.Enforcer
	if config.Subject == nil {
		panic("zinccasbin: Subject is required")
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
