// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package rewrite routes matching paths as other paths.
package rewrite

import (
	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config maps incoming paths to internal paths.
type Config struct {
	// Rules maps a path to the path to route instead. A path ending in "*"
	// matches a prefix, and a "*" in the target is replaced by the rest of
	// the path.
	Rules map[string]string
}

// New changes the routed path of matching requests without a redirect.
// Query parameters remain untouched. Register it with App.Use or
// App.UsePrefix: on a group or a route it runs after routing, when the route
// can no longer change, and logs a warning the first time.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("rewrite", configs)
	rules := shared.CloneRewriteRules(config.Rules)
	placement := shared.NewRoutingWarning("rewrite")

	return func(c *zinc.Context) error {
		placement.Check(c)
		req := c.Request()
		if req == nil || req.URL == nil {
			return c.Next()
		}

		if target, ok := shared.RewriteTarget(req.URL.Path, rules); ok {
			c.SetPath(target)
		}
		return c.Next()
	}
}
