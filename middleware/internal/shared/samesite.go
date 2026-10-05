// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package shared

import (
	"net/url"
	"strings"
)

// SameSitePath makes an escaped request path safe to send as a redirect
// target: the result starts with exactly one slash, so a browser resolves it
// on this site, and backslashes, spaces and control characters are escaped.
// A path starting "//" or "/\" would otherwise name another host.
func SameSitePath(path string) string {
	if len(path) > 0 && path[0] == '/' && (len(path) == 1 || path[1] != '/') && !needsEscape(path) {
		return path
	}
	rest := strings.TrimLeft(path, "/")
	var b strings.Builder
	b.Grow(len(rest) + 1)
	b.WriteByte('/')
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if escapeInRedirect(c) {
			const hex = "0123456789ABCDEF"
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func needsEscape(s string) bool {
	for i := 0; i < len(s); i++ {
		if escapeInRedirect(s[i]) {
			return true
		}
	}
	return false
}

func escapeInRedirect(c byte) bool {
	return c <= ' ' || c == 0x7f || c == '\\'
}

// EscapedTail returns the escaped form of the last n bytes of u.Path, so a
// redirect built from it keeps encoded characters such as %2F encoded.
func EscapedTail(u *url.URL, n int) string {
	escaped := u.EscapedPath()
	skip := len(u.Path) - n
	i := 0
	for ; skip > 0 && i < len(escaped); skip-- {
		if escaped[i] == '%' {
			i += 3
		} else {
			i++
		}
	}
	if i >= len(escaped) {
		return ""
	}
	return escaped[i:]
}

// ExternalTarget reports whether a rule's target names a scheme or host, so
// it is meant to leave the site. Decide this on the configured target, before
// any part of the request path is put into it.
func ExternalTarget(to string) bool {
	if strings.HasPrefix(to, "//") {
		return true
	}
	for i := 0; i < len(to); i++ {
		c := to[i]
		switch {
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
		case '0' <= c && c <= '9' || c == '+' || c == '-' || c == '.':
			if i == 0 {
				return false
			}
		case c == ':':
			return i > 0
		default:
			return false
		}
	}
	return false
}
