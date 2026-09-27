// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// asciiFoldKindSlow is the byte-at-a-time definition asciiFoldKind must match.
func asciiFoldKindSlow(path string) (hasUpper, ascii bool) {
	for i := 0; i < len(path); i++ {
		if path[i] >= utf8.RuneSelf {
			return false, false
		}
		if path[i] >= 'A' && path[i] <= 'Z' {
			hasUpper = true
		}
	}
	return hasUpper, true
}

func TestASCIIFoldKindMatchesByteScan(t *testing.T) {
	var cases []string
	for b := 0; b < 256; b++ {
		for pos := 0; pos < 17; pos++ {
			cases = append(cases, strings.Repeat("a", pos)+string([]byte{byte(b)})+strings.Repeat("z", 16-pos))
		}
	}
	cases = append(cases, "", "/", "/users/AbC123", "/users/abc123", "/é", "/users/42/@[`{")
	for _, c := range cases {
		gu, ga := asciiFoldKind(c)
		wu, wa := asciiFoldKindSlow(c)
		if !wa {
			wu = false // the caller ignores hasUpper for non-ASCII paths
			gu = gu && ga
		}
		if gu != wu || ga != wa {
			t.Fatalf("asciiFoldKind(%q) = %v, %v; want %v, %v", c, gu, ga, wu, wa)
		}
	}
}

func FuzzASCIIFoldKind(f *testing.F) {
	f.Add("/users/AbC123")
	f.Fuzz(func(t *testing.T, s string) {
		gu, ga := asciiFoldKind(s)
		wu, wa := asciiFoldKindSlow(s)
		if ga != wa || (wa && gu != wu) {
			t.Fatalf("asciiFoldKind(%q) = %v, %v; want %v, %v", s, gu, ga, wu, wa)
		}
	})
}

func BenchmarkASCIIFoldKind(b *testing.B) {
	path := "/api/v1/teams/100042/users/100007/preferences"
	for i := 0; i < b.N; i++ {
		asciiFoldKind(path)
	}
}
