// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

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
)

var (
	// Authentication errors are distinct so custom handlers can use errors.Is.
	ErrBasicAuthCredentialsMissing   = errors.New("zincbasicauth: credentials missing")
	ErrBasicAuthCredentialsMalformed = errors.New("zincbasicauth: credentials malformed")
	ErrBasicAuthCredentialsInvalid   = errors.New("zincbasicauth: credentials invalid")
)

// BasicAuthSource identifies where credentials were extracted.
type BasicAuthSource string

const (
	BasicAuthSourceAuthorizationHeader BasicAuthSource = "authorization_header"
	BasicAuthSourceHeader              BasicAuthSource = "header"
)

const basicAuthDefaultRealm = "Restricted"

// BasicAuthCredentials contains credentials supplied for validation.
type BasicAuthCredentials struct {
	Username string
	Password string
	Source   BasicAuthSource
}

// BasicAuthIdentity is the authenticated identity stored on the context.
type BasicAuthIdentity struct {
	Username string
	Source   BasicAuthSource
}

// BasicAuthExtractor reads Basic credentials from a request.
type BasicAuthExtractor func(*zinc.Context) (BasicAuthCredentials, error)

// BasicAuthValidator verifies extracted credentials.
type BasicAuthValidator func(*zinc.Context, BasicAuthCredentials) (bool, error)

// BasicAuthErrorHandler maps extraction and validation failures to responses.
type BasicAuthErrorHandler func(*zinc.Context, error) error

// BasicAuthPair configures one static username and password.
type BasicAuthPair struct {
	Username string
	Password string
}

// BasicAuthConfig controls credential extraction, validation, and failure handling.
type BasicAuthConfig struct {
	Skipper        func(*zinc.Context) bool
	Extractor      BasicAuthExtractor
	Validator      BasicAuthValidator
	SuccessHandler zinc.RouteHandler
	ErrorHandler   BasicAuthErrorHandler
	Realm          string
}

type basicAuthContextKey int

const basicAuthIdentityContextKey basicAuthContextKey = iota

// DefaultBasicAuthConfig returns header-based Basic authentication defaults.
func DefaultBasicAuthConfig() BasicAuthConfig {
	return BasicAuthConfig{
		Extractor: BasicAuthFromAuthorizationHeader(),
		SuccessHandler: func(c *zinc.Context) error {
			return c.Next()
		},
		Realm: basicAuthDefaultRealm,
	}
}

// BasicAuth authenticates requests with validator.
func BasicAuth(validator BasicAuthValidator) zinc.Middleware {
	return BasicAuthWithConfig(BasicAuthConfig{Validator: validator})
}

// BasicAuthWithConfig authenticates requests using config.
func BasicAuthWithConfig(config BasicAuthConfig) zinc.Middleware {
	cfg := resolveBasicAuthConfig(config)

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
			return cfg.ErrorHandler(c, ErrBasicAuthCredentialsInvalid)
		}

		c.Set(basicAuthIdentityContextKey, BasicAuthIdentity{
			Username: credentials.Username,
			Source:   credentials.Source,
		})

		return cfg.SuccessHandler(c)
	}
}

// BasicAuthFromAuthorizationHeader extracts RFC 7617 credentials.
func BasicAuthFromAuthorizationHeader() BasicAuthExtractor {
	return basicAuthFromHeader(zinc.HeaderAuthorization, "Basic ", BasicAuthSourceAuthorizationHeader)
}

// BasicAuthFromHeader extracts an unprefixed Basic value from header.
func BasicAuthFromHeader(header string) BasicAuthExtractor {
	return basicAuthFromHeader(header, "", BasicAuthSourceHeader)
}

// BasicAuthFromHeaderPrefix extracts credentials after a case-insensitive prefix.
func BasicAuthFromHeaderPrefix(header, prefix string) BasicAuthExtractor {
	return basicAuthFromHeader(header, prefix, BasicAuthSourceHeader)
}

// BasicAuthFromFirst tries extractors until credentials are found; malformed
// credentials stop fallback so a bad stronger source cannot be bypassed.
func BasicAuthFromFirst(extractors ...BasicAuthExtractor) BasicAuthExtractor {
	list := append([]BasicAuthExtractor(nil), extractors...)

	return func(c *zinc.Context) (BasicAuthCredentials, error) {
		var lastMissing error
		for _, extractor := range list {
			if extractor == nil {
				continue
			}
			credentials, err := extractor(c)
			if err == nil {
				return credentials, nil
			}
			if errors.Is(err, ErrBasicAuthCredentialsMissing) {
				lastMissing = err
				continue
			}
			return BasicAuthCredentials{}, err
		}
		if lastMissing != nil {
			return BasicAuthCredentials{}, lastMissing
		}
		return BasicAuthCredentials{}, ErrBasicAuthCredentialsMissing
	}
}

