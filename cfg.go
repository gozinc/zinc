// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"time"
)

// Defaults applied to zero-valued Config fields.
const (
	DefaultBodyLimit       int64 = 4 << 20
	DefaultReadTimeout           = 5 * time.Second
	DefaultWriteTimeout          = 10 * time.Second
	DefaultIdleTimeout           = 120 * time.Second
	DefaultShutdownTimeout       = 10 * time.Second
	DefaultRouteCacheSize        = 1000
	DefaultProxyHeader           = "X-Forwarded-For"
)

// Config holds the application and server configuration. The zero value of
// every field is the default, so a literal only needs the fields it changes:
//
//	app := zinc.New(zinc.Config{BodyLimit: 64 << 20})
//
// For limits and timeouts, 0 selects the default and a negative value turns
// the limit off.
type Config struct {
	// ServerHeader is written to every response when non-empty.
	ServerHeader string

	// CaseSensitive makes literal route segments case-sensitive.
	CaseSensitive bool
	// StrictRouting treats a trailing slash as part of the route identity.
	StrictRouting bool
	// DisableAutoHead stops HEAD requests from being served by a matching GET
	// route when no HEAD route exists.
	DisableAutoHead bool
	// DisableAutoOptions stops OPTIONS requests from being answered with the
	// methods registered for a path.
	DisableAutoOptions bool
	// DisableMethodNotAllowed answers a method mismatch with 404 instead of
	// 405 and an Allow header.
	DisableMethodNotAllowed bool

	// BodyLimit is the maximum request body size accepted by binding helpers
	// and form parsing. 0 means DefaultBodyLimit; negative means no limit.
	BodyLimit int64
	// ReadTimeout bounds reading the request, including its body.
	// 0 means DefaultReadTimeout; negative means no timeout.
	ReadTimeout time.Duration
	// WriteTimeout bounds writing a response, or one event of a stream.
	// 0 means DefaultWriteTimeout; negative means no timeout.
	WriteTimeout time.Duration
	// IdleTimeout controls keep-alive connection lifetime.
	// 0 means DefaultIdleTimeout; negative means no timeout.
	IdleTimeout time.Duration
	// ShutdownTimeout bounds how long ListenContext waits for in-flight
	// requests after its context ends. 0 means DefaultShutdownTimeout;
	// negative means wait until they finish.
	ShutdownTimeout time.Duration

	// CookieSameSite is applied to cookies written by SetCookie and
	// ClearCookie that don't set SameSite themselves. Zero leaves them unset.
	CookieSameSite http.SameSite

	// ProxyHeader identifies the forwarding header used by Context.IP.
	// Empty means DefaultProxyHeader.
	ProxyHeader string
	// TrustedProxies contains IPs/CIDRs trusted in the forwarded chain.
	// Entries are copied and validated at construction.
	TrustedProxies []string

	// RequestBinder replaces the default request-data binder.
	RequestBinder RequestBinder
	// Validator runs after successful default binding, or through Context.Validate.
	Validator Validator
	// Renderer provides named template rendering.
	Renderer Renderer
	// JSONCodec replaces the standard JSON encoder and decoder.
	JSONCodec JSONCodec
	// ErrorHandler receives errors returned by handlers and middleware.
	// Nil means DefaultErrorHandler.
	ErrorHandler ErrorHandler

	// RouteCacheSize bounds cached concrete dynamic paths.
	// 0 means DefaultRouteCacheSize; negative disables the cache.
	RouteCacheSize int
}

// orDefault resolves the zero value of a limit to its default. Negative
// values, which turn the limit off, pass through for callers to interpret.
func orDefault[T int | int64 | time.Duration](value, fallback T) T {
	if value == 0 {
		return fallback
	}
	return value
}

// serverTimeout converts a resolved timeout to net/http's convention, where
// zero means no timeout.
func serverTimeout(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}
