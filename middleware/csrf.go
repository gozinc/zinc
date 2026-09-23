// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

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
)

var (
	// CSRF errors are stable sentinels for custom policy and telemetry.
	ErrCSRFTokenMissing      = errors.New("zinccsrf: request token missing")
	ErrCSRFCookieMissing     = errors.New("zinccsrf: csrf cookie missing")
	ErrCSRFTokenInvalid      = errors.New("zinccsrf: request token invalid")
	ErrCSRFFetchSiteRejected = errors.New("zinccsrf: request rejected by fetch metadata")
)

// CSRFReason classifies why a request failed validation.
type CSRFReason string

const (
	CSRFReasonTokenMissing      CSRFReason = "token_missing"
	CSRFReasonCookieMissing     CSRFReason = "cookie_missing"
	CSRFReasonTokenInvalid      CSRFReason = "token_invalid"
	CSRFReasonFetchSiteRejected CSRFReason = "fetch_site_rejected"
)

// CSRFFetchSite represents a normalized Sec-Fetch-Site value.
type CSRFFetchSite string

const (
	CSRFFetchSiteSameOrigin CSRFFetchSite = "same-origin"
	CSRFFetchSiteSameSite   CSRFFetchSite = "same-site"
	CSRFFetchSiteCrossSite  CSRFFetchSite = "cross-site"
	CSRFFetchSiteNone       CSRFFetchSite = "none"
)

// CSRFReader extracts candidate request tokens.
type CSRFReader func(*zinc.Context) ([]string, error)

// CSRFGenerator creates a new token value.
type CSRFGenerator func(*zinc.Context) (string, error)

// CSRFErrorHandler maps a CSRF violation to a handler error or response.
type CSRFErrorHandler func(*zinc.Context, error) error

// CSRFFetchSiteDecider decides whether fetch metadata permits a request.
type CSRFFetchSiteDecider func(*zinc.Context, CSRFDecision) (bool, error)

// CSRFCookie controls the double-submit token cookie.
type CSRFCookie struct {
	Name     string
	Domain   string
	Path     string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
}

// CSRFDecision describes fetch-metadata information for policy decisions.
type CSRFDecision struct {
	Site    CSRFFetchSite
	Origin  string
	Trusted bool
}

// CSRFState records token issuance or verification for the current request.
type CSRFState struct {
	Token      string
	CookieName string
	Issued     bool
	Verified   bool
	FetchSite  CSRFFetchSite
}

// CSRFViolation preserves a machine-readable failure reason and context.
type CSRFViolation struct {
	Reason     CSRFReason
	CookieName string
	FetchSite  CSRFFetchSite
	Origin     string
	Cause      error
}

func (e *CSRFViolation) Error() string {
	if e == nil {
		return "zinccsrf: validation failed"
	}

	switch e.Reason {
	case CSRFReasonTokenMissing:
		if e.Cause != nil {
			return fmt.Sprintf("zinccsrf: request token missing: %v", e.Cause)
		}
		return ErrCSRFTokenMissing.Error()
	case CSRFReasonCookieMissing:
		if e.CookieName != "" {
			return fmt.Sprintf("zinccsrf: csrf cookie %q missing", e.CookieName)
		}
		return ErrCSRFCookieMissing.Error()
	case CSRFReasonTokenInvalid:
		return ErrCSRFTokenInvalid.Error()
	case CSRFReasonFetchSiteRejected:
		if e.FetchSite != "" {
			return fmt.Sprintf("zinccsrf: request blocked by Sec-Fetch-Site %q", e.FetchSite)
		}
		return ErrCSRFFetchSiteRejected.Error()
	default:
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return "zinccsrf: validation failed"
	}
}

func (e *CSRFViolation) Unwrap() error {
	if e == nil {
		return nil
	}

	status := zinc.ErrForbidden
	if e.Reason == CSRFReasonTokenMissing {
		status = zinc.ErrBadRequest
	}
	if e.Cause != nil {
		return errors.Join(status, e.Cause)
	}
	return status
}

