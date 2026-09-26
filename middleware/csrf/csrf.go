// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

var (
	// CSRF errors are stable sentinels for custom policy and telemetry.
	ErrTokenMissing      = errors.New("csrf: request token missing")
	ErrCookieMissing     = errors.New("csrf: csrf cookie missing")
	ErrTokenInvalid      = errors.New("csrf: request token invalid")
	ErrFetchSiteRejected = errors.New("csrf: request rejected by fetch metadata")
)

// Reason classifies why a request failed validation.
type Reason string

const (
	ReasonTokenMissing      Reason = "token_missing"
	ReasonCookieMissing     Reason = "cookie_missing"
	ReasonTokenInvalid      Reason = "token_invalid"
	ReasonFetchSiteRejected Reason = "fetch_site_rejected"
)

// FetchSite represents a normalized Sec-Fetch-Site value.
type FetchSite string

const (
	FetchSiteSameOrigin FetchSite = "same-origin"
	FetchSiteSameSite   FetchSite = "same-site"
	FetchSiteCrossSite  FetchSite = "cross-site"
	FetchSiteNone       FetchSite = "none"
)

// Reader extracts candidate request tokens.
type Reader func(*zinc.Context) ([]string, error)

// Generator creates a new token value.
type Generator func(*zinc.Context) (string, error)

// FetchSiteDecider decides whether fetch metadata permits a request.
type FetchSiteDecider func(*zinc.Context, Decision) (bool, error)

// Cookie controls the double-submit token cookie.
type Cookie struct {
	Name     string
	Domain   string
	Path     string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
}

// Decision describes fetch-metadata information for policy decisions.
type Decision struct {
	Site    FetchSite
	Origin  string
	Trusted bool
}

// State records token issuance or verification for the current request.
type State struct {
	Token      string
	CookieName string
	Issued     bool
	Verified   bool
	FetchSite  FetchSite
}

// Violation preserves a machine-readable failure reason and context.
type Violation struct {
	Reason     Reason
	CookieName string
	FetchSite  FetchSite
	Origin     string
	Cause      error
}

func (e *Violation) Error() string {
	if e == nil {
		return "csrf: validation failed"
	}

	switch e.Reason {
	case ReasonTokenMissing:
		if e.Cause != nil {
			return fmt.Sprintf("csrf: request token missing: %v", e.Cause)
		}
		return ErrTokenMissing.Error()
	case ReasonCookieMissing:
		if e.CookieName != "" {
			return fmt.Sprintf("csrf: csrf cookie %q missing", e.CookieName)
		}
		return ErrCookieMissing.Error()
	case ReasonTokenInvalid:
		return ErrTokenInvalid.Error()
	case ReasonFetchSiteRejected:
		if e.FetchSite != "" {
			return fmt.Sprintf("csrf: request blocked by Sec-Fetch-Site %q", e.FetchSite)
		}
		return ErrFetchSiteRejected.Error()
	default:
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return "csrf: validation failed"
	}
}

func (e *Violation) Unwrap() error {
	if e == nil {
		return nil
	}

	status := zinc.ErrForbidden
	if e.Reason == ReasonTokenMissing {
		status = zinc.ErrBadRequest
	}
	if e.Cause != nil {
		return errors.Join(status, e.Cause)
	}
	return status
}

func (e *Violation) Is(target error) bool {
	if e == nil {
		return false
	}

	switch target {
	case ErrTokenMissing:
		return e.Reason == ReasonTokenMissing
	case ErrCookieMissing:
		return e.Reason == ReasonCookieMissing
	case ErrTokenInvalid:
		return e.Reason == ReasonTokenInvalid
	case ErrFetchSiteRejected:
		return e.Reason == ReasonFetchSiteRejected
	default:
		return false
	}
}

// Config controls double-submit validation and fetch-metadata policy.
type Config struct {
	Readers        []Reader
	Generate       Generator
	TokenBytes     int
	Cookie         Cookie
	ExposeHeader   string
	TrustedOrigins []string
	AllowFetchSite FetchSiteDecider
	ErrorHandler   func(*zinc.Context, error) error
}

