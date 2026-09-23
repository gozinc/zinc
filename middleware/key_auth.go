// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/textproto"
	"strings"

	"github.com/0mjs/zinc"
)

var (
	// Key authentication errors distinguish missing from rejected credentials.
	ErrKeyAuthKeyMissing = errors.New("zinckeyauth: key missing")
	ErrKeyAuthKeyInvalid = errors.New("zinckeyauth: key invalid")
)

// KeyAuthSource identifies where a key was extracted.
type KeyAuthSource string

const (
	KeyAuthSourceAuthorizationHeader KeyAuthSource = "authorization_header"
	KeyAuthSourceHeader              KeyAuthSource = "header"
	KeyAuthSourceQuery               KeyAuthSource = "query"
	KeyAuthSourceCookie              KeyAuthSource = "cookie"
)

// KeyAuthCredentials contains a key supplied for validation.
type KeyAuthCredentials struct {
	Key    string
	Source KeyAuthSource
}

// KeyAuthState is the authenticated key state stored on the context.
type KeyAuthState struct {
	Key    string
	Source KeyAuthSource
}

// KeyAuthExtractor reads key credentials from a request.
type KeyAuthExtractor func(*zinc.Context) (KeyAuthCredentials, error)

// KeyAuthValidator verifies extracted key credentials.
type KeyAuthValidator func(*zinc.Context, KeyAuthCredentials) (bool, error)

// KeyAuthErrorHandler maps extraction and validation failures.
type KeyAuthErrorHandler func(*zinc.Context, error) error

// KeyAuthConfig controls key extraction, validation, and failure handling.
type KeyAuthConfig struct {
	Skipper        func(*zinc.Context) bool
	Extractor      KeyAuthExtractor
	Validator      KeyAuthValidator
	SuccessHandler zinc.RouteHandler
	ErrorHandler   KeyAuthErrorHandler
}

type keyAuthContextKey int

const keyAuthStateContextKey keyAuthContextKey = iota

// DefaultKeyAuthConfig extracts Bearer keys from Authorization.
func DefaultKeyAuthConfig() KeyAuthConfig {
	return KeyAuthConfig{
		Extractor: KeyAuthFromAuthorizationHeader(),
		SuccessHandler: func(c *zinc.Context) error {
			return c.Next()
		},
	}
}

// KeyAuth authenticates requests with validator.
func KeyAuth(validator KeyAuthValidator) zinc.Middleware {
	return KeyAuthWithConfig(KeyAuthConfig{Validator: validator})
}

// KeyAuthWithConfig authenticates a request and publishes state only after
// successful validation.
func KeyAuthWithConfig(config KeyAuthConfig) zinc.Middleware {
	cfg := resolveKeyAuthConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		credentials, err := cfg.Extractor(c)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}

		ok, err := cfg.Validator(c, credentials)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}
		if !ok {
			return cfg.ErrorHandler(c, ErrKeyAuthKeyInvalid)
		}

		c.Set(keyAuthStateContextKey, KeyAuthState{
			Key:    credentials.Key,
			Source: credentials.Source,
		})
		return cfg.SuccessHandler(c)
	}
}

// KeyAuthFromAuthorizationHeader extracts a Bearer key.
func KeyAuthFromAuthorizationHeader() KeyAuthExtractor {
	return keyAuthFromHeader(zinc.HeaderAuthorization, "Bearer ", KeyAuthSourceAuthorizationHeader)
}

// KeyAuthFromHeader extracts an unprefixed key from header.
func KeyAuthFromHeader(header string) KeyAuthExtractor {
	return keyAuthFromHeader(header, "", KeyAuthSourceHeader)
}

// KeyAuthFromHeaderPrefix extracts a key after a case-insensitive prefix.
func KeyAuthFromHeaderPrefix(header, prefix string) KeyAuthExtractor {
	return keyAuthFromHeader(header, prefix, KeyAuthSourceHeader)
}

// KeyAuthFromQuery extracts a key from a query parameter.
func KeyAuthFromQuery(name string) KeyAuthExtractor {
	return func(c *zinc.Context) (KeyAuthCredentials, error) {
		key := strings.TrimSpace(c.Query(name))
		if key == "" {
			return KeyAuthCredentials{}, fmt.Errorf("%w: %s query value", ErrKeyAuthKeyMissing, name)
		}
		return KeyAuthCredentials{Key: key, Source: KeyAuthSourceQuery}, nil
	}
}