func (e *CSRFViolation) Is(target error) bool {
	if e == nil {
		return false
	}

	switch target {
	case ErrCSRFTokenMissing:
		return e.Reason == CSRFReasonTokenMissing
	case ErrCSRFCookieMissing:
		return e.Reason == CSRFReasonCookieMissing
	case ErrCSRFTokenInvalid:
		return e.Reason == CSRFReasonTokenInvalid
	case ErrCSRFFetchSiteRejected:
		return e.Reason == CSRFReasonFetchSiteRejected
	default:
		return false
	}
}

// CSRFConfig controls double-submit validation and fetch-metadata policy.
type CSRFConfig struct {
	Skipper        func(*zinc.Context) bool
	Readers        []CSRFReader
	Generate       CSRFGenerator
	TokenBytes     int
	Cookie         CSRFCookie
	ExposeHeader   string
	TrustedOrigins []string
	AllowFetchSite CSRFFetchSiteDecider
	ErrorHandler   CSRFErrorHandler
}

type csrfContextKey int

const csrfStateContextKey csrfContextKey = iota

var csrfSafeMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodTrace:   {},
}

// DefaultCSRFConfig returns secure double-submit cookie defaults.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		Readers:    []CSRFReader{CSRFFromHeader(zinc.HeaderXCSRFToken)},
		TokenBytes: 32,
		Cookie: CSRFCookie{
			Name:     "_csrf",
			Path:     "/",
			MaxAge:   86400,
			SameSite: http.SameSiteLaxMode,
		},
	}
}

// CSRF returns middleware using DefaultCSRFConfig.
func CSRF() zinc.Middleware {
	return CSRFWithConfig(DefaultCSRFConfig())
}

// CSRFWithConfig issues tokens on safe methods and requires a constant-time
// cookie/request-token match on unsafe methods. Cookies alone are never proof.
func CSRFWithConfig(config CSRFConfig) zinc.Middleware {
	cfg := resolveCSRFConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		req := c.Request()
		if req == nil {
			return c.Next()
		}

		state := CSRFState{
			CookieName: cfg.Cookie.Name,
			FetchSite:  normalizeCSRFFetchSite(c.GetHeader(zinc.HeaderSecFetchSite)),
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

// CSRFFromHeader reads token candidates from a request header.
func CSRFFromHeader(header string) CSRFReader {
	header = textproto.CanonicalMIMEHeaderKey(header)

	return func(c *zinc.Context) ([]string, error) {
		req := c.Request()
		if req == nil {
			return nil, fmt.Errorf("%w: %s header", ErrCSRFTokenMissing, header)
		}

		values := req.Header.Values(header)
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: %s header", ErrCSRFTokenMissing, header)
		}

		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, strings.TrimSpace(value))
		}
		return out, nil
	}
}

// CSRFFromQuery reads a token from a query parameter.
func CSRFFromQuery(name string) CSRFReader {
	return func(c *zinc.Context) ([]string, error) {
		values := c.QueryValues()[name]
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: %s query value", ErrCSRFTokenMissing, name)
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, strings.TrimSpace(value))
		}
		return out, nil
	}
}

// CSRFFromForm reads a token from URL-encoded or multipart form input.
func CSRFFromForm(name string) CSRFReader {
	return func(c *zinc.Context) ([]string, error) {
		req := c.Request()
		if req == nil {
			return nil, fmt.Errorf("%w: %s form value", ErrCSRFTokenMissing, name)
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
			return nil, fmt.Errorf("%w: %s form value", ErrCSRFTokenMissing, name)
		}

		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, strings.TrimSpace(value))
		}
		return out, nil
	}
}

// CSRFFromFirst tries token sources in order. A source may return multiple
// candidates; verification succeeds when any candidate matches the cookie.
func CSRFFromFirst(readers ...CSRFReader) CSRFReader {
	list := compactCSRFReaders(readers)

	return func(c *zinc.Context) ([]string, error) {
		var lastMissing error
		for _, reader := range list {
			values, err := reader(c)
			if err == nil {
				return values, nil
			}
			if errors.Is(err, ErrCSRFTokenMissing) {
				lastMissing = err
				continue
			}
			return nil, err
		}
		if lastMissing != nil {
			return nil, lastMissing
		}
		return nil, ErrCSRFTokenMissing
	}
}

