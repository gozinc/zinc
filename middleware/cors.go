// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0mjs/zinc"
)

// CORSConfig defines origins, methods, and headers accepted across origins.
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
	Skipper          func(*zinc.Context) bool
}

// DefaultCORSConfig allows common methods and headers from any origin without credentials.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodHead,
			http.MethodPut,
			http.MethodDelete,
			http.MethodPatch,
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
		},
		ExposeHeaders:    []string{},
		AllowCredentials: false,
		MaxAge:           0,
	}
}

// CORSOption mutates CORS configuration before middleware construction.
type CORSOption func(*CORSConfig)

// CORS creates middleware for the supplied origins, or defaults when omitted.
func CORS(allowOrigins ...string) zinc.Middleware {
	if len(allowOrigins) == 0 {
		return CORSWithConfig(DefaultCORSConfig())
	}
	config := DefaultCORSConfig()
	config.AllowOrigins = append([]string(nil), allowOrigins...)
	return CORSWithConfig(config)
}

func CORSAllowOrigins(origins ...string) CORSOption {
	return func(c *CORSConfig) {
		c.AllowOrigins = origins
	}
}

func CORSAllowMethods(methods ...string) CORSOption {
	return func(c *CORSConfig) {
		c.AllowMethods = methods
	}
}

func CORSAllowHeaders(headers ...string) CORSOption {
	return func(c *CORSConfig) {
		c.AllowHeaders = headers
	}
}

func CORSExposeHeaders(headers ...string) CORSOption {
	return func(c *CORSConfig) {
		c.ExposeHeaders = headers
	}
}

func CORSAllowCredentials(allow bool) CORSOption {
	return func(c *CORSConfig) {
		c.AllowCredentials = allow
	}
}

func CORSMaxAge(maxAge time.Duration) CORSOption {
	return func(c *CORSConfig) {
		c.MaxAge = int(maxAge.Seconds())
	}
}

func CORSMaxAgeSeconds(seconds int) CORSOption {
	return func(c *CORSConfig) {
		c.MaxAge = seconds
	}
}

func CORSSkipper(skipper func(*zinc.Context) bool) CORSOption {
	return func(c *CORSConfig) {
		c.Skipper = skipper
	}
}

// CORSWithOptions applies functional options to the defaults.
func CORSWithOptions(options ...CORSOption) zinc.Middleware {
	config := DefaultCORSConfig()
	for _, option := range options {
		option(&config)
	}
	return CORSWithConfig(config)
}

// CORSWithConfig applies CORS response headers and terminates valid preflight
// requests. Vary is always set to keep shared caches origin-safe.
func CORSWithConfig(config CORSConfig) zinc.Middleware {
	allowOriginsMap := make(map[string]bool, len(config.AllowOrigins))
	for _, origin := range config.AllowOrigins {
		allowOriginsMap[origin] = true
	}

	allowMethodsStr := strings.Join(config.AllowMethods, ",")
	allowHeadersStr := strings.Join(config.AllowHeaders, ",")
	exposeHeadersStr := strings.Join(config.ExposeHeaders, ",")

	return func(c *zinc.Context) error {
		if config.Skipper != nil && config.Skipper(c) {
			return c.Next()
		}

		c.Writer().Header().Add("Vary", "Origin")
		c.Writer().Header().Add("Vary", "Access-Control-Request-Method")
		c.Writer().Header().Add("Vary", "Access-Control-Request-Headers")

		origin := c.Request().Header.Get("Origin")
		if origin == "" {
			return c.Next()
		}

		allowOrigin := ""
		if config.AllowOrigins[0] == "*" && !config.AllowCredentials {
			allowOrigin = "*"
		} else if allowOriginsMap[origin] {
			allowOrigin = origin
		} else if allowOriginsMap["*"] {
			allowOrigin = origin
		} else {
			return c.Next()
		}

		c.Writer().Header().Set("Access-Control-Allow-Origin", allowOrigin)

		if config.AllowCredentials {
			c.Writer().Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if c.Method() == http.MethodOptions {
			requestMethod := c.Request().Header.Get("Access-Control-Request-Method")
			if requestMethod != "" {
				c.Writer().Header().Set("Access-Control-Allow-Methods", allowMethodsStr)

				requestHeaders := c.Request().Header.Get("Access-Control-Request-Headers")
				if requestHeaders != "" {
					c.Writer().Header().Set("Access-Control-Allow-Headers", allowHeadersStr)
				}

				if config.MaxAge > 0 {
					c.Writer().Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
				}

				return c.Status(http.StatusNoContent).Send("")
			}
		}

		if len(config.ExposeHeaders) > 0 {
			c.Writer().Header().Set("Access-Control-Expose-Headers", exposeHeadersStr)
		}

		return c.Next()
	}
}
