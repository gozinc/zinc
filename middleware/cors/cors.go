// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package cors

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config defines origins, methods, and headers accepted across origins. The
// zero value allows common methods and headers from any origin without
// credentials.
type Config struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	// MaxAge is how long, in seconds, a browser may cache a preflight answer.
	MaxAge int
}

// defaultConfig allows common methods and headers from any origin without credentials.
func defaultConfig() Config {
	return Config{
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

// New applies CORS response headers and terminates valid preflight
// requests. Vary is always set to keep shared caches origin-safe.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("cors", configs)
	defaults := defaultConfig()
	if len(config.AllowOrigins) == 0 && !config.AllowCredentials {
		config.AllowOrigins = defaults.AllowOrigins
	}
	if len(config.AllowMethods) == 0 {
		config.AllowMethods = defaults.AllowMethods
	}
	if len(config.AllowHeaders) == 0 {
		config.AllowHeaders = defaults.AllowHeaders
	}
	if config.MaxAge < 0 {
		panic("zinc: CORS MaxAge must not be negative")
	}
	allowOriginsMap := make(map[string]bool, len(config.AllowOrigins))
	for _, origin := range config.AllowOrigins {
		if origin == "*" && config.AllowCredentials {
			panic("zinc: credentialed CORS requires explicit origins")
		}
		allowOriginsMap[origin] = true
	}

	allowMethodsStr := strings.Join(config.AllowMethods, ",")
	allowHeadersStr := strings.Join(config.AllowHeaders, ",")
	exposeHeadersStr := strings.Join(config.ExposeHeaders, ",")

	return func(c *zinc.Context) error {
		c.Writer().Header().Add("Vary", "Origin")
		c.Writer().Header().Add("Vary", "Access-Control-Request-Method")
		c.Writer().Header().Add("Vary", "Access-Control-Request-Headers")

		origin := c.Request().Header.Get("Origin")
		if origin == "" {
			return c.Next()
		}

		allowOrigin := ""
		if allowOriginsMap["*"] {
			allowOrigin = "*"
		} else if allowOriginsMap[origin] {
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
