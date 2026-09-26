// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package trailingslash normalizes trailing slashes in request paths.
package trailingslash

import (
	"net/http"
	"strings"

	"github.com/0mjs/zinc"
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
// string.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("trailingslash", configs)
	cfg := resolveTrailingSlashConfig(config)

	return func(c *zinc.Context) error {
		req := c.Request()
		if req == nil || req.URL == nil {
			return c.Next()
		}

		nextPath := normalizeTrailingSlashPath(req.URL.Path, cfg.Add)
		if nextPath == req.URL.Path {
			return c.Next()
		}

		if cfg.Redirect {
			return c.Status(cfg.StatusCode).Redirect(shared.PathWithRawQuery(nextPath, req.URL.RawQuery))
		}

		c.SetPath(nextPath)
		return c.Next()
	}
}

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
