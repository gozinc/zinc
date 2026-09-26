// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package requestid gives every request an identifier, reusing the one the
// client or an upstream proxy sent.
package requestid

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Generator creates an identifier when the request does not carry one.
type Generator func(*zinc.Context) (string, error)

// Config controls the header and identifier generation. The zero value uses
// X-Request-ID and random 128-bit identifiers.
type Config struct {
	// Header carries the identifier on the request and the response.
	Header string
	// Generate creates identifiers. Nil uses Random.
	Generate Generator
}

type contextKey struct{}

// New reuses the request's identifier or generates one, and mirrors it into
// both the request and the response headers.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("requestid", configs)
	header := config.Header
	if header == "" {
		header = zinc.HeaderXRequestID
	}
	generate := config.Generate
	if generate == nil {
		generate = Random
	}

	return func(c *zinc.Context) error {
		req := c.Request()
		id := ""
		if req != nil {
			id = req.Header.Get(header)
		}
		if id == "" {
			var err error
			if id, err = generate(c); err != nil {
				return err
			}
			if req != nil {
				req.Header.Set(header, id)
			}
		}
		c.SetHeader(header, id)
		c.Set(contextKey{}, id)
		return c.Next()
	}
}

// Get returns the request's identifier. Without the middleware, it falls back
// to the X-Request-ID request header.
func Get(c *zinc.Context) string {
	if c == nil {
		return ""
	}
	if id, ok := c.Get(contextKey{}); ok {
		return id.(string)
	}
	return c.Header(zinc.HeaderXRequestID)
}

// Random returns a 128-bit cryptographically random hexadecimal identifier.
func Random(*zinc.Context) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Static returns a generator that always answers id, for tests.
func Static(id string) Generator {
	return func(*zinc.Context) (string, error) {
		return id, nil
	}
}
