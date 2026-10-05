// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package shared

import (
	"net/url"
	"testing"
)

// A redirect built from the request path stays on the same site, even when
// the path reaches the helper unescaped.
func TestSameSitePath(t *testing.T) {
	cases := map[string]string{
		"":                  "/",
		"/":                 "/",
		"/users":            "/users",
		"//evil.example":    "/evil.example",
		"/\\evil.example":   "/%5Cevil.example",
		"\\\\evil.example":  "/%5C%5Cevil.example",
		"/\\/evil.example":  "/%5C/evil.example",
		"evil.example":      "/evil.example",
		"https:evil":        "/https:evil",
		"/a\r\nSet-Cookie:": "/a%0D%0ASet-Cookie:",
		"/a b\x00\x7f":      "/a%20b%00%7F",
		"/%2Fevil.example":  "/%2Fevil.example",
	}
	for in, want := range cases {
		if got := SameSitePath(in); got != want {
			t.Errorf("SameSitePath(%q)=%q want %q", in, got, want)
		}
	}
}

func TestEscapedTail(t *testing.T) {
	cases := []struct {
		raw  string
		n    int
		want string
	}{
		{"/old/%2Fevil.example/steal", len("/evil.example/steal"), "%2Fevil.example/steal"},
		{"/ol%64/rest", len("rest"), "rest"},
		{"/old/a%20b", len("a b"), "a%20b"},
		{"/old/a", 0, ""},
	}
	for _, tc := range cases {
		u, err := url.ParseRequestURI(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := EscapedTail(u, tc.n); got != tc.want {
			t.Errorf("EscapedTail(%q, %d)=%q want %q", tc.raw, tc.n, got, tc.want)
		}
	}
}

func TestExternalTarget(t *testing.T) {
	for to, want := range map[string]bool{
		"https://example.com/*": true,
		"//cdn.example/*":       true,
		"mailto:a@example.com":  true,
		"/*":                    false,
		"*":                     false,
		"":                      false,
		"/new/*":                false,
		"/a:b":                  false,
	} {
		if got := ExternalTarget(to); got != want {
			t.Errorf("ExternalTarget(%q)=%v want %v", to, got, want)
		}
	}
}