// KeyAuthFromCookie extracts a key from a cookie.
func KeyAuthFromCookie(name string) KeyAuthExtractor {
	return func(c *zinc.Context) (KeyAuthCredentials, error) {
		cookie, err := c.Cookie(name)
		if err != nil {
			return KeyAuthCredentials{}, fmt.Errorf("%w: %s cookie", ErrKeyAuthKeyMissing, name)
		}
		key := strings.TrimSpace(cookie.Value)
		if key == "" {
			return KeyAuthCredentials{}, fmt.Errorf("%w: %s cookie", ErrKeyAuthKeyMissing, name)
		}
		return KeyAuthCredentials{Key: key, Source: KeyAuthSourceCookie}, nil
	}
}

// KeyAuthFromFirst falls back only for missing credentials.
func KeyAuthFromFirst(extractors ...KeyAuthExtractor) KeyAuthExtractor {
	list := append([]KeyAuthExtractor(nil), extractors...)
	return func(c *zinc.Context) (KeyAuthCredentials, error) {
		var lastMissing error
		for _, extractor := range list {
			if extractor == nil {
				continue
			}
			credentials, err := extractor(c)
			if err == nil {
				return credentials, nil
			}
			if errors.Is(err, ErrKeyAuthKeyMissing) {
				lastMissing = err
				continue
			}
			return KeyAuthCredentials{}, err
		}
		if lastMissing != nil {
			return KeyAuthCredentials{}, lastMissing
		}
		return KeyAuthCredentials{}, ErrKeyAuthKeyMissing
	}
}

// KeyAuthStatic returns a constant-time validator for one key.
func KeyAuthStatic(key string) KeyAuthValidator {
	return KeyAuthStaticKeys(key)
}

// KeyAuthStaticKeys returns a constant-time validator for a fixed key set.
func KeyAuthStaticKeys(keys ...string) KeyAuthValidator {
	hashes := make([][32]byte, len(keys))
	for i, key := range keys {
		hashes[i] = sha256.Sum256([]byte(key))
	}
	return func(_ *zinc.Context, credentials KeyAuthCredentials) (bool, error) {
		keyHash := sha256.Sum256([]byte(credentials.Key))
		match := 0
		for _, hash := range hashes {
			match |= subtle.ConstantTimeCompare(keyHash[:], hash[:])
		}
		return match == 1, nil
	}
}

// KeyAuthCurrent returns authenticated key state.
func KeyAuthCurrent(c *zinc.Context) (KeyAuthState, bool) {
	if c == nil {
		return KeyAuthState{}, false
	}
	value, ok := c.Get(keyAuthStateContextKey)
	if !ok {
		return KeyAuthState{}, false
	}
	state, ok := value.(KeyAuthState)
	return state, ok
}

// MustKeyAuthCurrent returns key state or panics when absent.
func MustKeyAuthCurrent(c *zinc.Context) KeyAuthState {
	state, ok := KeyAuthCurrent(c)
	if !ok {
		panic("zinckeyauth: state not found")
	}
	return state
}

func resolveKeyAuthConfig(config KeyAuthConfig) KeyAuthConfig {
	cfg := DefaultKeyAuthConfig()
	cfg.Skipper = config.Skipper
	if config.Extractor != nil {
		cfg.Extractor = config.Extractor
	}
	if config.Validator == nil {
		panic("zinckeyauth: Validator is required")
	}
	cfg.Validator = config.Validator
	if config.SuccessHandler != nil {
		cfg.SuccessHandler = config.SuccessHandler
	}
	if config.ErrorHandler != nil {
		cfg.ErrorHandler = config.ErrorHandler
	} else {
		cfg.ErrorHandler = keyAuthDefaultErrorHandler
	}
	return cfg
}

func keyAuthDefaultErrorHandler(c *zinc.Context, err error) error {
	if errors.Is(err, ErrKeyAuthKeyMissing) || errors.Is(err, ErrKeyAuthKeyInvalid) {
		c.SetHeader(zinc.HeaderWWWAuthenticate, "Bearer")
		return zinc.ErrUnauthorized
	}
	var httpErr *zinc.HTTPError
	if errors.As(err, &httpErr) {
		return err
	}
	return err
}

func keyAuthFromHeader(header, prefix string, source KeyAuthSource) KeyAuthExtractor {
	header = textproto.CanonicalMIMEHeaderKey(header)
	return func(c *zinc.Context) (KeyAuthCredentials, error) {
		if c == nil || c.Request() == nil {
			return KeyAuthCredentials{}, fmt.Errorf("%w: %s header", ErrKeyAuthKeyMissing, header)
		}

		for _, value := range c.Request().Header.Values(header) {
			raw := strings.TrimSpace(value)
			if raw == "" {
				continue
			}
			if prefix != "" {
				if len(raw) <= len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
					continue
				}
				raw = strings.TrimSpace(raw[len(prefix):])
			}
			if raw == "" {
				continue
			}
			return KeyAuthCredentials{Key: raw, Source: source}, nil
		}
		return KeyAuthCredentials{}, fmt.Errorf("%w: %s header", ErrKeyAuthKeyMissing, header)
	}
}