type csrfContextKey int

const csrfStateContextKey csrfContextKey = iota

var csrfSafeMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodTrace:   {},
}

// defaultConfig returns secure double-submit cookie defaults.
func defaultConfig() Config {
	return Config{
		Readers:    []Reader{FromHeader(zinc.HeaderXCSRFToken)},
		TokenBytes: 32,
		Cookie: Cookie{
			Name:     "_csrf",
			Path:     "/",
			MaxAge:   86400,
			SameSite: http.SameSiteLaxMode,
		},
	}
}

// New issues tokens on safe methods and requires a constant-time
// cookie/request-token match on unsafe methods. Cookies alone are never proof.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("csrf", configs)
	cfg := resolveCSRFConfig(config)

	return func(c *zinc.Context) error {
		req := c.Request()
		if req == nil {
			return c.Next()
		}

		state := State{
			CookieName: cfg.Cookie.Name,
			FetchSite:  normalizeCSRFFetchSite(c.Header(zinc.HeaderSecFetchSite)),
		}

		if !isCSRFSafeMethod(req.Method) {
			if err := enforceCSRFFetchSite(c, cfg, state.FetchSite); err != nil {
				return cfg.ErrorHandler(c, err)
			}

			token, err := readCSRFCookieToken(c, cfg.Cookie.Name)
			if err != nil {
				return cfg.ErrorHandler(c, err)
			}

			if err := verifyCSRFRequestToken(c, cfg.Readers, token); err != nil {
				return cfg.ErrorHandler(c, err)
			}

			state.Token = token
			state.Verified = true
			c.Set(csrfStateContextKey, state)
			publishCSRFToken(c, cfg, token)
			return c.Next()
		}

		token, issued, err := ensureCSRFCookieToken(c, cfg)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}

		state.Token = token
		state.Issued = issued
		c.Set(csrfStateContextKey, state)
		publishCSRFToken(c, cfg, token)
		return c.Next()
	}
}

// FromHeader reads token candidates from a request header.
func FromHeader(header string) Reader {
	header = textproto.CanonicalMIMEHeaderKey(header)

	return func(c *zinc.Context) ([]string, error) {
		req := c.Request()
		if req == nil {
			return nil, fmt.Errorf("%w: %s header", ErrTokenMissing, header)
		}

		values := req.Header.Values(header)
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: %s header", ErrTokenMissing, header)
		}

		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, strings.TrimSpace(value))
		}
		return out, nil
	}
}

// FromQuery reads a token from a query parameter.
func FromQuery(name string) Reader {
	return func(c *zinc.Context) ([]string, error) {
		values := c.QueryValues()[name]
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: %s query value", ErrTokenMissing, name)
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, strings.TrimSpace(value))
		}
		return out, nil
	}
}

// FromForm reads a token from URL-encoded or multipart form input.
func FromForm(name string) Reader {
	return func(c *zinc.Context) ([]string, error) {
		req := c.Request()
		if req == nil {
			return nil, fmt.Errorf("%w: %s form value", ErrTokenMissing, name)
		}

		mediaType, _, err := mime.ParseMediaType(req.Header.Get(zinc.HeaderContentType))
		if err != nil {
			return nil, err
		}

		switch mediaType {
		case "multipart/form-data":
			if req.MultipartForm == nil {
				if err := req.ParseMultipartForm(32 << 20); err != nil {
					return nil, err
				}
			}
		default:
			if err := req.ParseForm(); err != nil {
				return nil, err
			}
		}

		values := req.Form[name]
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: %s form value", ErrTokenMissing, name)
		}

		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, strings.TrimSpace(value))
		}
		return out, nil
	}
}

