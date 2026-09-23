// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import "time"

// Config holds the application and server configuration.
type Config struct {
	// ServerHeader is written to every response when non-empty.
	ServerHeader string

	// CaseSensitive controls whether literal route segments are case-sensitive.
	CaseSensitive bool
	// StrictRouting treats a trailing slash as part of the route identity.
	StrictRouting bool
	// AutoHead serves HEAD through a matching GET route when no HEAD route exists.
	AutoHead bool
	// AutoOptions answers OPTIONS from the methods registered for a path.
	AutoOptions bool
	// HandleMethodNotAllowed distinguishes method mismatches from not-found paths.
	HandleMethodNotAllowed bool

	// BodyLimit is the maximum request body size accepted by binding helpers.
	BodyLimit int64
	// ReadTimeout bounds reading the request, including its body.
	ReadTimeout time.Duration
	// WriteTimeout bounds writing the response.
	WriteTimeout time.Duration
	// IdleTimeout controls keep-alive connection lifetime.
	IdleTimeout time.Duration

	// ProxyHeader identifies the forwarding header used by Context.IP.
	ProxyHeader string
	// TrustedProxies restricts which immediate peers may supply forwarding headers.
	TrustedProxies []string

	// RequestBinder replaces the default request-data binder.
	RequestBinder RequestBinder
	// Validator runs after binding when Context.Bind().Validate is used.
	Validator Validator
	// Renderer provides named template rendering.
	Renderer Renderer
	// JSONCodec replaces the standard JSON encoder and decoder.
	JSONCodec JSONCodec
	// ErrorHandler receives errors returned by handlers and middleware.
	ErrorHandler ErrorHandler

	// RouteCacheSize bounds cached concrete dynamic paths; zero disables caching.
	RouteCacheSize int
}

// DefaultConfig is the baseline Zinc configuration copied by New.
// It remains mutable for compatibility; applications should prefer NewWithConfig.
var DefaultConfig = Config{
	ServerHeader:           "",
	CaseSensitive:          false,
	StrictRouting:          false,
	AutoHead:               true,
	AutoOptions:            true,
	HandleMethodNotAllowed: true,
	BodyLimit:              4 << 20,
	ReadTimeout:            5 * time.Second,
	WriteTimeout:           10 * time.Second,
	IdleTimeout:            120 * time.Second,
	ProxyHeader:            "X-Forwarded-For",
	TrustedProxies:         nil,
	RouteCacheSize:         1000,
}
