// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestProxyConfigHooks(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Director", r.Header.Get("X-Director"))
		_, _ = w.Write([]byte("upstream"))
	}))
	defer upstream.Close()

	app := zinc.New()
	app.Use(New(Config{
		Target: upstream.URL,
		Director: func(req *http.Request) {
			req.Header.Set("X-Director", "set")
		},
		ModifyResponse: func(resp *http.Response) error {
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
	app.Use(New(Config{
		Targets: []*Target{
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
	app.Use(New(Config{
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
	app.Use(New(Config{
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
	targets := []*Target{
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
	original := &Target{Name: "copy", URL: targetURL}
	normalized := normalizeProxyTargets([]*Target{original})
	original.Name = "changed"
	original.URL.Host = "mutated.example.test"
	if normalized[0].Name != "copy" || normalized[0].URL.Host != "example.test" {
		t.Fatalf("normalized target was not cloned: %+v", normalized[0])
	}

	assertPanicHardening(t, func() {
		normalizeOptionalProxyTargets([]*Target{{URL: &url.URL{Path: "/relative"}}})
	})

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

func assertPanicHardening(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
