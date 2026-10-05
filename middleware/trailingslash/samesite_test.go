// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package trailingslash

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/0mjs/zinc"
)

// A redirect built from the request path stays on the same site.
func TestTrailingSlashRedirectStaysOnSameSite(t *testing.T) {
	cases := []struct {
		name   string
		add    bool
		target string
		want   string
	}{
		{"encoded slash", false, "/%2Fevil.example/", "/%2Fevil.example"},
		{"lower-case encoded slash", false, "/%2fevil.example/", "/%2fevil.example"},
		{"encoded backslash", false, "/%5Cevil.example/", "/%5Cevil.example"},
		{"repeated leading slashes", false, "//evil.example/", "/evil.example"},
		{"many leading slashes", false, "////evil.example/x/", "/evil.example/x"},
		{"raw backslash", false, "/\\evil.example/", "/%5Cevil.example"},
		{"encoded control characters", false, "/%0D%0Aevil/", "/%0D%0Aevil"},
		{"query kept", false, "/%2Fevil.example/?next=//other.example", "/%2Fevil.example?next=//other.example"},
		{"add mode encoded slash", true, "/%2Fevil.example", "/%2Fevil.example/"},
		{"add mode repeated slashes", true, "//evil.example", "/evil.example/"},
		{"add mode raw backslash", true, "/\\evil.example?a=1", "/%5Cevil.example/?a=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := zinc.New()
			app.Use(New(Config{Add: tc.add, Redirect: true}))
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

func TestTrailingSlashRedirectKeepsOrdinaryPaths(t *testing.T) {
	cases := []struct {
		add          bool
		target, want string
	}{
		{false, "/users/", "/users"},
		{false, "/users/?page=2", "/users?page=2"},
		{false, "/a%20b/", "/a%20b"},
		{false, "/files/a%2Fb/", "/files/a%2Fb"},
		{true, "/users", "/users/"},
		{true, "/users?page=2", "/users/?page=2"},
	}
	for _, tc := range cases {
		app := zinc.New()
		app.Use(New(Config{Add: tc.add, Redirect: true}))
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
		if got := rec.Header().Get(zinc.HeaderLocation); rec.Code != http.StatusMovedPermanently || got != tc.want {
			t.Fatalf("%s: status=%d location=%q want %q", tc.target, rec.Code, got, tc.want)
		}
	}
}

// An encoded trailing slash is part of the last segment, so redirecting to
// the same escaped path would loop; the request is routed instead.
func TestTrailingSlashRedirectEncodedTrailingSlashDoesNotLoop(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Redirect: true}))
	app.Get("/a", func(c *zinc.Context) error { return c.String("a") })
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/a%2F", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "a" {
		t.Fatalf("status=%d location=%q body=%q", rec.Code, rec.Header().Get(zinc.HeaderLocation), rec.Body.String())
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
