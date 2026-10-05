// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package redirect answers matching paths with a redirect.
package redirect

import (
	"net/http"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/internal/marks"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config maps request paths to redirect targets.
type Config struct {
	// Rules maps a path to its target. A path ending in "*" matches a prefix,
	// and a "*" in the target is replaced by the rest of the path, still
	// escaped. A target without a scheme or host is a path on this site.
	Rules map[string]string
	// StatusCode is the redirect status. Zero uses 301 Moved Permanently.
	StatusCode int
}

// New redirects requests whose path matches a rule, keeping the original
// query string. Register it with App.Use or App.UsePrefix, so it runs before
// routing. Registering it on a group panics, since it would only see paths
// that already have a route; on a single route it logs a warning the first
// time it runs.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("redirect", configs)
	rules := shared.CloneRewriteRules(config.Rules)
	statusCode := config.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusMovedPermanently
	}
	placement := shared.NewRoutingWarning("redirect")

	mw := func(c *zinc.Context) error {
		req := c.Request()
		if req == marks.Probe {
			return probeAnswer
		}
		placement.Check(c)
		if req == nil || req.URL == nil {
			return c.Next()
		}

		to, tail, prefix, ok := shared.MatchRule(req.URL.Path, rules)
		if !ok {
			return c.Next()
		}
		target := to
		if prefix {
			target = shared.ExpandTarget(to, shared.EscapedTail(req.URL, len(tail)))
			// The tail comes from the request, so unless the rule itself
			// names another site, the result must stay on this one.
			if !shared.ExternalTarget(to) {
				target = shared.SameSitePath(target)
			}
		}
		return c.Status(statusCode).Redirect(shared.PathWithRawQuery(target, req.URL.RawQuery))
	}
	marks.Answers(mw)
	return mw
}

// probeAnswer tells Zinc a redirect must run before routing: on a group it
// would run after routing, so Group.Use panics.
var probeAnswer = &marks.Marks{Prerouting: "redirect"}