// FromFirst tries token sources in order. A source may return multiple
// candidates; verification succeeds when any candidate matches the cookie.
func FromFirst(readers ...Reader) Reader {
	list := compactCSRFReaders(readers)

	return func(c *zinc.Context) ([]string, error) {
		var lastMissing error
		for _, reader := range list {
			values, err := reader(c)
			if err == nil {
				return values, nil
			}
			if errors.Is(err, ErrTokenMissing) {
				lastMissing = err
				continue
			}
			return nil, err
		}
		if lastMissing != nil {
			return nil, lastMissing
		}
		return nil, ErrTokenMissing
	}
}

// Get returns CSRF state for the current request.
func Get(c *zinc.Context) (State, bool) {
	if c == nil {
		return State{}, false
	}
	value, ok := c.Get(csrfStateContextKey)
	if !ok {
		return State{}, false
	}
	state, ok := value.(State)
	return state, ok
}

// MustGet returns CSRF state or panics when middleware did not set it.
func MustGet(c *zinc.Context) State {
	state, ok := Get(c)
	if !ok {
		panic("csrf: state not found")
	}
	return state
}

// Token returns the issued or verified token.
func Token(c *zinc.Context) string {
	state, _ := Get(c)
	return state.Token
}

func resolveCSRFConfig(config Config) Config {
	cfg := defaultConfig()

	cfg.ExposeHeader = config.ExposeHeader
	cfg.AllowFetchSite = config.AllowFetchSite

	if config.TokenBytes > 0 {
		cfg.TokenBytes = config.TokenBytes
	}
	if config.Generate != nil {
		cfg.Generate = config.Generate
	}
	if cfg.Generate == nil {
		cfg.Generate = randomCSRFGenerator(cfg.TokenBytes)
	}

	if config.Readers != nil {
		cfg.Readers = compactCSRFReaders(config.Readers)
		if len(cfg.Readers) == 0 {
			panic("csrf: at least one Reader is required")
		}
	}

	if config.Cookie.Name != "" {
		cfg.Cookie.Name = config.Cookie.Name
	}
	if config.Cookie.Domain != "" {
		cfg.Cookie.Domain = config.Cookie.Domain
	}
	if config.Cookie.Path != "" {
		cfg.Cookie.Path = config.Cookie.Path
	}
	if config.Cookie.MaxAge != 0 {
		cfg.Cookie.MaxAge = config.Cookie.MaxAge
	}
	if config.Cookie.Secure {
		cfg.Cookie.Secure = true
	}
	if config.Cookie.HTTPOnly {
		cfg.Cookie.HTTPOnly = true
	}
	if config.Cookie.SameSite != 0 {
		cfg.Cookie.SameSite = config.Cookie.SameSite
	}
	if cfg.Cookie.SameSite == http.SameSiteNoneMode {
		cfg.Cookie.Secure = true
	}

	if len(config.TrustedOrigins) > 0 {
		if err := validateCSRFOrigins(config.TrustedOrigins); err != nil {
			panic(err)
		}
		cfg.TrustedOrigins = append([]string(nil), config.TrustedOrigins...)
	}

	if config.ErrorHandler != nil {
		cfg.ErrorHandler = config.ErrorHandler
	} else {
		cfg.ErrorHandler = func(_ *zinc.Context, err error) error {
			return err
		}
	}

	return cfg
}

func compactCSRFReaders(readers []Reader) []Reader {
	out := make([]Reader, 0, len(readers))
	for _, reader := range readers {
		if reader != nil {
			out = append(out, reader)
		}
	}
	return out
}

func randomCSRFGenerator(size int) Generator {
	if size <= 0 {
		size = 32
	}

	return func(*zinc.Context) (string, error) {
		b := make([]byte, size)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(b), nil
	}
}

func ensureCSRFCookieToken(c *zinc.Context, cfg Config) (string, bool, error) {
	cookie, err := c.Cookie(cfg.Cookie.Name)
	if err == nil {
		if value := strings.TrimSpace(cookie.Value); value != "" {
			return value, false, nil
		}
	}
	if err != nil && !errors.Is(err, http.ErrNoCookie) {
		return "", false, err
	}

	token, err := cfg.Generate(c)
	if err != nil {
		return "", false, err
	}
	return token, true, nil
}

