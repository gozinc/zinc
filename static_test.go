// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticFSHonorsIndexAndBrowse(t *testing.T) {
	fsys := fstest.MapFS{
		"home.html":          &fstest.MapFile{Data: []byte("root-home")},
		"docs/home.html":     &fstest.MapFile{Data: []byte("docs-home")},
		"browse/one.txt":     &fstest.MapFile{Data: []byte("one")},
		"browse/sub/two.txt": &fstest.MapFile{Data: []byte("two")},
	}

	app := New()
	mustDo(t, app.StaticFS("/assets", fsys, WithStaticBrowse(true), WithStaticIndex("home.html")))

	root := performRequest(t, app, http.MethodGet, "/assets", nil, nil)
	if root.Code != http.StatusOK {
		t.Fatalf("root status=%d", root.Code)
	}
	if body := root.Body.String(); body != "root-home" {
		t.Fatalf("root body=%q", body)
	}

	docs := performRequest(t, app, http.MethodGet, "/assets/docs", nil, nil)
	if docs.Code != http.StatusOK {
		t.Fatalf("docs status=%d", docs.Code)
	}
	if body := docs.Body.String(); body != "docs-home" {
		t.Fatalf("docs body=%q", body)
	}

	browse := performRequest(t, app, http.MethodGet, "/assets/browse", nil, nil)
	if browse.Code != http.StatusOK {
		t.Fatalf("browse status=%d", browse.Code)
	}
	if got := browse.Header().Get(HeaderContentType); got != htmlType {
		t.Fatalf("browse content-type=%q", got)
	}
	if body := browse.Body.String(); !strings.Contains(body, "one.txt") || !strings.Contains(body, "sub/") {
		t.Fatalf("browse body=%q", body)
	}
}

func TestStaticFSBrowseDisabledWithoutIndexReturnsNotFound(t *testing.T) {
	fsys := fstest.MapFS{
		"browse/one.txt": &fstest.MapFile{Data: []byte("one")},
	}

	app := New()
	mustDo(t, app.StaticFS("/assets", fsys))

	resp := performRequest(t, app, http.MethodGet, "/assets/browse", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d", resp.Code)
	}
}

func TestStaticFSRejectsTraversal(t *testing.T) {
	fsys := fstest.MapFS{
		"safe.txt": &fstest.MapFile{Data: []byte("safe")},
	}

	app := New()
	mustDo(t, app.StaticFS("/assets", fsys, WithStaticBrowse(true)))

	resp := performRequest(t, app, http.MethodGet, "/assets/%2e%2e/safe.txt", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d", resp.Code)
	}
}

func TestStaticUsesDirectoryFSAndCustomIndex(t *testing.T) {
	dir := t.TempDir()
	mustDo(t, os.WriteFile(filepath.Join(dir, "index.htm"), []byte("custom-root"), 0o644))

	app := New()
	mustDo(t, app.Static("/public", dir, WithStaticIndex("index.htm")))

	resp := performRequest(t, app, http.MethodGet, "/public", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d", resp.Code)
	}
	if body := resp.Body.String(); body != "custom-root" {
		t.Fatalf("body=%q", body)
	}
}

func TestServeOpenedStaticFileReadAllFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	rec := httptest.NewRecorder()
	err := serveOpenedStaticFile(rec, req, &memoryFile{
		data: []byte("fallback-static"),
		info: fileInfoStub{name: "asset.txt"},
	}, fileInfoStub{name: "asset.txt"})
	mustDo(t, err)
	if rec.Body.String() != "fallback-static" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
