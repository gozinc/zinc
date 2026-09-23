// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"strings"

	"github.com/0mjs/zinc"
)

const (
	// Method override headers expose the requested and original methods.
	HeaderXHTTPMethodOverride = "X-HTTP-Method-Override"
	HeaderXOriginalMethod     = "X-Original-Method"
)

// MethodOverrideGetter reads a requested replacement method.
type MethodOverrideGetter func(*zinc.Context) string

// MethodOverrideConfig limits both source and destination methods.
type MethodOverrideConfig struct {
	Skipper       func(*zinc.Context) bool
	Getter        MethodOverrideGetter
	SourceMethods []string
	Methods       []string
}

// MethodOverride allows POST to request PUT, PATCH, or DELETE via the standard header.
func MethodOverride() zinc.Middleware {
	return MethodOverrideWithConfig(MethodOverrideConfig{})
}

// MethodOverrideWithConfig rewrites only explicitly permitted source methods,
// preventing arbitrary header input from widening route access.
func MethodOverrideWithConfig(config MethodOverrideConfig) zinc.Middleware {
	cfg := resolveMethodOverrideConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		req := c.Request()
		if req == nil || !methodInSet(req.Method, cfg.SourceMethods) {
			return c.Next()
		}

		override := strings.ToUpper(strings.TrimSpace(cfg.Getter(c)))
		if override == "" {
			return c.Next()
		}
		if !methodInSet(override, cfg.Methods) {
			return zinc.ErrMethodNotAllowed.WithMessage("method override not allowed")
		}

		req.Header.Set(HeaderXOriginalMethod, req.Method)
		req.Method = override
		return c.Next()
	}
}

func MethodOverrideFromHeader(header string) MethodOverrideGetter {
	return func(c *zinc.Context) string {
		return c.GetHeader(header)
	}
}

func MethodOverrideFromQuery(name string) MethodOverrideGetter {
	return func(c *zinc.Context) string {
		return c.Query(name)
	}
}

func MethodOverrideFromFirst(getters ...MethodOverrideGetter) MethodOverrideGetter {
	list := append([]MethodOverrideGetter(nil), getters...)
	return func(c *zinc.Context) string {
		for _, getter := range list {
			if getter == nil {
				continue
			}
			if method := strings.TrimSpace(getter(c)); method != "" {
				return method
			}
		}
		return ""
	}
}

func resolveMethodOverrideConfig(config MethodOverrideConfig) MethodOverrideConfig {
	cfg := MethodOverrideConfig{
		Getter:        MethodOverrideFromHeader(HeaderXHTTPMethodOverride),
		SourceMethods: []string{http.MethodPost},
		Methods: []string{
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
	}
	cfg.Skipper = config.Skipper
	if config.Getter != nil {
		cfg.Getter = config.Getter
	}
	if len(config.SourceMethods) > 0 {
		cfg.SourceMethods = normalizeMethods(config.SourceMethods)
	}
	if len(config.Methods) > 0 {
		cfg.Methods = normalizeMethods(config.Methods)
	}
	return cfg
}

func normalizeMethods(methods []string) []string {
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if method != "" {
			out = append(out, method)
		}
	}
	return out
}

func methodInSet(method string, methods []string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, allowed := range methods {
		if method == allowed {
			return true
		}
	}
	return false
}
