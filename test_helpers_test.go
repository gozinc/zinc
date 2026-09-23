// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mustDo(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustPanic(t *testing.T, contains string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		if contains != "" && !strings.Contains(fmt.Sprint(recovered), contains) {
			t.Fatalf("panic=%v", recovered)
		}
	}()
	fn()
}

func performRequest(t *testing.T, app *App, method, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	return resp
}

func newRecorderContext(t *testing.T, req *http.Request) (*Context, *httptest.ResponseRecorder) {
	t.Helper()
	resp := httptest.NewRecorder()
	ctx := NewContext(resp, req)
	ctx.app = New()
	return ctx, resp
}
