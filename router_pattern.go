// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	legacyParamIdentifier    = ':'
	legacyWildcardIdentifier = '*'
)

// collectedRouteParams keeps the ordinary one- or two-parameter route inline.
// Registration allocates extra storage only for wider patterns.
type collectedRouteParams struct {
	count  int
	inline [2]string
	extra  []string
}

var pathBuilderPool = sync.Pool{
	New: func() any {
		return new(strings.Builder)
	},
}

// normalizePath applies the sole unconditional normalization: every registered
// route begins with '/'. Case and trailing-slash policy are handled separately.
func (r *routeTable) normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if path[0] == '/' {
		return path
	}
	builder := pathBuilderPool.Get().(*strings.Builder)
	builder.Reset()
	builder.WriteByte('/')
	builder.WriteString(path)
	result := builder.String()
	pathBuilderPool.Put(builder)
	return result
}

// rejectLegacyRoutePattern reports a segment that starts with ':' or '*' with
// the {…} form to use. That is Gin's, Echo's and httprouter's parameter and
// wildcard syntax (and was Zinc 0.1's), so a literal there would be a route
// that silently never matches. Anywhere else in a segment, ':' and '*' are literal,
// as in /v1/users:batch or /times/12:00.
func rejectLegacyRoutePattern(path string) error {
	if !strings.ContainsAny(path, ":*") {
		return nil
	}

	for start := 0; start < len(path); {
		end := start
		for end < len(path) && path[end] != '/' {
			end++
		}
		if end > start {
			switch path[start] {
			case legacyParamIdentifier:
				return fmt.Errorf("legacy route parameter %q in path %q: use {name} syntax", path[start:end], path)
			case legacyWildcardIdentifier:
				return fmt.Errorf("legacy route wildcard %q in path %q: use {name...} syntax", path[start:end], path)
			}
		}
		start = end + 1
	}
	return nil
}

// collectBraceRouteParams validates Zinc's complete dynamic grammar:
// {name} consumes one non-empty segment and {name...} consumes the final suffix.
// Wildcards must occupy a complete segment and parameter names must be unique.
func collectBraceRouteParams(path string) (collectedRouteParams, error) {
	var names collectedRouteParams

	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '}':
			return names, fmt.Errorf("unmatched closing brace in route path %q", path)
		case '{':
			if i == 0 || path[i-1] != '/' {
				return names, fmt.Errorf("route parameter must occupy a complete segment in path %q", path)
			}
			endOffset := strings.IndexByte(path[i+1:], '}')
			if endOffset < 0 {
				return names, fmt.Errorf("unclosed route parameter in path %q", path)
			}
			end := i + 1 + endOffset
			if end+1 < len(path) && path[end+1] != '/' {
				return names, fmt.Errorf("route parameter must occupy a complete segment in path %q", path)
			}

			rawName := path[i+1 : end]
			catchAll := strings.HasSuffix(rawName, "...")
			name := strings.TrimSuffix(rawName, "...")
			if !validRouteParamName(name) {
				return names, fmt.Errorf("invalid route parameter %q in path %q", name, path)
			}
			if names.contains(name) {
				return names, fmt.Errorf("duplicate route parameter %q in path %q", name, path)
			}
			names.add(name)
			if catchAll && end != len(path)-1 {
				return names, fmt.Errorf("route wildcard %q must be final in path %q", name, path)
			}
			i = end
		}
	}
	return names, nil
}

// validRouteParamName accepts Unicode letters and digits plus underscore, but a
// digit cannot begin a name. This keeps names usable with Request.PathValue.
func validRouteParamName(name string) bool {
	if name == "" {
		return false
	}
	for index, r := range name {
		if index == 0 && unicode.IsDigit(r) {
			return false
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func (c *collectedRouteParams) add(name string) {
	if c.count < len(c.inline) {
		c.inline[c.count] = name
		c.count++
		return
	}
	c.extra = append(c.extra, name)
	c.count++
}

func (c *collectedRouteParams) contains(name string) bool {
	inlineCount := c.count
	if inlineCount > len(c.inline) {
		inlineCount = len(c.inline)
	}
	for i := 0; i < inlineCount; i++ {
		if c.inline[i] == name {
			return true
		}
	}
	for _, existing := range c.extra {
		if existing == name {
			return true
		}
	}
	return false
}

func (c collectedRouteParams) slice() []string {
	if c.count == 0 {
		return nil
	}
	out := make([]string, 0, c.count)
	inlineCount := c.count
	if inlineCount > len(c.inline) {
		inlineCount = len(c.inline)
	}
	out = append(out, c.inline[:inlineCount]...)
	if len(c.extra) > 0 {
		out = append(out, c.extra...)
	}
	return out
}

// lowercasePath avoids strings.ToLower for already-normalized ASCII paths.
// The boolean tells callers whether a second lookup can produce a new result.
func lowercasePath(path string) (string, bool) {
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c >= 'A' && c <= 'Z' {
			return strings.ToLower(path), true
		}
		if c >= 0x80 {
			lower := strings.ToLower(path)
			return lower, lower != path
		}
	}
	return path, false
}

// asciiFoldKind reports whether path has ASCII capital letters, and whether it
// is all ASCII. Folding ASCII preserves byte offsets, so an ASCII path can be
// matched case-insensitively in place, without a lowercase copy.
func asciiFoldKind(path string) (hasUpper, ascii bool) {
	// Eight bytes at a time: for bytes below 0x80, adding 0x3f sets a byte's
	// high bit when it is at least 'A', and adding 0x25 when it is past 'Z',
	// with no carry between bytes. The byte-wise loads combine into one load.
	const high = 0x8080808080808080
	var upper uint64
	i := 0
	for ; i+8 <= len(path); i += 8 {
		w := uint64(path[i]) | uint64(path[i+1])<<8 | uint64(path[i+2])<<16 | uint64(path[i+3])<<24 |
			uint64(path[i+4])<<32 | uint64(path[i+5])<<40 | uint64(path[i+6])<<48 | uint64(path[i+7])<<56
		if w&high != 0 {
			return false, false
		}
		upper |= (w + 0x3f3f3f3f3f3f3f3f) &^ (w + 0x2525252525252525) & high
	}
	hasUpper = upper != 0
	for ; i < len(path); i++ {
		c := path[i]
		if c >= utf8.RuneSelf {
			return hasUpper, false
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
	}
	return hasUpper, true
}

// foldByte lowercases an ASCII capital letter.
func foldByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// hasFoldedPrefix reports whether path starts with prefix, a lowercase label,
// ignoring ASCII case in path.
func hasFoldedPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if foldByte(path[i]) != prefix[i] {
			return false
		}
	}
	return true
}
