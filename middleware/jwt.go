// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/0mjs/zinc"
	jwtgo "github.com/golang-jwt/jwt/v5"
)

var (
	// JWT errors separate absent, malformed, and cryptographically invalid input.
	ErrJWTTokenMissing   = errors.New("zincjwt: token missing")
	ErrJWTTokenMalformed = errors.New("zincjwt: token malformed")
	ErrJWTTokenInvalid   = errors.New("zincjwt: token invalid")
)

// JWTExtractor retrieves a serialized token from a request.
type JWTExtractor func(*zinc.Context) (string, error)

// JWTKeyFunc resolves the verification key for a parsed token.
type JWTKeyFunc func(*zinc.Context, *jwtgo.Token) (any, error)

// JWTParseTokenFunc parses and cryptographically verifies a serialized token.
type JWTParseTokenFunc func(*zinc.Context, string) (*jwtgo.Token, error)

// JWTValidateFunc applies application-specific validation after verification.
type JWTValidateFunc func(*zinc.Context, *jwtgo.Token) error

// JWTErrorHandler maps extraction and verification failures.
type JWTErrorHandler func(*zinc.Context, error) error

// JWTConfig controls extraction, parsing, validation, and error handling.
type JWTConfig struct {
	Skipper        func(*zinc.Context) bool
	Extractor      JWTExtractor
	NewClaims      func(*zinc.Context) jwtgo.Claims
	KeyFunc        JWTKeyFunc
	ParseTokenFunc JWTParseTokenFunc
	ParserOptions  []jwtgo.ParserOption
	Validate       JWTValidateFunc
	SuccessHandler zinc.RouteHandler
	ErrorHandler   JWTErrorHandler
	Realm          string
}

type jwtContextKey int

const (
	jwtTokenContextKey jwtContextKey = iota
	jwtClaimsContextKey
	jwtTokenStringContextKey
)

// DefaultJWTConfig extracts Bearer tokens from Authorization.
func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		Extractor: JWTFromAuthHeader("Bearer"),
		SuccessHandler: func(c *zinc.Context) error {
			return c.Next()
		},
	}
}

// JWT verifies Bearer tokens using keyFunc.
func JWT(keyFunc JWTKeyFunc) zinc.Middleware {
	return JWTWithConfig(JWTConfig{KeyFunc: keyFunc})
}

// JWTWithConfig accepts only tokens marked valid by golang-jwt and then applies
// any additional application validation before publishing claims.
func JWTWithConfig(config JWTConfig) zinc.Middleware {
	cfg := resolveJWTConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		tokenString, err := cfg.Extractor(c)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}

		token, err := cfg.ParseTokenFunc(c, tokenString)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}
		if token == nil || !token.Valid {
			return cfg.ErrorHandler(c, ErrJWTTokenInvalid)
		}

		if cfg.Validate != nil {
			if err := cfg.Validate(c, token); err != nil {
				return cfg.ErrorHandler(c, err)
			}
		}

		c.Set(jwtTokenContextKey, token)
		c.Set(jwtClaimsContextKey, token.Claims)
		c.Set(jwtTokenStringContextKey, tokenString)
		return cfg.SuccessHandler(c)
	}
}

// JWTFromAuthHeader extracts a token using an authorization scheme.
func JWTFromAuthHeader(scheme string) JWTExtractor {
	scheme = strings.TrimSpace(scheme)
	if scheme == "" {
		scheme = "Bearer"
	}
	return JWTFromHeaderPrefix(zinc.HeaderAuthorization, scheme+" ")
}

// JWTFromHeader extracts an unprefixed token from header.
func JWTFromHeader(header string) JWTExtractor {
	header = textproto.CanonicalMIMEHeaderKey(header)

	return func(c *zinc.Context) (string, error) {
		value := strings.TrimSpace(c.GetHeader(header))
		if value == "" {
			return "", fmt.Errorf("%w: %s header", ErrJWTTokenMissing, header)
		}
		return value, nil
	}
}

// JWTFromHeaderPrefix extracts a token after a case-insensitive prefix.
func JWTFromHeaderPrefix(header, prefix string) JWTExtractor {
	header = textproto.CanonicalMIMEHeaderKey(header)
	if prefix == "" {
		return JWTFromHeader(header)
	}

	return func(c *zinc.Context) (string, error) {
		value := strings.TrimSpace(c.GetHeader(header))
		if value == "" {
			return "", fmt.Errorf("%w: %s header", ErrJWTTokenMissing, header)
		}
		if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
			return "", fmt.Errorf("%w: %s header", ErrJWTTokenMalformed, header)
		}
		token := strings.TrimSpace(value[len(prefix):])
		if token == "" {
			return "", fmt.Errorf("%w: %s header", ErrJWTTokenMalformed, header)
		}
		return token, nil
	}
}

// JWTFromCookie extracts a token from a cookie.
func JWTFromCookie(name string) JWTExtractor {
	return func(c *zinc.Context) (string, error) {
		cookie, err := c.Cookie(name)
		if err != nil {
			if errors.Is(err, http.ErrNoCookie) {
				return "", fmt.Errorf("%w: %s cookie", ErrJWTTokenMissing, name)
			}
			return "", err
		}
		if cookie.Value == "" {
			return "", fmt.Errorf("%w: %s cookie", ErrJWTTokenMalformed, name)
		}
		return cookie.Value, nil
	}
}

