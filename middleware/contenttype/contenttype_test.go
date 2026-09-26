// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package contenttype

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestContentType(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Types: []string{"application/json"}, Encodings: []string{"identity", "gzip"}}))
	app.Post("/", func(c *zinc.Context) error { return c.String("ok") })

	cases := []struct {
		name, contentType, encoding string
		status                      int
	}{
		{"parameters ignored", "application/json; charset=utf-8", "", http.StatusOK},
		{"listed encoding", "application/json", "gzip", http.StatusOK},
		{"missing type", "", "", http.StatusUnsupportedMediaType},
		{"other type", "text/plain", "", http.StatusUnsupportedMediaType},
		{"unlisted encoding", "application/json", "br", http.StatusUnsupportedMediaType},
		{"one unlisted coding in a list", "application/json", "gzip, br", http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if tc.contentType != "" {
			req.Header.Set(zinc.HeaderContentType, tc.contentType)
		}
		if tc.encoding != "" {
			req.Header.Set(zinc.HeaderContentEncoding, tc.encoding)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("%s: status=%d, want %d", tc.name, rec.Code, tc.status)
		}
	}
}

func TestContentTypeEmptyAllowsAnything(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Post("/", func(c *zinc.Context) error { return c.String("ok") })
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(zinc.HeaderContentEncoding, "br")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}
