// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package redirect answers matching paths with a redirect.
package redirect

import (
	"net/http"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config maps request paths to redirect targets.
type Config struct {
	// Rules maps a path to its target. A path ending in "*" matches a prefix,
	// and a "*" in the target is replaced by the rest of the path.
	Rules map[string]string
	// StatusCode is the redirect status. Zero uses 301 Moved Permanently.
	StatusCode int
}

// New redirects requests whose path matches a rule, keeping the original
// query string. Register it with App.Use or App.UsePrefix: on a group or a
// route it only sees paths that have a route, and logs a warning the first
// time it runs there.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("redirect", configs)
	rules := shared.CloneRewriteRules(config.Rules)
	statusCode := config.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusMovedPermanently
	}
	placement := shared.NewRoutingWarning("redirect")

	return func(c *zinc.Context) error {
		placement.Check(c)
		req := c.Request()
		if req == nil || req.URL == nil {
			return c.Next()
		}

		target, ok := shared.RewriteTarget(req.URL.Path, rules)
		if !ok {
			return c.Next()
		}
		return c.Status(statusCode).Redirect(shared.PathWithRawQuery(target, req.URL.RawQuery))
	}
}