// JWTFromQuery extracts a token from a query parameter.
func JWTFromQuery(name string) JWTExtractor {
	return func(c *zinc.Context) (string, error) {
		value := c.Query(name)
		if value == "" {
			return "", fmt.Errorf("%w: %s query value", ErrJWTTokenMissing, name)
		}
		return value, nil
	}
}

// JWTFromFirst falls back only for missing tokens; malformed tokens stop lookup.
func JWTFromFirst(extractors ...JWTExtractor) JWTExtractor {
	list := append([]JWTExtractor(nil), extractors...)

	return func(c *zinc.Context) (string, error) {
		var lastMissing error
		for _, extractor := range list {
			if extractor == nil {
				continue
			}
			token, err := extractor(c)
			if err == nil {
				return token, nil
			}
			if errors.Is(err, ErrJWTTokenMissing) {
				lastMissing = err
				continue
			}
			return "", err
		}
		if lastMissing != nil {
			return "", lastMissing
		}
		return "", ErrJWTTokenMissing
	}
}

// JWTToken returns the verified token for the current request.
func JWTToken(c *zinc.Context) (*jwtgo.Token, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(jwtTokenContextKey)
	if !ok {
		return nil, false
	}
	token, ok := value.(*jwtgo.Token)
	return token, ok
}

// MustJWTToken returns the verified token or panics when absent.
func MustJWTToken(c *zinc.Context) *jwtgo.Token {
	token, ok := JWTToken(c)
	if !ok {
		panic("zincjwt: token not found")
	}
	return token
}

// JWTTokenString returns the original serialized token.
func JWTTokenString(c *zinc.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	value, ok := c.Get(jwtTokenStringContextKey)
	if !ok {
		return "", false
	}
	token, ok := value.(string)
	return token, ok
}

// MustJWTTokenString returns the serialized token or panics when absent.
func MustJWTTokenString(c *zinc.Context) string {
	token, ok := JWTTokenString(c)
	if !ok {
		panic("zincjwt: token string not found")
	}
	return token
}

// JWTClaims returns verified claims as T.
func JWTClaims[T any](c *zinc.Context) (T, bool) {
	var zero T
	if c == nil {
		return zero, false
	}
	value, ok := c.Get(jwtClaimsContextKey)
	if !ok {
		return zero, false
	}
	claims, ok := value.(T)
	if !ok {
		return zero, false
	}
	return claims, true
}

// MustJWTClaims returns verified claims as T or panics on absence or mismatch.
func MustJWTClaims[T any](c *zinc.Context) T {
	claims, ok := JWTClaims[T](c)
	if !ok {
		panic("zincjwt: claims not found")
	}
	return claims
}

func resolveJWTConfig(config JWTConfig) JWTConfig {
	cfg := DefaultJWTConfig()

	cfg.Skipper = config.Skipper
	if config.Extractor != nil {
		cfg.Extractor = config.Extractor
	}
	if config.Validate != nil {
		cfg.Validate = config.Validate
	}
	if config.SuccessHandler != nil {
		cfg.SuccessHandler = config.SuccessHandler
	}
	if config.Realm != "" {
		cfg.Realm = config.Realm
	}
	cfg.ParserOptions = append([]jwtgo.ParserOption(nil), config.ParserOptions...)

	if config.ParseTokenFunc != nil {
		cfg.ParseTokenFunc = config.ParseTokenFunc
	} else {
		if config.KeyFunc == nil {
			panic("zincjwt: KeyFunc or ParseTokenFunc is required")
		}
		claimsFactory := config.NewClaims
		if claimsFactory == nil {
			claimsFactory = func(*zinc.Context) jwtgo.Claims {
				return jwtgo.MapClaims{}
			}
		}
		keyFunc := config.KeyFunc
		options := append([]jwtgo.ParserOption(nil), config.ParserOptions...)
		cfg.ParseTokenFunc = func(c *zinc.Context, tokenString string) (*jwtgo.Token, error) {
			claims := claimsFactory(c)
			if claims == nil {
				claims = jwtgo.MapClaims{}
			}
			return jwtgo.ParseWithClaims(tokenString, claims, func(token *jwtgo.Token) (any, error) {
				return keyFunc(c, token)
			}, options...)
		}
	}

	if config.ErrorHandler != nil {
		cfg.ErrorHandler = config.ErrorHandler
	} else {
		realm := cfg.Realm
		cfg.ErrorHandler = func(c *zinc.Context, err error) error {
			var httpErr *zinc.HTTPError
			if errors.As(err, &httpErr) && httpErr.Code != http.StatusUnauthorized {
				return err
			}

			c.SetHeader(zinc.HeaderWWWAuthenticate, buildJWTBearerChallenge(realm, err))

			if errors.As(err, &httpErr) {
				return err
			}
			return zinc.ErrUnauthorized
		}
	}

	return cfg
}

func buildJWTBearerChallenge(realm string, err error) string {
	params := make([]string, 0, 2)
	if realm != "" {
		params = append(params, fmt.Sprintf(`realm=%q`, realm))
	}
	if code := jwtChallengeErrorCode(err); code != "" {
		params = append(params, fmt.Sprintf(`error=%q`, code))
	}
	if len(params) == 0 {
		return "Bearer"
	}
	return "Bearer " + strings.Join(params, ", ")
}

func jwtChallengeErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrJWTTokenMissing):
		return ""
	case errors.Is(err, ErrJWTTokenMalformed):
		return "invalid_request"
	default:
		return "invalid_token"
	}
}
