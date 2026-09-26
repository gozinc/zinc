// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package basicauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

var (
	// Authentication errors are distinct so custom handlers can use errors.Is.
	ErrCredentialsMissing   = errors.New("basicauth: credentials missing")
	ErrCredentialsMalformed = errors.New("basicauth: credentials malformed")
	ErrCredentialsInvalid   = errors.New("basicauth: credentials invalid")
)

// Source identifies where credentials were extracted.
type Source string

const (
	SourceAuthorizationHeader Source = "authorization_header"
	SourceHeader              Source = "header"
)

const basicAuthDefaultRealm = "Restricted"

// Credentials contains credentials supplied for validation.
type Credentials struct {
	Username string
	Password string
	Source   Source
}

// Identity is the authenticated identity stored on the context.
type Identity struct {
	Username string
	Source   Source
}

// Extractor reads Basic credentials from a request.
type Extractor func(*zinc.Context) (Credentials, error)

// Validator verifies extracted credentials.
type Validator func(*zinc.Context, Credentials) (bool, error)

// Pair configures one static username and password.
type Pair struct {
	Username string
	Password string
}

// Config controls credential extraction, validation, and failure handling.
type Config struct {
	Extractor      Extractor
	Validator      Validator
	SuccessHandler zinc.HandlerFunc
	ErrorHandler   func(*zinc.Context, error) error
	Realm          string
}

type basicAuthContextKey int

const basicAuthIdentityContextKey basicAuthContextKey = iota

// defaultConfig returns header-based Basic authentication defaults.
func defaultConfig() Config {
	return Config{
		Extractor: FromAuthorizationHeader(),
		SuccessHandler: func(c *zinc.Context) error {
			return c.Next()
		},
		Realm: basicAuthDefaultRealm,
	}
}

// New authenticates requests with HTTP Basic credentials. Config.Validator
// is required; a failure answers 401 with a WWW-Authenticate challenge.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("basicauth", configs)
	cfg := resolveBasicAuthConfig(config)

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
			return cfg.ErrorHandler(c, ErrCredentialsInvalid)
		}

		c.Set(basicAuthIdentityContextKey, Identity{
			Username: credentials.Username,
			Source:   credentials.Source,
		})

		return cfg.SuccessHandler(c)
	}
}

// FromAuthorizationHeader extracts RFC 7617 credentials.
func FromAuthorizationHeader() Extractor {
	return basicAuthFromHeader(zinc.HeaderAuthorization, "Basic ", SourceAuthorizationHeader)
}

// FromHeader extracts an unprefixed Basic value from header.
func FromHeader(header string) Extractor {
	return basicAuthFromHeader(header, "", SourceHeader)
}

// FromHeaderPrefix extracts credentials after a case-insensitive prefix.
func FromHeaderPrefix(header, prefix string) Extractor {
	return basicAuthFromHeader(header, prefix, SourceHeader)
}

// FromFirst tries extractors until credentials are found; malformed
// credentials stop fallback so a bad stronger source cannot be bypassed.
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
			if errors.Is(err, ErrCredentialsMissing) {
				lastMissing = err
				continue
			}
			return Credentials{}, err
		}
		if lastMissing != nil {
			return Credentials{}, lastMissing
		}
		return Credentials{}, ErrCredentialsMissing
	}
}

// Static returns a constant-time validator for one credential pair.
func Static(username, password string) Validator {
	return StaticPairs(Pair{Username: username, Password: password})
}

// StaticPairs returns a validator that compares every credential pair
// in constant time after hashing both username and password.
func StaticPairs(pairs ...Pair) Validator {
	list := append([]Pair(nil), pairs...)
	type hashedPair struct {
		username [32]byte
		password [32]byte
	}
	hashed := make([]hashedPair, len(list))
	for i, pair := range list {
		hashed[i] = hashedPair{
			username: sha256.Sum256([]byte(pair.Username)),
			password: sha256.Sum256([]byte(pair.Password)),
		}
	}

	return func(_ *zinc.Context, credentials Credentials) (bool, error) {
		usernameHash := sha256.Sum256([]byte(credentials.Username))
		passwordHash := sha256.Sum256([]byte(credentials.Password))

		match := 0
		for _, pair := range hashed {
			userMatch := subtle.ConstantTimeCompare(usernameHash[:], pair.username[:])
			passwordMatch := subtle.ConstantTimeCompare(passwordHash[:], pair.password[:])
			match |= userMatch & passwordMatch
		}
		return match == 1, nil
	}
}

