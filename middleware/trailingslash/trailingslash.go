// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package trailingslash normalizes trailing slashes in request paths.
package trailingslash

import (
	"net/http"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/internal/marks"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config controls path normalization. The zero value removes trailing
// slashes and routes the result without redirecting.
type Config struct {
	// Add appends a trailing slash instead of removing it.
	Add bool
	// Redirect answers with a redirect to the normalized path instead of
	// routing it directly.
	Redirect bool
	// StatusCode is the redirect status. Zero uses 301 Moved Permanently.
	StatusCode int
}

// New normalizes the trailing slash of the request path, keeping the query
// string. Register it with App.Use or App.UsePrefix, so it runs before
// routing. Registering it on a group panics, since the route can no longer
// change there; on a single route it logs a warning the first time it runs.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("trailingslash", configs)
	cfg := resolveTrailingSlashConfig(config)
	placement := shared.NewRoutingWarning("trailingslash")

	mw := func(c *zinc.Context) error {
		req := c.Request()
		if req == marks.Probe {
			return probeAnswer
		}
		placement.Check(c)
		if req == nil || req.URL == nil {
			return c.Next()
		}

		nextPath := normalizeTrailingSlashPath(req.URL.Path, cfg.Add)
		if nextPath == req.URL.Path {
			return c.Next()
		}
		// A static directory's URL ends with a slash, and the URL without
		// it redirects back there, so removing the slash would loop.
		if !cfg.Add && marks.StaticDirectory != nil && marks.StaticDirectory(c, req.URL.Path) {
			return c.Next()
		}

		if cfg.Redirect {
			// Build the target from the escaped path, so an encoded slash
			// stays encoded. When the slash itself is encoded, the escaped
			// path does not change and redirecting would loop, so route it.
			escaped := req.URL.EscapedPath()
			if target := normalizeTrailingSlashPath(escaped, cfg.Add); target != escaped {
				target = shared.SameSitePath(target)
				return c.Status(cfg.StatusCode).Redirect(shared.PathWithRawQuery(target, req.URL.RawQuery))
			}
		}

		c.SetPath(nextPath)
		return c.Next()
	}
	marks.Answers(mw)
	return mw
}

// probeAnswer tells Zinc a trailingslash must run before routing: on a group
// it would run after routing, so Group.Use panics.
var probeAnswer = &marks.Marks{Prerouting: "trailingslash"}

func resolveTrailingSlashConfig(config Config) Config {
	if config.StatusCode == 0 {
		config.StatusCode = http.StatusMovedPermanently
	}
	return config
}

func normalizeTrailingSlashPath(path string, add bool) string {
	if path == "" {
		return "/"
	}
	if path == "/" {
		return path
	}
	if add {
		if strings.HasSuffix(path, "/") {
			return path
		}
		return path + "/"
	}
	return strings.TrimRight(path, "/")
}
