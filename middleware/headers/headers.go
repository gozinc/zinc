// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package headers sets response headers and selects middleware by request
// header.
package headers

import (
	"net/http"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Route runs Middleware when the request carries Header, and its value is
// Value when Value is not empty.
type Route struct {
	Header     string
	Value      string
	Middleware zinc.Middleware
}

// Config lists the response headers to set and the header routes to apply.
type Config struct {
	// Set holds response headers written before downstream handlers run.
	Set map[string]string
	// Routes are checked in order. The first match runs in place of the rest
	// of the chain; that middleware decides whether to call Next.
	Routes []Route
}

// New sets the configured response headers, then applies the first matching
// header route.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("headers", configs)
	set := make([][2]string, 0, len(config.Set))
	for key, value := range config.Set {
		set = append(set, [2]string{http.CanonicalHeaderKey(key), value})
	}
	routes := make([]Route, 0, len(config.Routes))
	for _, route := range config.Routes {
		if route.Header == "" || route.Middleware == nil {
			panic("headers: a Route needs a Header and a Middleware")
		}
		routes = append(routes, route)
	}

	return func(c *zinc.Context) error {
		for _, kv := range set {
			c.SetHeader(kv[0], kv[1])
		}
		for _, route := range routes {
			value := c.Header(route.Header)
			if value != "" && (route.Value == "" || value == route.Value) {
				return route.Middleware(c)
			}
		}
		return c.Next()
	}
}