// Get returns the authenticated identity for the current request.
func Get(c *zinc.Context) (Identity, bool) {
	if c == nil {
		return Identity{}, false
	}
	value, ok := c.Get(basicAuthIdentityContextKey)
	if !ok {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok
}

// MustGet returns the identity or panics when middleware did not set it.
func MustGet(c *zinc.Context) Identity {
	identity, ok := Get(c)
	if !ok {
		panic("basicauth: identity not found")
	}
	return identity
}

func resolveBasicAuthConfig(config Config) Config {
	cfg := defaultConfig()

	if config.Extractor != nil {
		cfg.Extractor = config.Extractor
	}
	if config.Validator == nil {
		panic("basicauth: Validator is required")
	}
	cfg.Validator = config.Validator
	if config.SuccessHandler != nil {
		cfg.SuccessHandler = config.SuccessHandler
	}
	if config.Realm != "" {
		cfg.Realm = config.Realm
	}

	if config.ErrorHandler != nil {
		cfg.ErrorHandler = config.ErrorHandler
	} else {
		cfg.ErrorHandler = func(c *zinc.Context, err error) error {
			return basicAuthDefaultErrorHandler(c, cfg.Realm, err)
		}
	}

	return cfg
}

func basicAuthDefaultErrorHandler(c *zinc.Context, realm string, err error) error {
	if !basicAuthShouldChallenge(err) {
		return err
	}

	if realm == "" {
		realm = basicAuthDefaultRealm
	}
	c.SetHeader(zinc.HeaderWWWAuthenticate, "Basic realm="+strconv.Quote(realm))

	var httpErr *zinc.HTTPError
	if errors.As(err, &httpErr) {
		return err
	}
	return zinc.ErrUnauthorized
}

func basicAuthShouldChallenge(err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, ErrCredentialsMissing):
		return true
	case errors.Is(err, ErrCredentialsMalformed):
		return true
	case errors.Is(err, ErrCredentialsInvalid):
		return true
	}

	var httpErr *zinc.HTTPError
	return errors.As(err, &httpErr) && httpErr.Code == http.StatusUnauthorized
}

func basicAuthFromHeader(header, prefix string, source Source) Extractor {
	header = textproto.CanonicalMIMEHeaderKey(header)

	return func(c *zinc.Context) (Credentials, error) {
		if c == nil || c.Request() == nil {
			return Credentials{}, fmt.Errorf("%w: %s header", ErrCredentialsMissing, header)
		}

		values := c.Request().Header.Values(header)
		if len(values) == 0 {
			return Credentials{}, fmt.Errorf("%w: %s header", ErrCredentialsMissing, header)
		}

		var lastErr error
		for _, value := range values {
			raw := strings.TrimSpace(value)
			if raw == "" {
				continue
			}

			if prefix != "" {
				if len(raw) <= len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
					continue
				}
				raw = strings.TrimSpace(raw[len(prefix):])
				if raw == "" {
					lastErr = fmt.Errorf("%w: %s header", ErrCredentialsMalformed, header)
					continue
				}
			}

			credentials, err := decodeBasicAuth(raw, source)
			if err == nil {
				return credentials, nil
			}
			lastErr = fmt.Errorf("%w: %s header", err, header)
		}

		if lastErr != nil {
			return Credentials{}, lastErr
		}
		return Credentials{}, fmt.Errorf("%w: %s header", ErrCredentialsMissing, header)
	}
}

func decodeBasicAuth(raw string, source Source) (Credentials, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return Credentials{}, fmt.Errorf("%w: decode basic credentials", ErrCredentialsMalformed)
	}

	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return Credentials{}, fmt.Errorf("%w: missing colon separator", ErrCredentialsMalformed)
	}

	return Credentials{
		Username: username,
		Password: password,
		Source:   source,
	}, nil
}
