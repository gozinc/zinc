// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A mount above a built-in's path, such as Static at /, leaves the built-in
// answering its own path; FindRoute and Routes agree with dispatch.
func TestBuiltinsAnswerUnderStaticRoot(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "home", "app.js": "js", "docs.txt": "file"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := New()
	app.Static("/", dir)
	t.Cleanup(func() { _ = app.Close() })

	if rec := performRequest(t, app, "GET", "/openapi.json", nil, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"openapi"`) {
		t.Fatalf("spec under Static /: %d %.80s", rec.Code, rec.Body)
	}
	if rec := performRequest(t, app, "HEAD", "/openapi.json", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("HEAD spec under Static /: %d", rec.Code)
	}
	if rec := performRequest(t, app, "GET", "/docs", nil, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-spec="/openapi.json"`) {
		t.Fatalf("docs under Static /: %d %.80s", rec.Code, rec.Body)
	}
	for path, want := range map[string]string{"/": "home", "/app.js": "js", "/docs.txt": "file"} {
		if rec := performRequest(t, app, "GET", path, nil, nil); rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Body)
		}
	}
	for _, path := range []string{"/openapi.json", "/docs"} {
		if info, ok := app.FindRoute("GET", path); !ok || !info.Builtin {
			t.Errorf("FindRoute %s: %+v %v", path, info, ok)
		}
	}
	if info, ok := app.FindRoute("GET", "/app.js"); !ok || !info.Mounted {
		t.Errorf("FindRoute /app.js: %+v %v", info, ok)
	}
	builtins := 0
	for _, r := range app.Routes() {
		if r.Builtin {
			builtins++
		}
	}
	if builtins != 2 {
		t.Errorf("Routes lists %d built-ins, want 2", builtins)
	}

	// A route at the path still wins over the built-in.
	own := New()
	own.Static("/", dir)
	own.Get("/openapi.json", func(c *Context) error { return c.String("mine") })
	t.Cleanup(func() { _ = own.Close() })
	if rec := performRequest(t, own, "GET", "/openapi.json", nil, nil); rec.Body.String() != "mine" {
		t.Fatalf("own route under Static /: %s", rec.Body)
	}
	if info, ok := own.FindRoute("GET", "/openapi.json"); !ok || info.Builtin || info.Mounted {
		t.Fatalf("FindRoute own route: %+v %v", info, ok)
	}

	// A plain Mount at / gives way the same way, and so does a group's.
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "mux") })
	mounted := New()
	mounted.Mount("/", mux)
	grouped := New()
	grouped.Group("/").Mount("/", mux)
	for name, a := range map[string]*App{"Mount": mounted, "Group.Mount": grouped} {
		if rec := performRequest(t, a, "GET", "/docs", nil, nil); !strings.Contains(rec.Body.String(), "data-spec") {
			t.Errorf("docs under %s /: %s", name, rec.Body)
		}
		if rec := performRequest(t, a, "GET", "/other", nil, nil); rec.Body.String() != "mux" {
			t.Errorf("%s /: %s", name, rec.Body)
		}
		// Other methods on a built-in's path still reach the mount.
		if rec := performRequest(t, a, "POST", "/docs", nil, nil); rec.Body.String() != "mux" {
			t.Errorf("POST /docs under %s /: %s", name, rec.Body)
		}
	}
}

// A mount at exactly a built-in's path claims it, as a route there would.
func TestMountAtBuiltinPathClaimsIt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("site"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := New()
	app.Static("/docs", dir)
	t.Cleanup(func() { _ = app.Close() })
	if rec := performRequest(t, app, "GET", "/docs", nil, nil); rec.Body.String() != "site" {
		t.Fatalf("Static /docs: %d %q", rec.Code, rec.Body)
	}
	if info, ok := app.FindRoute("GET", "/docs"); !ok || !info.Mounted {
		t.Fatalf("FindRoute /docs: %+v %v", info, ok)
	}
	for _, r := range app.Routes() {
		if r.Builtin && r.Path == "/docs" {
			t.Fatal("Routes lists the claimed /docs built-in")
		}
	}
	if rec := performRequest(t, app, "GET", "/openapi.json", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("spec: %d", rec.Code)
	}
}
