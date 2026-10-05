// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package redirect

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/0mjs/zinc"
)

// A redirect built from the request path stays on the same site.
func TestRedirectRuleStaysOnSameSite(t *testing.T) {
	cases := []struct {
		name         string
		from, to     string
		target, want string
	}{
		{"encoded slash", "/old/*", "/*", "/old/%2Fevil.example/steal", "/%2Fevil.example/steal"},
		{"lower-case encoded slash", "/old/*", "/*", "/old/%2fevil.example/steal", "/%2fevil.example/steal"},
		{"encoded backslash", "/old/*", "/*", "/old/%5Cevil.example/steal", "/%5Cevil.example/steal"},
		{"repeated slashes", "/old/*", "/*", "/old//evil.example/steal", "/evil.example/steal"},
		{"raw backslash", "/old/*", "/*", "/old/\\evil.example/steal", "/%5Cevil.example/steal"},
		{"encoded control characters", "/old/*", "/*", "/old/%0D%0Aevil", "/%0D%0Aevil"},
		{"bare wildcard target", "/old/*", "*", "/old/%2Fevil.example", "/%2Fevil.example"},
		{"bare wildcard target with scheme", "/old/*", "*", "/old/https:evil.example", "/https:evil.example"},
		{"prefix without wildcard target", "/old*", "", "/old//evil.example", "/evil.example"},
		{"query kept", "/old/*", "/*", "/old/%2Fevil.example?next=//other.example", "/%2Fevil.example?next=//other.example"},
		{"tail keeps its escaping", "/old/*", "/new/*", "/old/a%2Fb/c%20d", "/new/a%2Fb/c%20d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := zinc.New()
			app.Use(New(Config{Rules: map[string]string{tc.from: tc.to}}))
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
			if rec.Code != http.StatusMovedPermanently {
				t.Fatalf("status=%d", rec.Code)
			}
			got := rec.Header().Get(zinc.HeaderLocation)
			assertSameSite(t, got)
			if got != tc.want {
				t.Fatalf("location=%q want %q", got, tc.want)
			}
		})
	}
}

// A target the rule names with a scheme or host is meant to leave the site.
func TestRedirectRuleExternalTargetStillWorks(t *testing.T) {
	cases := []struct {
		from, to     string
		target, want string
	}{
		{"/docs/*", "https://example.com/*", "/docs/intro?x=1", "https://example.com/intro?x=1"},
		{"/docs/*", "https://example.com/*", "/docs/a%2Fb", "https://example.com/a%2Fb"},
		{"/docs/*", "//cdn.example/*", "/docs/app.js", "//cdn.example/app.js"},
		{"/docs", "https://example.com/docs", "/docs", "https://example.com/docs"},
	}
	for _, tc := range cases {
		app := zinc.New()
		app.Use(New(Config{Rules: map[string]string{tc.from: tc.to}}))
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
		if got := rec.Header().Get(zinc.HeaderLocation); rec.Code != http.StatusMovedPermanently || got != tc.want {
			t.Fatalf("%s: status=%d location=%q want %q", tc.target, rec.Code, got, tc.want)
		}
	}
}

func TestRedirectRuleOrdinaryLocalTargets(t *testing.T) {
	cases := []struct {
		from, to     string
		target, want string
	}{
		{"/old", "/new", "/old?x=1", "/new?x=1"},
		{"/old/*", "/new/*", "/old/a/b?x=1", "/new/a/b?x=1"},
		{"/old/*", "/new", "/old/a", "/newa"},
		{"/old/*", "/*", "/old/a", "/a"},
	}
	for _, tc := range cases {
		app := zinc.New()
		app.Use(New(Config{Rules: map[string]string{tc.from: tc.to}}))
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
		if got := rec.Header().Get(zinc.HeaderLocation); rec.Code != http.StatusMovedPermanently || got != tc.want {
			t.Fatalf("%s: status=%d location=%q want %q", tc.target, rec.Code, got, tc.want)
		}
	}
}

func assertSameSite(t *testing.T, location string) {
	t.Helper()
	if len(location) == 0 || location[0] != '/' {
		t.Fatalf("location=%q is not an absolute path", location)
	}
	if len(location) > 1 && (location[1] == '/' || location[1] == '\\') {
		t.Fatalf("location=%q is a network-path reference", location)
	}
	for i := 0; i < len(location); i++ {
		if b := location[i]; b < 0x21 || b == 0x7f || b == '\\' {
			t.Fatalf("location=%q has byte %q", location, b)
		}
	}
	base, _ := url.Parse("https://app.example/here/")
	ref, err := url.Parse(location)
	if err != nil {
		t.Fatalf("location=%q: %v", location, err)
	}
	if got := base.ResolveReference(ref); got.Host != "app.example" || got.Scheme != "https" {
		t.Fatalf("location=%q resolves to %s", location, got)
	}
}
