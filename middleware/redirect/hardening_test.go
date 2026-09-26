// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package redirect

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRedirectWildcardRule(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Rules: map[string]string{"/old/*": "/new/*"}}))

	req := httptest.NewRequest(http.MethodGet, "/old/path", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if got := rec.Header().Get(zinc.HeaderLocation); got != "/new/path" {
		t.Fatalf("location=%q", got)
	}
}
