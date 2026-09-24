// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/0mjs/zinc"
)

// ErrSessionInvalid identifies a cookie that fails decoding or authentication.
var (
	ErrSessionInvalid   = errors.New("zincsession: invalid session")
	ErrSessionCommitted = errors.New("zincsession: response already committed")
	ErrSessionTooLarge  = errors.New("zincsession: cookie exceeds 4096 bytes")
)

const maxSessionCookieBytes = 4096

// SessionConfig controls the signed client-side session cookie.
type SessionConfig struct {
	Skipper func(*zinc.Context) bool
	Name    string
	Secret  []byte
	// PreviousSecrets are verification-only keys for a bounded rotation window.
	PreviousSecrets [][]byte
	// Lifetime bounds browser-session cookies (MaxAge == 0); defaults to 24h.
	Lifetime time.Duration
	// Now supplies the clock; defaults to time.Now.
	Now             func() time.Time
	Path            string
	Domain          string
	MaxAge          int
	Secure          bool
	HTTPOnly        bool
	DisableHTTPOnly bool
	SameSite        http.SameSite
}

// Session stores string values for one request. Values are authenticated but not
// encrypted; do not place secrets in the client-visible cookie.
type Session struct {
	values  map[string]string
	persist func(map[string]string) error
	err     error
}

type sessionContextKey int

const sessionStateContextKey sessionContextKey = iota

// SessionCookie configures a signed cookie with name and secret.
func SessionCookie(name, secret string) zinc.Middleware {
	return SessionWithConfig(SessionConfig{
		Name:   name,
		Secret: []byte(secret),
	})
}

// DefaultSessionConfig returns HTTP-only, SameSite=Lax cookie defaults.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		Name:     "zinc_session",
		Path:     "/",
		HTTPOnly: true,
		SameSite: http.SameSiteLaxMode,
		Lifetime: 24 * time.Hour,
		Now:      time.Now,
	}
}

// SessionWithConfig authenticates expiring cookies without buffering responses.
// Set and Delete persist headers immediately and must run before response output.
func SessionWithConfig(config SessionConfig) zinc.Middleware {
	cfg := resolveSessionConfig(config)
	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}
		session := &Session{values: map[string]string{}}
		if cookie, err := c.Cookie(cfg.Name); err == nil {
			payload, previous, err := decodeSessionCookie(cookie.Value, cfg)
			if err != nil {
				// Clear rejected browser-session cookies so expiry does not trap
				// subsequent sign-in requests behind the same invalid cookie.
				deleted := cfg
				deleted.MaxAge = -1
				_ = writeSessionCookie(c, deleted, nil, cfg.Now())
				return errors.Join(zinc.ErrBadRequest, err)
			}
			session.values = payload.Values
			if previous {
				// Rotate the signature without extending the authenticated lifetime.
				if err := writeSessionCookie(c, cfg, payload.Values, time.Unix(payload.Expires, 0)); err != nil {
					return err
				}
			}
		}
		session.persist = func(values map[string]string) error {
			return writeSessionCookie(c, cfg, values, cfg.Now().Add(cfg.Lifetime))
		}
		c.Set(sessionStateContextKey, session)
		err := c.Next()
		return errors.Join(err, session.err)
	}
}

// SessionCurrent returns the request session.
func SessionCurrent(c *zinc.Context) (*Session, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(sessionStateContextKey)
	if !ok {
		return nil, false
	}
	session, ok := value.(*Session)
	return session, ok
}

// MustSession returns the request session or panics when absent.
func MustSession(c *zinc.Context) *Session {
	session, ok := SessionCurrent(c)
	if !ok {
		panic("zincsession: session not found")
	}
	return session
}

// Get returns a session value.
func (s *Session) Get(key string) string {
	if s == nil {
		return ""
	}
	return s.values[key]
}

