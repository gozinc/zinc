// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package keyauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/textproto"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

var (
	// Key authentication errors distinguish missing from rejected credentials.
	ErrKeyMissing = errors.New("keyauth: key missing")
	ErrKeyInvalid = errors.New("keyauth: key invalid")
)

// Source identifies where a key was extracted.
type Source string

const (
	SourceAuthorizationHeader Source = "authorization_header"
	SourceHeader              Source = "header"
	SourceQuery               Source = "query"
	SourceCookie              Source = "cookie"
)

// Credentials contains a key supplied for validation.
type Credentials struct {
	Key    string
	Source Source
}

// State is the authenticated key state stored on the context.
type State struct {
	Key    string
	Source Source
}

// Extractor reads key credentials from a request.
type Extractor func(*zinc.Context) (Credentials, error)

// Validator verifies extracted key credentials.
type Validator func(*zinc.Context, Credentials) (bool, error)

// Config controls key extraction, validation, and failure handling.
type Config struct {
	Extractor      Extractor
	Validator      Validator
	SuccessHandler zinc.HandlerFunc
	ErrorHandler   func(*zinc.Context, error) error
}

type keyAuthContextKey int

const keyAuthStateContextKey keyAuthContextKey = iota

// defaultConfig extracts Bearer keys from Authorization.
func defaultConfig() Config {
	return Config{
		Extractor: FromAuthorizationHeader(),
		SuccessHandler: func(c *zinc.Context) error {
			return c.Next()
		},
	}
}

// New authenticates requests with an API key. Config.Validator is required;
// state is published only after successful validation.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("keyauth", configs)
	cfg := resolveKeyAuthConfig(config)

	return func(c *zinc.Context) error {
		credentials, err := cfg.Extractor(c)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}

		ok, err := cfg.Validator(c, credentials)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}
		if !ok {
			return cfg.ErrorHandler(c, ErrKeyInvalid)
		}

		c.Set(keyAuthStateContextKey, State(credentials))
		return cfg.SuccessHandler(c)
	}
}

// FromAuthorizationHeader extracts a Bearer key.
func FromAuthorizationHeader() Extractor {
	return keyAuthFromHeader(zinc.HeaderAuthorization, "Bearer ", SourceAuthorizationHeader)
}

// FromHeader extracts an unprefixed key from header.
func FromHeader(header string) Extractor {
	return keyAuthFromHeader(header, "", SourceHeader)
}

// FromHeaderPrefix extracts a key after a case-insensitive prefix.
func FromHeaderPrefix(header, prefix string) Extractor {
	return keyAuthFromHeader(header, prefix, SourceHeader)
}

// FromQuery extracts a key from a query parameter.
func FromQuery(name string) Extractor {
	return func(c *zinc.Context) (Credentials, error) {
		key := strings.TrimSpace(c.Query(name))
		if key == "" {
			return Credentials{}, fmt.Errorf("%w: %s query value", ErrKeyMissing, name)
		}
		return Credentials{Key: key, Source: SourceQuery}, nil
	}
}

// FromCookie extracts a key from a cookie.
func FromCookie(name string) Extractor {
	return func(c *zinc.Context) (Credentials, error) {
		cookie, err := c.Cookie(name)
		if err != nil {
			return Credentials{}, fmt.Errorf("%w: %s cookie", ErrKeyMissing, name)
		}
		key := strings.TrimSpace(cookie.Value)
		if key == "" {
			return Credentials{}, fmt.Errorf("%w: %s cookie", ErrKeyMissing, name)
		}
		return Credentials{Key: key, Source: SourceCookie}, nil
	}
}

// FromFirst falls back only for missing credentials.
func FromFirst(extractors ...Extractor) Extractor {
	list := append([]Extractor(nil), extractors...)
	return func(c *zinc.Context) (Credentials, error) {
		var lastMissing error
		for _, extractor := range list {
			if extractor == nil {
				continue
			}
			credentials, err := extractor(c)
			if err == nil {
				return credentials, nil
			}
			if errors.Is(err, ErrKeyMissing) {
				lastMissing = err
				continue
			}
			return Credentials{}, err
		}
		if lastMissing != nil {
			return Credentials{}, lastMissing
		}
		return Credentials{}, ErrKeyMissing
	}
}

// Static returns a constant-time validator for one key.
func Static(key string) Validator {
	return StaticKeys(key)
}

// StaticKeys returns a constant-time validator for a fixed key set.
func StaticKeys(keys ...string) Validator {
	hashes := make([][32]byte, len(keys))
	for i, key := range keys {
		hashes[i] = sha256.Sum256([]byte(key))
	}
	return func(_ *zinc.Context, credentials Credentials) (bool, error) {
		keyHash := sha256.Sum256([]byte(credentials.Key))
		match := 0
		for _, hash := range hashes {
			match |= subtle.ConstantTimeCompare(keyHash[:], hash[:])
		}
		return match == 1, nil
	}
}

// Get returns authenticated key state.
func Get(c *zinc.Context) (State, bool) {
	if c == nil {
		return State{}, false
	}
	value, ok := c.Get(keyAuthStateContextKey)
	if !ok {
		return State{}, false
	}
	state, ok := value.(State)
	return state, ok
}

// MustGet returns key state or panics when absent.
func MustGet(c *zinc.Context) State {
	state, ok := Get(c)
	if !ok {
		panic("keyauth: state not found")
	}
	return state
}

func resolveKeyAuthConfig(config Config) Config {
	cfg := defaultConfig()
	if config.Extractor != nil {
		cfg.Extractor = config.Extractor
	}
	if config.Validator == nil {
		panic("keyauth: Validator is required")
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
	if errors.Is(err, ErrKeyMissing) || errors.Is(err, ErrKeyInvalid) {
		c.SetHeader(zinc.HeaderWWWAuthenticate, "Bearer")
		return zinc.ErrUnauthorized
	}
	var httpErr *zinc.HTTPError
	if errors.As(err, &httpErr) {
		return err
	}
	return err
}

func keyAuthFromHeader(header, prefix string, source Source) Extractor {
	header = textproto.CanonicalMIMEHeaderKey(header)
	return func(c *zinc.Context) (Credentials, error) {
		if c == nil || c.Request() == nil {
			return Credentials{}, fmt.Errorf("%w: %s header", ErrKeyMissing, header)
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
			return Credentials{Key: raw, Source: source}, nil
		}
		return Credentials{}, fmt.Errorf("%w: %s header", ErrKeyMissing, header)
	}
}
