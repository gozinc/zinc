// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package pprof serves the net/http/pprof profiling endpoints.
package pprof

import (
	"net/http/pprof"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// DefaultPrefix is the path the endpoints are served below when
// Config.Prefix is empty.
const DefaultPrefix = "/debug/pprof"

// Config controls where the profiling endpoints are served.
type Config struct {
	// Prefix is the path the endpoints are served below. Empty means
	// DefaultPrefix.
	Prefix string
}

// New serves the standard pprof handlers below Config.Prefix. Protect or
// disable these endpoints in untrusted environments.
func New(configs ...Config) zinc.Middleware {
	prefix := strings.TrimRight(shared.Config("pprof", configs).Prefix, "/")
	if prefix == "" {
		prefix = DefaultPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	return func(c *zinc.Context) error {
		switch c.Path() {
		case prefix, prefix + "/":
			pprof.Index(c.Writer(), c.Request())
			return nil
		case prefix + "/cmdline":
			pprof.Cmdline(c.Writer(), c.Request())
			return nil
		case prefix + "/profile":
			pprof.Profile(c.Writer(), c.Request())
			return nil
		case prefix + "/symbol":
			pprof.Symbol(c.Writer(), c.Request())
			return nil
		case prefix + "/trace":
			pprof.Trace(c.Writer(), c.Request())
			return nil
		default:
			if strings.HasPrefix(c.Path(), prefix+"/") {
				pprof.Index(c.Writer(), c.Request())
				return nil
			}
			return c.Next()
		}
	}
}
