// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMiscHelpersCoverage(t *testing.T) {
	if got := normalizeRegisteredPrefix(""); got != "/" {
		t.Fatalf("prefix=%q", got)
	}
	if got := normalizeRegisteredPrefix("api/"); got != "/api" {
		t.Fatalf("prefix=%q", got)
	}
	if !pathHasPrefix("/api/users", "/api", true) {
		t.Fatal("expected path prefix match")
	}
	if pathHasPrefix("/apix", "/api", true) {
		t.Fatal("unexpected path prefix match")
	}
	if got := storedPrefix("/API", false); got != "/api" {
		t.Fatalf("stored prefix=%q", got)
	}
	if got := storedPrefix("/API", true); got != "/API" {
		t.Fatalf("stored prefix=%q", got)
	}
	if got := normalizeRegisteredPrefix("//"); got != "/" {
		t.Fatalf("prefix=%q", got)
	}
	if !pathHasPrefix("/anything", "/", true) {
		t.Fatal("root prefix should always match")
	}
	if !pathHasPrefix("/api", "/api", true) {
		t.Fatal("exact path prefix should match")
	}
	if !pathHasPrefix("/api/users", "/api/", true) {
		t.Fatal("trailing slash prefix should match")
	}
	if pathHasPrefix("/apix", "/api", false) {
		t.Fatal("segment boundary should be required")
	}

	if clone := cloneURL(nil); clone == nil {
		t.Fatal("cloneURL(nil) should return empty URL")
	}
	if got := cloneRequestURI(nil); got != "" {
		t.Fatalf("request URI=%q", got)
	}

	router := &Router{config: &DefaultConfig}
	mustDo(t, router.Add(MethodGet, "/lookup/{id}", func(c *Context) error { return c.String(c.Param("id")) }))
	handler, ctx := router.Find(MethodGet, "/lookup/44")
	if handler == nil || ctx == nil || ctx.Param("id") != "44" {
		t.Fatalf("find failed: handler=%v ctx=%v", handler, ctx)
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{"name": []string{"value"}}.Encode()))
	req.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
	ctx2, _ := newRecorderContext(t, req)
	defer ctx2.release()
	if got := ctx2.FormValue("name"); got != "value" {
		t.Fatalf("form value=%q", got)
	}
}
