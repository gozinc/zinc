// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package shared

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestResponseStatus(t *testing.T) {
	rw := zinc.WrapResponseWriter(httptest.NewRecorder())
	if got := ResponseStatus(rw, nil); got != http.StatusOK {
		t.Fatalf("status with unwritten writer=%d", got)
	}

	rw.WriteHeader(http.StatusAccepted)
	if got := ResponseStatus(rw, errors.New("ignored")); got != http.StatusAccepted {
		t.Fatalf("status with written writer=%d", got)
	}

	wrappedHTTPError := fmt.Errorf("wrapped: %w", zinc.NewError(http.StatusTeapot))
	if got := ResponseStatus(nil, wrappedHTTPError); got != http.StatusTeapot {
		t.Fatalf("status from wrapped HTTPError=%d", got)
	}

	if got := ResponseStatus(nil, errors.New("boom")); got != http.StatusInternalServerError {
		t.Fatalf("status from non-http error=%d", got)
	}
}

func TestRewriteTarget(t *testing.T) {
	if target, ok := RewriteTarget("/x/path", map[string]string{"/x/*": "/y"}); !ok || target != "/ypath" {
		t.Fatalf("target=%q ok=%v", target, ok)
	}
	if target, ok := RewriteTarget("/x/path", map[string]string{"/z/*": "/y/*"}); ok || target != "" {
		t.Fatalf("target=%q ok=%v", target, ok)
	}
	if got := PathWithRawQuery("/users", ""); got != "/users" {
		t.Fatalf("path without query=%q", got)
	}
	if got := PathWithRawQuery("/users", "a=1"); got != "/users?a=1" {
		t.Fatalf("path with query=%q", got)
	}
}
