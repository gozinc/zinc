// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http/pprof"
	"strings"

	"github.com/0mjs/zinc"
)

const defaultPprofPrefix = "/debug/pprof"

// Pprof exposes the standard net/http/pprof handlers under /debug/pprof.
// Protect or disable these endpoints in untrusted environments.
func Pprof() zinc.Middleware {
	return PprofWithPrefix(defaultPprofPrefix)
}

// PprofWithPrefix exposes the standard pprof handlers below prefix.
func PprofWithPrefix(prefix string) zinc.Middleware {
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		prefix = defaultPprofPrefix
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