// BasicAuthStatic returns a constant-time validator for one credential pair.
func BasicAuthStatic(username, password string) BasicAuthValidator {
	return BasicAuthStaticPairs(BasicAuthPair{Username: username, Password: password})
}

// BasicAuthStaticPairs returns a validator that compares every credential pair
// in constant time after hashing both username and password.
func BasicAuthStaticPairs(pairs ...BasicAuthPair) BasicAuthValidator {
	list := append([]BasicAuthPair(nil), pairs...)
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

	return func(_ *zinc.Context, credentials BasicAuthCredentials) (bool, error) {
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

// BasicAuthCurrent returns the authenticated identity for the current request.
func BasicAuthCurrent(c *zinc.Context) (BasicAuthIdentity, bool) {
	if c == nil {
		return BasicAuthIdentity{}, false
	}
	value, ok := c.Get(basicAuthIdentityContextKey)
	if !ok {
		return BasicAuthIdentity{}, false
	}
	identity, ok := value.(BasicAuthIdentity)
	return identity, ok
}

// MustBasicAuthCurrent returns the identity or panics when middleware did not set it.
func MustBasicAuthCurrent(c *zinc.Context) BasicAuthIdentity {
	identity, ok := BasicAuthCurrent(c)
	if !ok {
		panic("zincbasicauth: identity not found")
	}
	return identity
}

// BasicAuthUsername returns the authenticated username.
func BasicAuthUsername(c *zinc.Context) (string, bool) {
	identity, ok := BasicAuthCurrent(c)
	if !ok {
		return "", false
	}
	return identity.Username, true
}

// MustBasicAuthUsername returns the username or panics when it is absent.
func MustBasicAuthUsername(c *zinc.Context) string {
	username, ok := BasicAuthUsername(c)
	if !ok {
		panic("zincbasicauth: username not found")
	}
	return username
}

func resolveBasicAuthConfig(config BasicAuthConfig) BasicAuthConfig {
	cfg := DefaultBasicAuthConfig()

	cfg.Skipper = config.Skipper
	if config.Extractor != nil {
		cfg.Extractor = config.Extractor
	}
	if config.Validator == nil {
		panic("zincbasicauth: Validator is required")
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
	case errors.Is(err, ErrBasicAuthCredentialsMissing):
		return true
	case errors.Is(err, ErrBasicAuthCredentialsMalformed):
		return true
	case errors.Is(err, ErrBasicAuthCredentialsInvalid):
		return true
	}

	var httpErr *zinc.HTTPError
	return errors.As(err, &httpErr) && httpErr.Code == http.StatusUnauthorized
}

func basicAuthFromHeader(header, prefix string, source BasicAuthSource) BasicAuthExtractor {
	header = textproto.CanonicalMIMEHeaderKey(header)

	return func(c *zinc.Context) (BasicAuthCredentials, error) {
		if c == nil || c.Request() == nil {
			return BasicAuthCredentials{}, fmt.Errorf("%w: %s header", ErrBasicAuthCredentialsMissing, header)
		}

		values := c.Request().Header.Values(header)
		if len(values) == 0 {
			return BasicAuthCredentials{}, fmt.Errorf("%w: %s header", ErrBasicAuthCredentialsMissing, header)
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
					lastErr = fmt.Errorf("%w: %s header", ErrBasicAuthCredentialsMalformed, header)
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
			return BasicAuthCredentials{}, lastErr
		}
		return BasicAuthCredentials{}, fmt.Errorf("%w: %s header", ErrBasicAuthCredentialsMissing, header)
	}
}

func decodeBasicAuth(raw string, source BasicAuthSource) (BasicAuthCredentials, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return BasicAuthCredentials{}, fmt.Errorf("%w: decode basic credentials", ErrBasicAuthCredentialsMalformed)
	}

	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return BasicAuthCredentials{}, fmt.Errorf("%w: missing colon separator", ErrBasicAuthCredentialsMalformed)
	}

	return BasicAuthCredentials{
		Username: username,
		Password: password,
		Source:   source,
	}, nil
}