// CSRFCurrent returns CSRF state for the current request.
func CSRFCurrent(c *zinc.Context) (CSRFState, bool) {
	if c == nil {
		return CSRFState{}, false
	}
	value, ok := c.Get(csrfStateContextKey)
	if !ok {
		return CSRFState{}, false
	}
	state, ok := value.(CSRFState)
	return state, ok
}

// MustCSRFCurrent returns CSRF state or panics when middleware did not set it.
func MustCSRFCurrent(c *zinc.Context) CSRFState {
	state, ok := CSRFCurrent(c)
	if !ok {
		panic("zinccsrf: state not found")
	}
	return state
}

// CSRFToken returns the issued or verified token.
func CSRFToken(c *zinc.Context) (string, bool) {
	state, ok := CSRFCurrent(c)
	if !ok {
		return "", false
	}
	return state.Token, true
}

// MustCSRFToken returns the token or panics when it is absent.
func MustCSRFToken(c *zinc.Context) string {
	token, ok := CSRFToken(c)
	if !ok {
		panic("zinccsrf: token not found")
	}
	return token
}

func resolveCSRFConfig(config CSRFConfig) CSRFConfig {
	cfg := DefaultCSRFConfig()

	cfg.Skipper = config.Skipper
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
			panic("zinccsrf: at least one Reader is required")
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

func compactCSRFReaders(readers []CSRFReader) []CSRFReader {
	out := make([]CSRFReader, 0, len(readers))
	for _, reader := range readers {
		if reader != nil {
			out = append(out, reader)
		}
	}
	return out
}

func randomCSRFGenerator(size int) CSRFGenerator {
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

func ensureCSRFCookieToken(c *zinc.Context, cfg CSRFConfig) (string, bool, error) {
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
			return "", &CSRFViolation{
				Reason:     CSRFReasonCookieMissing,
				CookieName: name,
				Cause:      err,
			}
		}
		return "", err
	}

	token := strings.TrimSpace(cookie.Value)
	if token == "" {
		return "", &CSRFViolation{
			Reason:     CSRFReasonCookieMissing,
			CookieName: name,
		}
	}
	return token, nil
}

func verifyCSRFRequestToken(c *zinc.Context, readers []CSRFReader, token string) error {
	var lastMissing error
	foundCandidate := false

	for _, reader := range readers {
		values, err := reader(c)
		if err != nil {
			if errors.Is(err, ErrCSRFTokenMissing) {
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
		return &CSRFViolation{
			Reason: CSRFReasonTokenMissing,
			Cause:  lastMissing,
		}
	}
	return &CSRFViolation{Reason: CSRFReasonTokenInvalid}
}

func publishCSRFToken(c *zinc.Context, cfg CSRFConfig, token string) {
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

func enforceCSRFFetchSite(c *zinc.Context, cfg CSRFConfig, site CSRFFetchSite) error {
	if site == "" || site == CSRFFetchSiteSameOrigin || site == CSRFFetchSiteNone {
		return nil
	}

	origin := strings.TrimSpace(c.GetHeader(zinc.HeaderOrigin))
	decision := CSRFDecision{
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

	return &CSRFViolation{
		Reason:    CSRFReasonFetchSiteRejected,
		FetchSite: site,
		Origin:    origin,
	}
}

func normalizeCSRFFetchSite(value string) CSRFFetchSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(CSRFFetchSiteSameOrigin):
		return CSRFFetchSiteSameOrigin
	case string(CSRFFetchSiteSameSite):
		return CSRFFetchSiteSameSite
	case string(CSRFFetchSiteCrossSite):
		return CSRFFetchSiteCrossSite
	case string(CSRFFetchSiteNone):
		return CSRFFetchSiteNone
	default:
		return CSRFFetchSite(strings.ToLower(strings.TrimSpace(value)))
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
