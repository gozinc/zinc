// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/0mjs/zinc"
)

func TestStaticMiddlewareConstructors(t *testing.T) {
	dir := t.TempDir()
	mustNoErrStatic(t, os.WriteFile(filepath.Join(dir, "asset.txt"), []byte("asset"), 0o644))
	mustNoErrStatic(t, os.Mkdir(filepath.Join(dir, "prefix"), 0o755))
	mustNoErrStatic(t, os.WriteFile(filepath.Join(dir, "prefix", "nested.txt"), []byte("nested"), 0o644))

	app := zinc.New()
	app.Use(Static(dir))
	req := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "asset" {
		t.Fatalf("static body=%q", rec.Body.String())
	}

	app = zinc.New()
	app.Use(StaticFrom("/files", dir))
	req = httptest.NewRequest(http.MethodGet, "/files/prefix/nested.txt", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "nested" {
		t.Fatalf("static from body=%q", rec.Body.String())
	}
}

func TestStaticMiddlewareConfigBranches(t *testing.T) {
	fsys := fstest.MapFS{
		"public.txt": &fstest.MapFile{Data: []byte("public")},
	}

	app := zinc.New()
	app.Use(StaticWithConfig(StaticConfig{
		Filesystem:     fsys,
		Prefix:         "/assets/",
		NextOnNotFound: true,
	}))
	app.Get("/assets/missing.txt", func(c *zinc.Context) error {
		return c.String("next")
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/public.txt", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "public" {
		t.Fatalf("static fs body=%q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/missing.txt", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "next" {
		t.Fatalf("next body=%q", rec.Body.String())
	}

	assertPanicStatic(t, func() {
		_ = StaticWithConfig(StaticConfig{})
	})
}

func mustNoErrStatic(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertPanicStatic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
