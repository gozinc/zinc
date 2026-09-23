// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"mime"
	"strings"

	"github.com/0mjs/zinc"
)

// NoCache applies conservative headers that disable browser and intermediary caches.
func NoCache() zinc.Middleware {
	return func(c *zinc.Context) error {
		c.SetHeader(zinc.HeaderCacheControl, "no-cache, no-store, max-age=0, must-revalidate")
		c.SetHeader(zinc.HeaderPragma, "no-cache")
		c.SetHeader(zinc.HeaderExpires, "0")
		return c.Next()
	}
}

// Heartbeat answers path with 204 without invoking downstream handlers.
func Heartbeat(path string) zinc.Middleware {
	if path == "" {
		path = "/"
	}
	return func(c *zinc.Context) error {
		if c.Path() == path {
			return c.NoContent()
		}
		return c.Next()
	}
}

// RealIP replaces RemoteAddr with Context.IP. Its safety therefore depends on
// Config.TrustedProxies being configured correctly.
func RealIP() zinc.Middleware {
	return func(c *zinc.Context) error {
		if ip := c.IP(); ip != "" && c.Request() != nil {
			c.Request().RemoteAddr = ip
		}
		return c.Next()
	}
}

// Throttle bounds concurrent downstream requests and rejects excess work.
func Throttle(limit int) zinc.Middleware {
	if limit <= 0 {
		panic("zincthrottle: limit must be greater than zero")
	}
	sem := make(chan struct{}, limit)
	return func(c *zinc.Context) error {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			return c.Next()
		default:
			return zinc.ErrTooManyRequests
		}
	}
}

// Maybe applies mw only when predicate succeeds.
func Maybe(predicate func(*zinc.Context) bool, mw zinc.Middleware) zinc.Middleware {
	if predicate == nil {
		panic("zincmaybe: predicate is required")
	}
	if mw == nil {
		panic("zincmaybe: middleware is required")
	}
	return func(c *zinc.Context) error {
		if predicate(c) {
			return mw(c)
		}
		return c.Next()
	}
}

// AllowContentType rejects request media types outside the normalized allowlist.
func AllowContentType(types ...string) zinc.Middleware {
	allowed := mediaTypeSet(types)
	return func(c *zinc.Context) error {
		if _, ok := allowed[c.ContentType()]; ok {
			return c.Next()
		}
		return zinc.ErrUnsupportedMediaType
	}
}

// AllowContentEncoding rejects any content coding outside the allowlist.
func AllowContentEncoding(encodings ...string) zinc.Middleware {
	allowed := stringSet(encodings)
	return func(c *zinc.Context) error {
		raw := strings.TrimSpace(c.GetHeader(zinc.HeaderContentEncoding))
		if raw == "" {
			raw = "identity"
		}
		for _, part := range strings.Split(raw, ",") {
			encoding := strings.ToLower(strings.TrimSpace(part))
			if encoding == "" {
				continue
			}
			if _, ok := allowed[encoding]; !ok {
				return zinc.ErrUnsupportedMediaType
			}
		}
		return c.Next()
	}
}

// SetHeader sets one response header before downstream execution.
func SetHeader(key, value string) zinc.Middleware {
	return func(c *zinc.Context) error {
		c.SetHeader(key, value)
		return c.Next()
	}
}

// HeaderRoute conditionally selects middleware by request header.
type HeaderRoute struct {
	Header     string
	Value      string
	Middleware zinc.Middleware
}

// RouteHeaders applies the first matching rule.
func RouteHeaders(rules ...HeaderRoute) zinc.Middleware {
	compiled := make([]HeaderRoute, 0, len(rules))
	for _, rule := range rules {
		if rule.Header == "" || rule.Middleware == nil {
			continue
		}
		compiled = append(compiled, rule)
	}

	return func(c *zinc.Context) error {
		for _, rule := range compiled {
			value := c.GetHeader(rule.Header)
			if value == "" {
				continue
			}
			if rule.Value == "" || value == rule.Value {
				return rule.Middleware(c)
			}
		}
		return c.Next()
	}
}

func mediaTypeSet(types []string) map[string]struct{} {
	out := make(map[string]struct{}, len(types))
	for _, value := range types {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mediaType, _, err := mime.ParseMediaType(value); err == nil {
			value = mediaType
		} else if mediaType, _, ok := strings.Cut(value, ";"); ok {
			value = mediaType
		}
		out[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}
