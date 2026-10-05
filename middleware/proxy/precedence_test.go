// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

// captureTransport answers every request itself and records the outbound
// request URI, so a test sees exactly what an upstream would receive.
func captureTransport(got *string) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		*got = req.URL.RequestURI()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})
}

func proxiedURI(t *testing.T, app *zinc.App, got *string, target string) string {
	t.Helper()
	*got = ""
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status=%d", target, rec.Code)
	}
	return *got
}

func TestProxyOverlappingRewriteRulesAreDeterministic(t *testing.T) {
	for build := 0; build < 50; build++ {
		var got string
		app := zinc.New()
		app.Use(New(Config{
			Target: "http://upstream.example",
			Rewrite: map[string]string{
				"/*":        "/global/*",
				"/api/*":    "/specific/*",
				"/api/pets": "/exact",
			},
			Transport: captureTransport(&got),
		}))
		for _, tc := range []struct {
			in, want string
			n        int
		}{
			{"/api/pets/1", "/specific/pets/1", 1000},
			{"/api/pets", "/exact", 100},
			{"/other", "/global/other", 100},
		} {
			for i := 0; i < tc.n; i++ {
				if uri := proxiedURI(t, app, &got, tc.in); uri != tc.want {
					t.Fatalf("build %d request %d: %s → %q want %q", build, i, tc.in, uri, tc.want)
				}
			}
		}
	}
}

// Regular expressions have no declared order in a map, so the longest
// pattern is tried first, then patterns in lexical order.
func TestProxyOverlappingRegexRulesAreDeterministic(t *testing.T) {
	for build := 0; build < 50; build++ {
		var got string
		app := zinc.New()
		app.Use(New(Config{
			Target: "http://upstream.example",
			RegexRewrite: map[*regexp.Regexp]string{
				regexp.MustCompile(`^/(.+)$`):           "/global/$1",
				regexp.MustCompile(`^/api/(.+)$`):       "/specific/$1",
				regexp.MustCompile(`^/api/pets/(\d+)$`): "/pets/$1",
				regexp.MustCompile(`^/a(.*)$`):          "/a-lexical/$1",
				regexp.MustCompile(`^/(a.*)$`):          "/paren/$1",
			},
			Transport: captureTransport(&got),
		}))
		for _, tc := range []struct {
			in, want string
			n        int
		}{
			{"/api/pets/7", "/pets/7", 1000},
			{"/api/pets/x", "/specific/pets/x", 100},
			{"/other", "/global/other", 100},
			// ^/(a.*)$ and ^/a(.*)$ tie on length; "(" sorts before "a".
			{"/abc", "/paren/abc", 100},
		} {
			for i := 0; i < tc.n; i++ {
				if uri := proxiedURI(t, app, &got, tc.in); uri != tc.want {
					t.Fatalf("build %d request %d: %s → %q want %q", build, i, tc.in, uri, tc.want)
				}
			}
		}
	}
}

func TestProxyRegexRewriteRejectsDuplicatePatterns(t *testing.T) {
	assertPanicHardening(t, func() {
		New(Config{
			Target: "http://upstream.example",
			RegexRewrite: map[*regexp.Regexp]string{
				regexp.MustCompile(`^/x$`): "/one",
				regexp.MustCompile(`^/x$`): "/two",
			},
		})
	})
}
