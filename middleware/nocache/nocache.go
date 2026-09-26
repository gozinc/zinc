// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package nocache tells browsers and intermediaries not to cache responses.
package nocache

import "github.com/0mjs/zinc"

// New sets Cache-Control, Pragma, and Expires so that no cache stores or
// reuses the response.
func New() zinc.Middleware {
	return func(c *zinc.Context) error {
		c.SetHeader(zinc.HeaderCacheControl, "no-cache, no-store, max-age=0, must-revalidate")
		c.SetHeader(zinc.HeaderPragma, "no-cache")
		c.SetHeader(zinc.HeaderExpires, "0")
		return c.Next()
	}
}
