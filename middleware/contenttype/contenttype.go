// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package contenttype rejects request bodies of media types or content
// codings the application does not accept.
package contenttype

import (
	"mime"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config lists what requests may send. An empty list allows anything.
type Config struct {
	// Types are the accepted media types, such as "application/json".
	// Parameters are ignored, so "application/json; charset=utf-8" matches.
	Types []string
	// Encodings are the accepted content codings, such as "gzip". A request
	// without Content-Encoding is "identity", so list it to allow plain bodies.
	Encodings []string
}

// New answers 415 Unsupported Media Type for a request whose Content-Type or
// Content-Encoding is not listed in config.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("contenttype", configs)
	types := mediaTypeSet(config.Types)
	encodings := stringSet(config.Encodings)
	return func(c *zinc.Context) error {
		if len(types) > 0 {
			if _, ok := types[c.ContentType()]; !ok {
				return zinc.ErrUnsupportedMediaType
			}
		}
		if len(encodings) > 0 && !encodingsAllowed(c.Header(zinc.HeaderContentEncoding), encodings) {
			return zinc.ErrUnsupportedMediaType
		}
		return c.Next()
	}
}

func encodingsAllowed(raw string, allowed map[string]struct{}) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "identity"
	}
	for part := range strings.SplitSeq(raw, ",") {
		encoding := strings.ToLower(strings.TrimSpace(part))
		if encoding == "" {
			continue
		}
		if _, ok := allowed[encoding]; !ok {
			return false
		}
	}
	return true
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
