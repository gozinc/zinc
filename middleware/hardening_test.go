// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/0mjs/zinc"
)

func TestRequestIDDefaultGeneratorAndFallbackValue(t *testing.T) {
	app := zinc.New()
	app.Use(RequestID())
	app.Get("/", func(c *zinc.Context) error {
		id := RequestIDValue(c)
		if len(id) != 32 {
			t.Fatalf("request id length=%d id=%q", len(id), id)
		}
		return c.String(id)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderXRequestID); got != rec.Body.String() {
		t.Fatalf("header=%q body=%q", got, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderXRequestID, "fallback")
	app = zinc.New()
	app.Get("/", func(c *zinc.Context) error {
		if got := RequestIDValue(c); got != "fallback" {
			t.Fatalf("fallback request id=%q", got)
		}
		return c.String("ok")
	})
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
}

func TestKeyAuthExtractorsAndFirstFallback(t *testing.T) {
	app := zinc.New()
	app.Use(KeyAuthWithConfig(KeyAuthConfig{
		Extractor: KeyAuthFromFirst(
			KeyAuthFromHeader("X-Missing"),
			KeyAuthFromHeaderPrefix("X-API-Key", "Token "),
			KeyAuthFromCookie("api_key"),
		),
		Validator: KeyAuthStaticKeys("secret", "backup"),
	}))
	app.Get("/private", func(c *zinc.Context) error {
		state := MustKeyAuthCurrent(c)
		return c.String(state.Key + ":" + string(state.Source))
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("X-API-Key", "Token backup")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "backup:"+string(KeyAuthSourceHeader) {
		t.Fatalf("body=%q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: "api_key", Value: "secret"})
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "secret:"+string(KeyAuthSourceCookie) {
		t.Fatalf("body=%q", rec.Body.String())
	}

	app = zinc.New()
	app.Use(KeyAuth(KeyAuthStatic("subject-key")))
	app.Get("/subject", func(c *zinc.Context) error {
		return c.String(CasbinSubjectFromKeyAuth()(c).(string))
	})
	req = httptest.NewRequest(http.MethodGet, "/subject", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer subject-key")
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "subject-key" {
		t.Fatalf("subject body=%q", rec.Body.String())
	}
}

func TestAuthTokenExtractorsAndErrorValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?jwt=query-token&csrf=one&csrf=two", nil)
	req.Header.Set("X-Basic", base64.StdEncoding.EncodeToString([]byte("joe:secret")))
	req.Header.Set("X-JWT", "header-token")
	app := zinc.New()
	ctx := app.AcquireContext(httptest.NewRecorder(), req)
	defer app.ReleaseContext(ctx)

	credentials, err := BasicAuthFromHeader("X-Basic")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Username != "joe" || credentials.Password != "secret" || credentials.Source != BasicAuthSourceHeader {
		t.Fatalf("credentials=%+v", credentials)
	}

	token, err := JWTFromHeader("X-JWT")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if token != "header-token" {
		t.Fatalf("header token=%q", token)
	}
	token, err = JWTFromQuery("jwt")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if token != "query-token" {
		t.Fatalf("query token=%q", token)
	}

	values, err := CSRFFromQuery("csrf")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(values, ",") != "one,two" {
		t.Fatalf("csrf values=%v", values)
	}

	timeoutErr := &ContextTimeoutError{
		Info:  ContextTimeoutInfo{Timeout: time.Second},
		Cause: context.Canceled,
	}
	if timeoutErr.Error() == "" || !errors.Is(timeoutErr, ErrContextTimeout) || !errors.Is(timeoutErr.Unwrap(), context.Canceled) {
		t.Fatalf("timeout error=%v unwrap=%v", timeoutErr, timeoutErr.Unwrap())
	}
	var nilTimeoutErr *ContextTimeoutError
	if nilTimeoutErr.Error() == "" || !errors.Is(nilTimeoutErr.Unwrap(), context.DeadlineExceeded) {
		t.Fatalf("nil timeout error=%v unwrap=%v", nilTimeoutErr, nilTimeoutErr.Unwrap())
	}

	if got := (&RecoverError{Value: "boom"}).Error(); !strings.Contains(got, "boom") {
		t.Fatalf("recover error=%q", got)
	}
	var nilRecoverErr *RecoverError
	if nilRecoverErr.Error() == "" {
		t.Fatal("nil recover error should have a message")
	}
}

func TestKeyAuthCustomErrorHandlerCanReturnHTTPError(t *testing.T) {
	app := zinc.New()
	app.Use(KeyAuthWithConfig(KeyAuthConfig{
		Validator: KeyAuthStatic("secret"),
		ErrorHandler: func(_ *zinc.Context, err error) error {
			if errors.Is(err, ErrKeyAuthKeyMissing) {
				return zinc.ErrForbidden
			}
			return err
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMethodOverrideQueryAndFirstGetter(t *testing.T) {
	app := zinc.New()
	app.Use(MethodOverrideWithConfig(MethodOverrideConfig{
		Getter: MethodOverrideFromFirst(
			MethodOverrideFromHeader("X-Missing"),
			MethodOverrideFromQuery("_method"),
		),
		SourceMethods: []string{"post"},
		Methods:       []string{"patch"},
	}))
	app.Patch("/resource", func(c *zinc.Context) error {
		return c.String(c.Method())
	})

	req := httptest.NewRequest(http.MethodPost, "/resource?_method=PATCH", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != http.MethodPatch {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestTrailingSlashAddAndPathHelpers(t *testing.T) {
	app := zinc.New()
	app.Use(AddTrailingSlash())
	app.Get("/users/", func(c *zinc.Context) error {
		return c.String(c.Path())
	})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "/users/" {
		t.Fatalf("body=%q", rec.Body.String())
	}

	if got := normalizeTrailingSlashPath("", false); got != "/" {
		t.Fatalf("empty path=%q", got)
	}
	if got := normalizeTrailingSlashPath("/", true); got != "/" {
		t.Fatalf("root path=%q", got)
	}
	if got := pathWithRawQuery("/users", ""); got != "/users" {
		t.Fatalf("path without query=%q", got)
	}
}

func TestGzipAcceptQValuesAndExistingVary(t *testing.T) {
	if requestAcceptsGzip("gzip;q=0") {
		t.Fatal("gzip q=0 should not be accepted")
	}
	if !requestAcceptsGzip("br;q=1, gzip;q=0.5") {
		t.Fatal("gzip q=0.5 should be accepted")
	}

	app := zinc.New()
	app.Use(Gzip())
	app.Get("/", func(c *zinc.Context) error {
		c.Vary(zinc.HeaderAcceptEncoding)
		return c.String("hello")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderAcceptEncoding, "gzip;q=1")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if values := rec.Header().Values(zinc.HeaderVary); len(values) != 1 || values[0] != zinc.HeaderAcceptEncoding {
		t.Fatalf("vary=%v", values)
	}
	if got := gunzipResponse(t, rec.Body.Bytes()); got != "hello" {
		t.Fatalf("body=%q", got)
	}
}

func TestGzipHeadSkipsBody(t *testing.T) {
	app := zinc.New()
	app.Use(Gzip())
	app.Head("/head", func(c *zinc.Context) error {
		return c.String("no body")
	})

	req := httptest.NewRequest(http.MethodHead, "/head", nil)
	req.Header.Set(zinc.HeaderAcceptEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderContentEncoding); got != "" {
		t.Fatalf("content-encoding=%q", got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestDecompressClose(t *testing.T) {
	app := zinc.New()
	app.Use(Decompress())
	app.Post("/", func(c *zinc.Context) error {
		if err := c.Request().Body.Close(); err != nil {
			return err
		}
		return c.String("closed")
	})

	req := httptest.NewRequest(http.MethodPost, "/", gzipBody(t, "close me"))
	req.Header.Set(zinc.HeaderContentEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSecureDefaultsAndHSTSOptions(t *testing.T) {
	app := zinc.New()
	app.Use(Secure())
	app.Get("/", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get(zinc.HeaderStrictTransportSecurity); got != "" {
		t.Fatalf("hsts over http=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderXXSSProtection); got != "0" {
		t.Fatalf("xss=%q", got)
	}

	app = zinc.New()
	app.Use(SecureWithConfig(SecureConfig{
		HSTSMaxAge:            60,
		HSTSExcludeSubdomains: true,
	}))
	app.Get("/", func(c *zinc.Context) error {
		return c.String("ok")
	})
	req = httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if got := rec.Header().Get(zinc.HeaderStrictTransportSecurity); got != "max-age=60" {
		t.Fatalf("hsts=%q", got)
	}
}

func TestSessionDeleteValuesAndOptions(t *testing.T) {
	cfg := DefaultSessionConfig()
	cfg.Secret = []byte("secret")
	cfg.Name = "sid"
	cfg.MaxAge = 10
	cfg.DisableHTTPOnly = true

	app := zinc.New()
	app.Use(SessionWithConfig(cfg))
	app.Get("/", func(c *zinc.Context) error {
		session := MustSession(c)
		session.Set("one", "1")
		session.Set("two", "2")
		session.Delete("two")
		values := session.Values()
		return c.String(values["one"] + values["two"])
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "1" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v", cookies)
	}
	if cookies[0].HttpOnly {
		t.Fatalf("cookie should not be http-only: %+v", cookies[0])
	}
	if cookies[0].MaxAge != 10 || cookies[0].Expires.IsZero() {
		t.Fatalf("cookie max-age/expires=%+v", cookies[0])
	}
}

func TestSessionHelpersOnNil(t *testing.T) {
	var session *Session
	if got := session.Get("missing"); got != "" {
		t.Fatalf("nil session get=%q", got)
	}
	session.Set("ignored", "value")
	session.Delete("ignored")
	if values := session.Values(); values != nil {
		t.Fatalf("nil session values=%v", values)
	}
	if _, ok := SessionCurrent(nil); ok {
		t.Fatal("nil context should not have session")
	}
}

func TestSessionResponseWriterBuffersStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := &sessionResponseWriter{ResponseWriter: rec}

	writer.WriteHeader(http.StatusCreated)
	writer.WriteHeader(http.StatusAccepted)
	if _, err := writer.Write([]byte("created")); err != nil {
		t.Fatal(err)
	}
	if writer.Unwrap() != rec {
		t.Fatal("unwrap returned the wrong writer")
	}
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("response should still be buffered: status=%d body=%q", rec.Code, rec.Body.String())
	}

	mustNoErrHardening(t, writer.FlushBuffered())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != "created" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestPrometheusConvenienceMiddleware(t *testing.T) {
	metrics := NewPrometheusMetrics()
	app := zinc.New()
	app.Use(Prometheus(metrics))
	app.Get("/ok", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	text := metrics.Text()
	if !strings.Contains(text, `method="GET"`) || !strings.Contains(text, `route="/ok"`) || !strings.Contains(text, `status="200"`) {
		t.Fatalf("metrics=%s", text)
	}
}

func TestStaticConstructorsAndPrefixHelpers(t *testing.T) {
	fsys := fstest.MapFS{
		"hello.txt": &fstest.MapFile{Data: []byte("hello")},
	}

	app := zinc.New()
	app.Use(StaticFS(fsys))
	req := httptest.NewRequest(http.MethodGet, "/hello.txt", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if strings.TrimSpace(rec.Body.String()) != "hello" {
		t.Fatalf("static fs body=%q", rec.Body.String())
	}

	name, ok := staticRequestName("/assets/../bad", "/assets")
	if ok || name != "" {
		t.Fatalf("bad static path name=%q ok=%v", name, ok)
	}
	if got := normalizeStaticPrefix("assets/"); got != "/assets" {
		t.Fatalf("prefix=%q", got)
	}
}

func TestRedirectAndRewriteRulesHelpers(t *testing.T) {
	app := zinc.New()
	app.Use(RedirectWithRules(map[string]string{"/old/*": "/new/*"}))

	req := httptest.NewRequest(http.MethodGet, "/old/path", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/new/path" {
		t.Fatalf("location=%q", got)
	}

	if target, ok := rewriteTarget("/x/path", map[string]string{"/x/*": "/y"}); !ok || target != "/ypath" {
		t.Fatalf("target=%q ok=%v", target, ok)
	}
	if target, ok := rewriteTarget("/x/path", map[string]string{"/z/*": "/y/*"}); ok || target != "" {
		t.Fatalf("target=%q ok=%v", target, ok)
	}
}

func TestPrometheusDefaultAndEscaping(t *testing.T) {
	metrics := NewPrometheusMetrics()
	metrics.Observe("GE\"T", "/line\n\\route", 200, 0)
	text := metrics.Text()
	if !strings.Contains(text, `method="GE\"T"`) {
		t.Fatalf("metrics missing escaped quote: %s", text)
	}
	if !strings.Contains(text, `route="/line\n\\route"`) {
		t.Fatalf("metrics missing escaped route: %s", text)
	}
	var nilMetrics *PrometheusMetrics
	if nilMetrics.Text() != "" {
		t.Fatal("nil metrics should render empty text")
	}
}

func TestProxyConfigHooksAndSkipper(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Director", r.Header.Get("X-Director"))
		_, _ = w.Write([]byte("upstream"))
	}))
	defer upstream.Close()

	app := zinc.New()
	app.Use(ProxyWithConfig(ProxyConfig{
		Target: upstream.URL,
		Director: func(req *http.Request) {
			req.Header.Set("X-Director", "set")
		},
		Modify: func(resp *http.Response) error {
			resp.Header.Set("X-Modified", "yes")
			return nil
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Director"); got != "set" {
		t.Fatalf("director header=%q", got)
	}
	if got := rec.Header().Get("X-Modified"); got != "yes" {
		t.Fatalf("modified header=%q", got)
	}

	app = zinc.New()
	app.Use(ProxyWithConfig(ProxyConfig{
		Target:  upstream.URL,
		Skipper: func(*zinc.Context) bool { return true },
	}))
	app.Get("/", func(c *zinc.Context) error {
		return c.String("skipped")
	})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "skipped" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestProxyTargetsRewriteAndRetry(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first:"+r.URL.Path)
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "second:"+r.URL.Path)
	}))
	defer second.Close()

	app := zinc.New()
	app.Use(ProxyWithConfig(ProxyConfig{
		Targets: []*ProxyTarget{
			{Name: "first", URL: mustParseProxyURLHardening(t, first.URL)},
			{Name: "second", URL: mustParseProxyURLHardening(t, second.URL)},
		},
		Rewrite: map[string]string{"/proxy/*": "/*"},
	}))

	req := httptest.NewRequest(http.MethodGet, "/proxy/users", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "first:/users" {
		t.Fatalf("first body=%q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/proxy/users", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "second:/users" {
		t.Fatalf("second body=%q", rec.Body.String())
	}

	app = zinc.New()
	app.Use(ProxyWithConfig(ProxyConfig{
		Target: first.URL,
		RegexRewrite: map[*regexp.Regexp]string{
			regexp.MustCompile(`^/v([0-9]+)/(.+)$`): `/api/v$1/$2`,
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Modified-Response", "yes")
			return nil
		},
	}))
	req = httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "first:/api/v1/users" {
		t.Fatalf("regex rewrite body=%q", rec.Body.String())
	}
	if got := rec.Header().Get("X-Modified-Response"); got != "yes" {
		t.Fatalf("modify response header=%q", got)
	}

	attempts := 0
	retryErr := errors.New("temporary upstream failure")
	app = zinc.New()
	app.Use(ProxyWithConfig(ProxyConfig{
		Target:  "http://example.com",
		Retries: 1,
		RetryFilter: func(c *zinc.Context, err error) bool {
			return c.Path() == "/retry" && errors.Is(err, retryErr)
		},
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return nil, retryErr
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("retried")),
				Request:    req,
			}, nil
		}),
	}))
	req = httptest.NewRequest(http.MethodGet, "/retry", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if attempts != 2 || rec.Body.String() != "retried" {
		t.Fatalf("attempts=%d body=%q", attempts, rec.Body.String())
	}
}

func TestProxyBalancersAndPathHelpers(t *testing.T) {
	firstURL := mustParseProxyURLHardening(t, "http://first.example.test/base/")
	secondURL := mustParseProxyURLHardening(t, "http://second.example.test")
	targets := []*ProxyTarget{
		{Name: "first", URL: firstURL},
		{Name: "second", URL: secondURL},
	}

	roundRobin := NewRoundRobinBalancer(targets)
	for _, want := range []string{"first", "second", "first"} {
		target, err := roundRobin.Next(nil)
		if err != nil {
			t.Fatal(err)
		}
		if target.Name != want {
			t.Fatalf("round robin target=%q want %q", target.Name, want)
		}
	}

	random := NewRandomBalancer(targets)
	target, err := random.Next(nil)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "first" && target.Name != "second" {
		t.Fatalf("random target=%q", target.Name)
	}

	assertPanicHardening(t, func() {
		_ = NewRoundRobinBalancer(nil)
	})
	emptyRandom := &randomProxyBalancer{}
	if _, err := emptyRandom.Next(nil); err == nil {
		t.Fatal("expected random balancer with no targets to fail")
	}

	targetURL := mustParseProxyURLHardening(t, "http://example.test/api/")
	requestURL := mustParseProxyURLHardening(t, "http://example.test/users")
	path, rawPath := joinProxyPaths(targetURL, requestURL)
	if path != "/api/users" || rawPath != "" {
		t.Fatalf("joined path=%q raw=%q", path, rawPath)
	}
	if got := singleJoiningSlash("/api", "users"); got != "/api/users" {
		t.Fatalf("single joining slash=%q", got)
	}
	if got := singleJoiningSlash("/api/", "/users"); got != "/api/users" {
		t.Fatalf("double slash join=%q", got)
	}
}

func TestProxyNormalizeAndModifyResponseBranches(t *testing.T) {
	targetURL := mustParseProxyURLHardening(t, "http://example.test")
	original := &ProxyTarget{Name: "copy", URL: targetURL}
	normalized := normalizeProxyTargets([]*ProxyTarget{original})
	original.Name = "changed"
	original.URL.Host = "mutated.example.test"
	if normalized[0].Name != "copy" || normalized[0].URL.Host != "example.test" {
		t.Fatalf("normalized target was not cloned: %+v", normalized[0])
	}

	assertPanicHardening(t, func() {
		normalizeOptionalProxyTargets([]*ProxyTarget{{URL: &url.URL{Path: "/relative"}}})
	})

	var calls []string
	firstErr := errors.New("first failed")
	chained := chainProxyModifyResponse(
		func(*http.Response) error {
			calls = append(calls, "first")
			return nil
		},
		func(*http.Response) error {
			calls = append(calls, "second")
			return nil
		},
	)
	if err := chained(&http.Response{}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "first,second" {
		t.Fatalf("calls=%v", calls)
	}

	chained = chainProxyModifyResponse(
		func(*http.Response) error { return firstErr },
		func(*http.Response) error {
			t.Fatal("second modify should not run after first error")
			return nil
		},
	)
	if err := chained(&http.Response{}); !errors.Is(err, firstErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestJaegerGeneratedTraceAndObserverError(t *testing.T) {
	observeErr := errors.New("observe")
	app := zinc.New()
	app.Use(Jaeger(func(*zinc.Context, JaegerSpan) error {
		return observeErr
	}))
	app.Get("/trace", func(c *zinc.Context) error {
		span, ok := JaegerCurrent(c)
		if !ok {
			t.Fatal("missing span")
		}
		if len(span.TraceID) != 32 || len(span.SpanID) != 16 {
			t.Fatalf("span=%+v", span)
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/trace", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCasbinSubjectHelpers(t *testing.T) {
	app := zinc.New()
	app.Use(BasicAuth(BasicAuthStatic("alice", "secret")))
	app.Use(func(c *zinc.Context) error {
		c.Set("object", "doc")
		return c.Next()
	})
	enforcer := &casbinStub{allow: true}
	app.Use(CasbinAuthWithConfig(CasbinAuthConfig{
		Enforcer: enforcer,
		Subject:  CasbinSubjectFromBasicAuth(),
		Object:   CasbinSubjectFromContext("object"),
		Action: func(*zinc.Context) any {
			return "read"
		},
	}))
	app.Get("/docs", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	req.SetBasicAuth("alice", "secret")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(enforcer.args) != 3 || enforcer.args[0] != "alice" || enforcer.args[1] != "doc" || enforcer.args[2] != "read" {
		t.Fatalf("args=%v", enforcer.args)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func mustParseProxyURLHardening(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return parsed
}

func mustNoErrHardening(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertPanicHardening(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