func readCSRFCookieToken(c *zinc.Context, name string) (string, error) {
	cookie, err := c.Cookie(name)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return "", &Violation{
				Reason:     ReasonCookieMissing,
				CookieName: name,
				Cause:      err,
			}
		}
		return "", err
	}

	token := strings.TrimSpace(cookie.Value)
	if token == "" {
		return "", &Violation{
			Reason:     ReasonCookieMissing,
			CookieName: name,
		}
	}
	return token, nil
}

func verifyCSRFRequestToken(c *zinc.Context, readers []Reader, token string) error {
	var lastMissing error
	foundCandidate := false

	for _, reader := range readers {
		values, err := reader(c)
		if err != nil {
			if errors.Is(err, ErrTokenMissing) {
				lastMissing = err
				continue
			}
			return errors.Join(zinc.ErrBadRequest, err)
		}

		foundCandidate = true
		// Compare every supplied representation without normalizing the token;
		// transformations here could make distinct credentials equivalent.
		for _, value := range values {
			if subtle.ConstantTimeCompare([]byte(token), []byte(value)) == 1 {
				return nil
			}
		}
	}

	if !foundCandidate {
		return &Violation{
			Reason: ReasonTokenMissing,
			Cause:  lastMissing,
		}
	}
	return &Violation{Reason: ReasonTokenInvalid}
}

func publishCSRFToken(c *zinc.Context, cfg Config, token string) {
	c.SetCookie(&http.Cookie{
		Name:     cfg.Cookie.Name,
		Value:    token,
		Path:     cfg.Cookie.Path,
		Domain:   cfg.Cookie.Domain,
		MaxAge:   cfg.Cookie.MaxAge,
		Expires:  time.Now().Add(time.Duration(cfg.Cookie.MaxAge) * time.Second),
		Secure:   cfg.Cookie.Secure,
		HttpOnly: cfg.Cookie.HTTPOnly,
		SameSite: cfg.Cookie.SameSite,
	})
	c.AppendHeader(zinc.HeaderVary, zinc.HeaderCookie)
	if cfg.ExposeHeader != "" {
		c.SetHeader(cfg.ExposeHeader, token)
	}
}

func enforceCSRFFetchSite(c *zinc.Context, cfg Config, site FetchSite) error {
	if site == "" || site == FetchSiteSameOrigin || site == FetchSiteNone {
		return nil
	}

	origin := strings.TrimSpace(c.Header(zinc.HeaderOrigin))
	decision := Decision{
		Site:   site,
		Origin: origin,
	}

	for _, trustedOrigin := range cfg.TrustedOrigins {
		if strings.EqualFold(origin, trustedOrigin) {
			decision.Trusted = true
			return nil
		}
	}

	if cfg.AllowFetchSite != nil {
		allow, err := cfg.AllowFetchSite(c, decision)
		if err != nil {
			return err
		}
		if allow {
			return nil
		}
	}

	return &Violation{
		Reason:    ReasonFetchSiteRejected,
		FetchSite: site,
		Origin:    origin,
	}
}

func normalizeCSRFFetchSite(value string) FetchSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(FetchSiteSameOrigin):
		return FetchSiteSameOrigin
	case string(FetchSiteSameSite):
		return FetchSiteSameSite
	case string(FetchSiteCrossSite):
		return FetchSiteCrossSite
	case string(FetchSiteNone):
		return FetchSiteNone
	default:
		return FetchSite(strings.ToLower(strings.TrimSpace(value)))
	}
}

func isCSRFSafeMethod(method string) bool {
	_, ok := csrfSafeMethods[method]
	return ok
}

func validateCSRFOrigins(origins []string) error {
	for _, origin := range origins {
		if err := validateCSRFOrigin(origin); err != nil {
			return err
		}
	}
	return nil
}

func validateCSRFOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("can not parse trusted origin: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("trusted origin is missing scheme or host: %s", origin)
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("trusted origin can not have path, query, and fragments: %s", origin)
	}
	return nil
}