// Set persists a value before response commitment. Check the returned error;
// late or oversized mutations leave the previous session unchanged.
func (s *Session) Set(key, value string) error {
	if s == nil {
		return nil
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	previous, existed := s.values[key]
	s.values[key] = value
	if s.persist != nil {
		s.err = s.persist(s.values)
		if s.err != nil {
			if existed {
				s.values[key] = previous
			} else {
				delete(s.values, key)
			}
			return s.err
		}
	}
	return nil
}

// Delete persists removal of a value before response commitment.
func (s *Session) Delete(key string) error {
	if s == nil {
		return nil
	}
	previous, existed := s.values[key]
	delete(s.values, key)
	if s.persist != nil {
		s.err = s.persist(s.values)
		if s.err != nil {
			if existed {
				s.values[key] = previous
			}
			return s.err
		}
	}
	return nil
}

// Values returns a copy so callers cannot mutate session state without marking
// it changed and causing a new signed cookie to be written.
func (s *Session) Values() map[string]string {
	if s == nil {
		return nil
	}
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out
}

func resolveSessionConfig(config SessionConfig) SessionConfig {
	cfg := DefaultSessionConfig()
	cfg.Skipper = config.Skipper
	if config.Name != "" {
		cfg.Name = config.Name
	}
	if len(config.Secret) < 32 {
		panic("zincsession: Secret must contain at least 32 random bytes")
	}
	cfg.Secret = append([]byte(nil), config.Secret...)
	for _, key := range config.PreviousSecrets {
		if len(key) < 32 {
			panic("zincsession: PreviousSecrets must contain at least 32 random bytes each")
		}
		cfg.PreviousSecrets = append(cfg.PreviousSecrets, append([]byte(nil), key...))
	}
	cfg.Now = config.Now
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Lifetime = config.Lifetime
	if cfg.Lifetime < 0 {
		panic("zincsession: Lifetime must not be negative")
	}
	if cfg.Lifetime == 0 {
		cfg.Lifetime = 24 * time.Hour
	}
	if config.MaxAge > 0 {
		if int64(config.MaxAge) > int64((1<<63-1)/time.Second) {
			panic("zincsession: MaxAge is too large")
		}
		cfg.Lifetime = time.Duration(config.MaxAge) * time.Second
	}
	if cfg.Lifetime < time.Second {
		panic("zincsession: Lifetime must be at least one second")
	}
	if config.Path != "" {
		cfg.Path = config.Path
	}
	cfg.Domain = config.Domain
	cfg.MaxAge = config.MaxAge
	cfg.Secure = config.Secure
	if config.DisableHTTPOnly {
		cfg.HTTPOnly = false
	} else if config.HTTPOnly {
		cfg.HTTPOnly = true
	}
	if config.SameSite != 0 {
		cfg.SameSite = config.SameSite
	}
	if err := (&http.Cookie{Name: cfg.Name, Path: cfg.Path, Domain: cfg.Domain}).Valid(); err != nil {
		panic(fmt.Sprintf("zincsession: invalid cookie configuration: %v", err))
	}
	return cfg
}

type sessionPayload struct {
	Version int               `json:"v"`
	Expires int64             `json:"exp"`
	Values  map[string]string `json:"values"`
}

func encodeSessionCookie(values map[string]string, secret []byte, expires time.Time) (string, error) {
	body, err := json.Marshal(sessionPayload{Version: 1, Expires: expires.Unix(), Values: values})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	return payload + "." + signSessionPayload(payload, secret), nil
}

func decodeSessionCookie(value string, cfg SessionConfig) (sessionPayload, bool, error) {
	var decoded sessionPayload
	if len(value) > maxSessionCookieBytes {
		return decoded, false, ErrSessionInvalid
	}
	payload, signature, ok := strings.Cut(value, ".")
	if !ok || payload == "" || signature == "" {
		return decoded, false, ErrSessionInvalid
	}
	previous := false
	valid := hmac.Equal([]byte(signature), []byte(signSessionPayload(payload, cfg.Secret)))
	if !valid {
		for _, key := range cfg.PreviousSecrets {
			if hmac.Equal([]byte(signature), []byte(signSessionPayload(payload, key))) {
				valid, previous = true, true
				break
			}
		}
	}
	if !valid {
		return decoded, false, ErrSessionInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return decoded, false, ErrSessionInvalid
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return decoded, false, ErrSessionInvalid
	}
	if decoded.Version != 1 || decoded.Expires <= cfg.Now().Unix() || decoded.Values == nil {
		return decoded, false, ErrSessionInvalid
	}
	return decoded, previous, nil
}

func signSessionPayload(payload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func writeSessionCookie(c *zinc.Context, cfg SessionConfig, values map[string]string, expires time.Time) error {
	writer := c.Writer()
	if writer == nil || zinc.WrapResponseWriter(writer).Written() {
		return ErrSessionCommitted
	}
	value, err := encodeSessionCookie(values, cfg.Secret, expires)
	if err != nil {
		return err
	}
	cookie := &http.Cookie{Name: cfg.Name, Value: value, Path: cfg.Path, Domain: cfg.Domain, MaxAge: cfg.MaxAge, Secure: cfg.Secure, HttpOnly: cfg.HTTPOnly, SameSite: cfg.SameSite}
	if cfg.MaxAge > 0 {
		cookie.Expires = expires
		cookie.MaxAge = int(expires.Unix() - cfg.Now().Unix())
	}
	if cfg.MaxAge < 0 {
		cookie.Value = ""
		cookie.Expires = time.Unix(1, 0)
	}
	serialized := cookie.String()
	if len(serialized) > maxSessionCookieBytes {
		return ErrSessionTooLarge
	}
	// Replace only this cookie's name/path/domain, preserving unrelated cookies.
	header := writer.Header()
	cookies := header.Values(zinc.HeaderSetCookie)
	kept := cookies[:0]
	for _, existing := range cookies {
		parsed, err := http.ParseSetCookie(existing)
		if err == nil && parsed.Name == cookie.Name && parsed.Path == cookie.Path && parsed.Domain == cookie.Domain {
			continue
		}
		kept = append(kept, existing)
	}
	header[zinc.HeaderSetCookie] = append(kept, serialized)
	return nil
}
