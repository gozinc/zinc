// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/0mjs/zinc"
)

// ErrSessionInvalid identifies a cookie that fails decoding or authentication.
var ErrSessionInvalid = errors.New("zincsession: invalid session")

// SessionConfig controls the signed client-side session cookie.
type SessionConfig struct {
	Skipper         func(*zinc.Context) bool
	Name            string
	Secret          []byte
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
	changed bool
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
	}
}

// SessionWithConfig verifies incoming state before exposing it and buffers the
// response until a changed session cookie can be attached safely.
func SessionWithConfig(config SessionConfig) zinc.Middleware {
	cfg := resolveSessionConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		session := &Session{values: map[string]string{}}
		if cookie, err := c.Cookie(cfg.Name); err == nil {
			values, err := decodeSessionCookie(cookie.Value, cfg.Secret)
			if err != nil {
				return errors.Join(zinc.ErrBadRequest, err)
			}
			session.values = values
		}

		c.Set(sessionStateContextKey, session)
		baseWriter := c.Writer()
		writer := &sessionResponseWriter{ResponseWriter: baseWriter}
		c.SetWriter(writer)

		err := c.Next()
		if err != nil {
			c.Error(err)
		}
		if session.changed {
			writeSessionCookie(c, cfg, session)
		}
		c.SetWriter(baseWriter)
		if flushErr := writer.FlushBuffered(); flushErr != nil && err == nil {
			return flushErr
		}
		return err
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

// Set updates a value and marks the session for persistence.
func (s *Session) Set(key, value string) {
	if s == nil {
		return
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	s.changed = true
}

// Delete removes a value and marks the session for persistence.
func (s *Session) Delete(key string) {
	if s == nil {
		return
	}
	delete(s.values, key)
	s.changed = true
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
	if len(config.Secret) == 0 {
		panic("zincsession: Secret is required")
	}
	cfg.Secret = append([]byte(nil), config.Secret...)
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
	return cfg
}

func encodeSessionCookie(values map[string]string, secret []byte) (string, error) {
	body, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	return payload + "." + signSessionPayload(payload, secret), nil
}

func decodeSessionCookie(value string, secret []byte) (map[string]string, error) {
	payload, signature, ok := strings.Cut(value, ".")
	if !ok || payload == "" || signature == "" {
		return nil, ErrSessionInvalid
	}
	expected := signSessionPayload(payload, secret)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, ErrSessionInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	if err := json.Unmarshal(body, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func signSessionPayload(payload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func writeSessionCookie(c *zinc.Context, cfg SessionConfig, session *Session) {
	value, err := encodeSessionCookie(session.values, cfg.Secret)
	if err != nil {
		c.Error(err)
		return
	}
	c.SetCookie(&http.Cookie{
		Name:     cfg.Name,
		Value:    value,
		Path:     cfg.Path,
		Domain:   cfg.Domain,
		MaxAge:   cfg.MaxAge,
		Secure:   cfg.Secure,
		HttpOnly: cfg.HTTPOnly,
		SameSite: cfg.SameSite,
		Expires:  sessionExpires(cfg),
	})
}

func sessionExpires(cfg SessionConfig) time.Time {
	if cfg.MaxAge <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(cfg.MaxAge) * time.Second)
}

type sessionResponseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *sessionResponseWriter) WriteHeader(code int) {
	// Session cookies may change after the handler returns, so headers and body
	// remain uncommitted until the middleware has persisted session state.
	if w.status == 0 {
		w.status = code
	}
}

func (w *sessionResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func (w *sessionResponseWriter) FlushBuffered() error {
	if w.status != 0 {
		w.ResponseWriter.WriteHeader(w.status)
	}
	if w.body.Len() == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.body.Bytes())
	return err
}

func (w *sessionResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
