// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package redirect

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRedirectOverlappingRulesAreDeterministic(t *testing.T) {
	for build := 0; build < 50; build++ {
		app := zinc.New()
		app.Use(New(Config{Rules: map[string]string{
			"/*":        "/global/*",
			"/api/*":    "/specific/*",
			"/api/pets": "/exact",
		}}))
		for _, tc := range []struct {
			in, want string
			n        int
		}{
			{"/api/pets/1", "/specific/pets/1", 1000},
			{"/api/pets", "/exact", 100},
			{"/other", "/global/other", 100},
		} {
			for i := 0; i < tc.n; i++ {
				rec := httptest.NewRecorder()
				app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.in, nil))
				if got := rec.Header().Get(zinc.HeaderLocation); got != tc.want {
					t.Fatalf("build %d request %d: %s → %q want %q", build, i, tc.in, got, tc.want)
				}
			}
		}
	}
}
