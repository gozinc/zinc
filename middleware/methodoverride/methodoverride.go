// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package methodoverride

import (
	"net/http"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

const (
	// Method override headers expose the requested and original methods.
	HeaderXHTTPMethodOverride = "X-HTTP-Method-Override"
	HeaderXOriginalMethod     = "X-Original-Method"
)

// Getter reads a requested replacement method.
type Getter func(*zinc.Context) string

// Config limits both source and destination methods.
type Config struct {
	Getter        Getter
	SourceMethods []string
	Methods       []string
}

// New lets POST requests ask for PUT, PATCH, or DELETE. It rewrites only
// explicitly permitted source methods, so header input cannot widen route access.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("methodoverride", configs)
	cfg := resolveMethodOverrideConfig(config)

	return func(c *zinc.Context) error {
		req := c.Request()
		if req == nil || !methodInSet(req.Method, cfg.SourceMethods) {
			return c.Next()
		}

		override := strings.ToUpper(strings.TrimSpace(cfg.Getter(c)))
		if override == "" {
			return c.Next()
		}
		if !methodInSet(override, cfg.Methods) {
			return zinc.NewError(zinc.StatusMethodNotAllowed, "method override not allowed")
		}

		req.Header.Set(HeaderXOriginalMethod, req.Method)
		req.Method = override
		return c.Next()
	}
}

func FromHeader(header string) Getter {
	return func(c *zinc.Context) string {
		return c.Header(header)
	}
}

func FromQuery(name string) Getter {
	return func(c *zinc.Context) string {
		return c.Query(name)
	}
}

func FromFirst(getters ...Getter) Getter {
	list := append([]Getter(nil), getters...)
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

func resolveMethodOverrideConfig(config Config) Config {
	cfg := Config{
		Getter:        FromHeader(HeaderXHTTPMethodOverride),
		SourceMethods: []string{http.MethodPost},
		Methods: []string{
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
	}
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
