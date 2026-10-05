// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package shared

import "testing"

func TestRulesPrecedence(t *testing.T) {
	rules := map[string]string{
		"":            "/dropped",
		"/*":          "/global/*",
		"/api/*":      "/specific/*",
		"/api/pets":   "/exact",
		"/api/pets/*": "/pets/*",
		"/a*":         "/a-prefix/*",
	}
	for build := 0; build < 50; build++ {
		compiled := CompileRules(rules)
		for _, tc := range []struct{ in, want string }{
			{"/api/pets", "/exact"},
			{"/api/pets/1", "/pets/1"},
			{"/api/x", "/specific/x"},
			{"/api", "/a-prefix/pi"},
			{"/b", "/global/b"},
		} {
			for i := 0; i < 100; i++ {
				got, ok := compiled.Rewrite(tc.in)
				if !ok || got != tc.want {
					t.Fatalf("build %d: %s → %q, %v want %q", build, tc.in, got, ok, tc.want)
				}
			}
		}
	}
}

func TestRulesNoMatch(t *testing.T) {
	compiled := CompileRules(map[string]string{"/x/*": "/y/*", "/exact": "/e"})
	if got, ok := compiled.Rewrite("/z"); ok || got != "" {
		t.Fatalf("got %q, %v", got, ok)
	}
	if got, ok := CompileRules(nil).Rewrite("/z"); ok || got != "" {
		t.Fatalf("empty rules got %q, %v", got, ok)
	}
	if compiled.Len() != 2 || CompileRules(nil).Len() != 0 {
		t.Fatal("Len counts compiled rules")
	}
}
