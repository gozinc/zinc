// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package proxy

import (
	"regexp"
	"testing"

	"github.com/0mjs/zinc"
)

// The upstream must receive the request path with its escaping: an encoded
// slash, space or percent sign names a different resource than its decoded
// form.
func TestProxyKeepsEncodedPath(t *testing.T) {
	for _, tc := range []struct {
		name, target, in, want string
	}{
		{"plain path", "http://upstream.example/base", "/objects/a/b", "/base/objects/a/b"},
		{"plain path, no target path", "http://upstream.example", "/objects/a", "/objects/a"},
		{"encoded slash", "http://upstream.example", "/objects/a%2Fb", "/objects/a%2Fb"},
		{"encoded slash under a target path", "http://upstream.example/base/", "/objects/a%2Fb", "/base/objects/a%2Fb"},
		{"escaped target prefix", "http://upstream.example/base%2Froot", "/objects/a%2Fb", "/base%2Froot/objects/a%2Fb"},
		{"escaped target prefix, plain request", "http://upstream.example/base%2Froot", "/objects/a", "/base%2Froot/objects/a"},
		{"space", "http://upstream.example/base", "/files/a%20b", "/base/files/a%20b"},
		{"percent sign", "http://upstream.example/base", "/files/100%25", "/base/files/100%25"},
		{"encoded percent-two-F", "http://upstream.example", "/files/a%252Fb", "/files/a%252Fb"},
		{"target ends in an escaped slash", "http://upstream.example/base%2F", "/x", "/base%2F/x"},
		{"query kept", "http://upstream.example/base?k=1", "/objects/a%2Fb?x=2", "/base/objects/a%2Fb?k=1&x=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			app := zinc.New()
			app.Use(New(Config{Target: tc.target, Transport: captureTransport(&got)}))
			if uri := proxiedURI(t, app, &got, tc.in); uri != tc.want {
				t.Fatalf("upstream RequestURI=%q want %q", uri, tc.want)
			}
		})
	}
}

// A rewrite rule's target is a decoded path, escaped as usual; the part of
// the request a "*" carries over keeps its escaping.
func TestProxyRewriteKeepsEncodedTail(t *testing.T) {
	for _, tc := range []struct {
		name, target, in, want string
		rules                  map[string]string
	}{
		{"plain tail", "http://upstream.example", "/api/users/42", "/v1/users/42", map[string]string{"/api/*": "/v1/*"}},
		{"encoded slash in tail", "http://upstream.example", "/api/a%2Fb", "/v1/a%2Fb", map[string]string{"/api/*": "/v1/*"}},
		{"space and percent in tail", "http://upstream.example", "/api/a%20b/100%25", "/v1/a%20b/100%25", map[string]string{"/api/*": "/v1/*"}},
		{"star mid-target", "http://upstream.example", "/api/a%2Fb", "/v1/a%2Fb/meta", map[string]string{"/api/*": "/v1/*/meta"}},
		{"no star appends tail", "http://upstream.example", "/api/a%2Fb", "/v1/a%2Fb", map[string]string{"/api/*": "/v1/"}},
		{"target text is escaped", "http://upstream.example", "/api/a%2Fb", "/my%20files/a%2Fb", map[string]string{"/api/*": "/my files/*"}},
		{"under escaped target prefix", "http://upstream.example/base%2Froot", "/api/a%2Fb", "/v1/a%2Fb", map[string]string{"/base/root/api/*": "/v1/*"}},
		{"exact rule", "http://upstream.example", "/api/a%2Fb", "/fixed", map[string]string{"/api/a/b": "/fixed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			app := zinc.New()
			app.Use(New(Config{Target: tc.target, Rewrite: tc.rules, Transport: captureTransport(&got)}))
			if uri := proxiedURI(t, app, &got, tc.in); uri != tc.want {
				t.Fatalf("upstream RequestURI=%q want %q", uri, tc.want)
			}
		})
	}
}

// A regular expression matches the decoded path. The result keeps the
// request's escaping when the same rule, applied to the escaped path, gives
// the same path; otherwise it is escaped as usual.
func TestProxyRegexRewriteEscaping(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"plain", "/api/users/42", "/v1/users/42"},
		{"encoded slash kept", "/api/files/a%2Fb", "/v1/files/a%2Fb"},
		{"space kept", "/api/files/a%20b", "/v1/files/a%20b"},
		// ^/api/(.+)$ doesn't match the escaped "/api%2Fx/a%2Fb", so the
		// result falls back to standard escaping.
		{"no escaped match falls back", "/api%2Fx/a%2Fb", "/v1/x/a/b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			app := zinc.New()
			app.Use(New(Config{
				Target:       "http://upstream.example",
				RegexRewrite: map[*regexp.Regexp]string{regexp.MustCompile(`^/api/(.+)$`): "/v1/$1"},
				Transport:    captureTransport(&got),
			}))
			if uri := proxiedURI(t, app, &got, tc.in); uri != tc.want {
				t.Fatalf("upstream RequestURI=%q want %q", uri, tc.want)
			}
		})
	}
}
