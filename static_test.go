// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	app.StaticFS("/assets", fsys, WithStaticBrowse(true), WithStaticIndex("home.html"))

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
	app.StaticFS("/assets", fsys)

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
	app.StaticFS("/assets", fsys, WithStaticBrowse(true))

	resp := performRequest(t, app, http.MethodGet, "/assets/%2e%2e/safe.txt", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d", resp.Code)
	}
}

func TestStaticUsesDirectoryFSAndCustomIndex(t *testing.T) {
	dir := t.TempDir()
	mustDo(t, os.WriteFile(filepath.Join(dir, "index.htm"), []byte("custom-root"), 0o644))

	app := New()
	app.Static("/public", dir, WithStaticIndex("index.htm"))

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

func TestConfinedDirFSReusesRootAndPreservesConfinement(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	mustDo(t, os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe"), 0o600))
	mustDo(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600))
	if err := os.Symlink("safe.txt", filepath.Join(root, "inside")); err != nil {
		t.Skip(err)
	}
	mustDo(t, os.Symlink(outside, filepath.Join(root, "escape")))

	filesystem := &confinedDirFS{path: root}
	for range 3 {
		file, err := filesystem.Open("inside")
		mustDo(t, err)
		data, err := io.ReadAll(file)
		mustDo(t, err)
		mustDo(t, file.Close())
		if string(data) != "safe" {
			t.Fatalf("in-root symlink returned %q", data)
		}
		if _, err := filesystem.Open("escape/secret.txt"); err == nil {
			t.Fatal("symlink escaped retained root")
		}
	}
	retained := filesystem.root
	if retained == nil {
		t.Fatal("root was not retained")
	}
	file, err := filesystem.Open("safe.txt")
	mustDo(t, err)
	mustDo(t, file.Close())
	if filesystem.root != retained {
		t.Fatal("root was reopened on a later request")
	}
	mustDo(t, filesystem.Close())
	mustDo(t, filesystem.Close())
	if _, err := filesystem.Open("safe.txt"); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("open after close: %v", err)
	}
}

func TestStaticRootLifecycleAndLateDirectory(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "late")
	app := New()
	app.Static("/files", root)
	if got := performRequest(t, app, http.MethodGet, "/files/item.txt", nil, nil); got.Code != http.StatusNotFound {
		t.Fatalf("missing root status=%d", got.Code)
	}
	mustDo(t, os.Mkdir(root, 0o700))
	mustDo(t, os.WriteFile(filepath.Join(root, "item.txt"), []byte("item"), 0o600))
	if got := performRequest(t, app, http.MethodGet, "/files/item.txt", nil, nil); got.Code != http.StatusOK || got.Body.String() != "item" {
		t.Fatalf("late root status=%d body=%q", got.Code, got.Body.String())
	}
	group := app.Group("/nested")
	group.Static("/files", root)
	if got := performRequest(t, app, http.MethodGet, "/nested/files/item.txt", nil, nil); got.Code != http.StatusOK {
		t.Fatalf("group static status=%d", got.Code)
	}
	if len(app.staticRoots) != 2 || app.staticRoots[0].root == nil || app.staticRoots[1].root == nil {
		t.Fatalf("static roots not retained: %d", len(app.staticRoots))
	}
	mustDo(t, app.Close())
	mustDo(t, app.Close())
	for _, filesystem := range app.staticRoots {
		if !filesystem.closed || filesystem.root != nil {
			t.Fatal("static root remained open after App.Close")
		}
	}
}

func TestConfinedDirFSConcurrentOpenAndClose(t *testing.T) {
	root := t.TempDir()
	mustDo(t, os.WriteFile(filepath.Join(root, "item.txt"), []byte("item"), 0o600))
	filesystem := &confinedDirFS{path: root}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				file, err := filesystem.Open("item.txt")
				if errors.Is(err, os.ErrClosed) {
					return
				}
				if err != nil {
					t.Errorf("open: %v", err)
					return
				}
				if err := file.Close(); err != nil {
					t.Errorf("close file: %v", err)
					return
				}
			}
		}()
	}
	mustDo(t, filesystem.Close())
	workers.Wait()
}

func TestConfinedDirFSRemainsConfinedDuringSymlinkReplacement(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	mustDo(t, os.Mkdir(filepath.Join(root, "inside"), 0o700))
	mustDo(t, os.WriteFile(filepath.Join(root, "inside", "item.txt"), []byte("inside"), 0o600))
	mustDo(t, os.WriteFile(filepath.Join(outside, "item.txt"), []byte("outside"), 0o600))
	link := filepath.Join(root, "current")
	if err := os.Symlink("inside", link); err != nil {
		t.Skip(err)
	}
	filesystem := &confinedDirFS{path: root}
	defer filesystem.Close()
	file, err := filesystem.Open("current/item.txt")
	mustDo(t, err)
	mustDo(t, file.Close()) // retain the root before replacement begins

	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 100 {
				file, err := filesystem.Open("current/item.txt")
				if err != nil { // the link may be absent between remove and create
					continue
				}
				data, readErr := io.ReadAll(file)
				closeErr := file.Close()
				if readErr != nil || closeErr != nil {
					t.Errorf("read=%v close=%v", readErr, closeErr)
					return
				}
				if string(data) != "inside" {
					t.Errorf("confined root returned %q", data)
					return
				}
			}
		}()
	}
	for i := range 100 {
		mustDo(t, os.Remove(link))
		target := "inside"
		if i%2 == 0 {
			target = outside
		}
		mustDo(t, os.Symlink(target, link))
	}
	readers.Wait()
}

func TestConfinedDirFSPinsOpenedDirectoryAcrossRename(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "public")
	mustDo(t, os.Mkdir(root, 0o700))
	mustDo(t, os.WriteFile(filepath.Join(root, "item.txt"), []byte("original"), 0o600))
	filesystem := &confinedDirFS{path: root}
	defer filesystem.Close()
	first, err := filesystem.Open("item.txt")
	mustDo(t, err)
	mustDo(t, first.Close())

	if err := os.Rename(root, filepath.Join(parent, "old-public")); err != nil {
		t.Skipf("renaming an open directory is unsupported: %v", err)
	}
	mustDo(t, os.Mkdir(root, 0o700))
	mustDo(t, os.WriteFile(filepath.Join(root, "item.txt"), []byte("replacement"), 0o600))
	file, err := filesystem.Open("item.txt")
	mustDo(t, err)
	body, err := io.ReadAll(file)
	mustDo(t, err)
	mustDo(t, file.Close())
	if string(body) != "original" {
		t.Fatalf("retained root switched directories: %q", body)
	}
}
