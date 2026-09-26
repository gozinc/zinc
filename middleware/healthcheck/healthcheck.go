// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package healthcheck answers a health endpoint before routing work.
package healthcheck

import (
	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// DefaultPath is the endpoint answered when Config.Path is empty.
const DefaultPath = "/healthz"

// Config controls the health endpoint.
type Config struct {
	// Path is the endpoint to answer. Empty means DefaultPath.
	Path string
	// Check reports whether the service is healthy. A non-nil error answers
	// 503 Service Unavailable. Nil means always healthy.
	Check func(*zinc.Context) error
}

// New answers GET and HEAD requests for Config.Path with 204 No Content, or
// 503 when Check fails, without invoking downstream handlers.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("healthcheck", configs)
	path := config.Path
	if path == "" {
		path = DefaultPath
	}
	check := config.Check
	return func(c *zinc.Context) error {
		if c.Path() != path || (c.Method() != zinc.MethodGet && c.Method() != zinc.MethodHead) {
			return c.Next()
		}
		if check != nil {
			if err := check(c); err != nil {
				return zinc.ServiceUnavailable("unhealthy").Wrap(err)
			}
		}
		return c.NoContent()
	}
}
