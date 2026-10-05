// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/0mjs/zinc/internal/marks"
)

// StaticConfig controls directory serving and index behaviour.
type StaticConfig struct {
	Browse bool
	Index  string
}

// StaticOption mutates static-file configuration during registration.
type StaticOption func(*StaticConfig)

// WithStaticBrowse enables or disables directory listings.
func WithStaticBrowse(browse bool) StaticOption {
	return func(cfg *StaticConfig) {
		cfg.Browse = browse
	}
}

// WithStaticIndex changes the filename served for directory requests.
func WithStaticIndex(index string) StaticOption {
	return func(cfg *StaticConfig) {
		cfg.Index = index
	}
}

// Static serves root from the operating-system filesystem below prefix. The
// confined directory handle is retained after its first use and released by
// Shutdown or Close.
func (a *App) Static(prefix, root string, opts ...StaticOption) {
	filesystem := &confinedDirFS{path: root}
	a.StaticFS(prefix, filesystem, opts...)
	a.staticRoots = append(a.staticRoots, filesystem)
}

// StaticFS serves filesystem below prefix. It panics if filesystem is nil.
func (a *App) StaticFS(prefix string, filesystem fs.FS, opts ...StaticOption) {
	a.staticFS(prefix, filesystem, nil, opts...)
}

func (a *App) staticFS(prefix string, filesystem fs.FS, middleware []HandlerFunc, opts ...StaticOption) {
	if filesystem == nil {
		panic("zinc: static filesystem is nil")
	}
	cfg := StaticConfig{Index: "index.html"}
	for _, opt := range opts {
		opt(&cfg)
	}
	a.mountNative(prefix, newStaticHandler(filesystem, cfg), middleware)
}

// File serves one operating-system file at path.
func (a *App) File(path, file string) Route {
	return a.Get(path, func(c *Context) error {
		return c.File(file)
	})
}

// FileFS serves one file from filesystem at path.
func (a *App) FileFS(path, file string, filesystem fs.FS) Route {
	return a.Get(path, func(c *Context) error {
		return c.FileFS(file, filesystem)
	})
}

// newStaticHandler limits methods before resolving paths so unsupported
// requests never touch the filesystem. Misses and failures are returned to the
// application's error handler, like any other route.
func newStaticHandler(filesystem fs.FS, cfg StaticConfig) func(*Context, string) error {
	return func(c *Context, requestPath string) error {
		r := c.Request()
		if r == marks.Probe {
			// Asked by staticDirectory, outside any request's chain.
			if staticDirectoryServed(filesystem, requestPath, cfg) {
				return errStaticDirectory
			}
			return nil
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			c.SetHeader(HeaderAllow, "GET, HEAD")
			return ErrMethodNotAllowed
		}

		name, err := staticPathName(requestPath)
		if err != nil {
			return ErrNotFound
		}

		if err := serveStaticPath(c, requestPath, filesystem, name, cfg); err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) || errors.Is(err, fs.ErrPermission) {
				return ErrNotFound.Wrap(err)
			}
			return err
		}
		return nil
	}
}

// errStaticDirectory is a static handler's answer to a probe for a path it
// serves as a directory.
var errStaticDirectory = errors.New("zinc: static directory")

func init() { marks.StaticDirectory = staticDirectory }

// staticDirectory reports whether a static mount of c's app serves requestPath as a
// directory, whose URL must end with a slash. trailingslash asks, so it
// doesn't strip that slash and loop with the directory's redirect.
func staticDirectory(v any, requestPath string) bool {
	c, ok := v.(*Context)
	if !ok || c == nil || c.app == nil {
		return false
	}
	mount := c.app.matchMount(requestPath)
	if mount == nil || mount.native == nil {
		return false
	}
	return mount.native(&Context{request: marks.Probe}, requestPath[mount.prefixCut(requestPath):]) == errStaticDirectory
}

// confinedDirFS retains one OS root per static mount after its first open.
// Root.Open keeps every request confined, including symlink traversal.
type confinedDirFS struct {
	path   string
	mu     sync.RWMutex
	root   *os.Root
	closed bool
}

func (filesystem *confinedDirFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	filesystem.mu.RLock()
	if filesystem.closed {
		filesystem.mu.RUnlock()
		return nil, os.ErrClosed
	}
	if root := filesystem.root; root != nil {
		file, err := root.Open(name)
		filesystem.mu.RUnlock()
		return file, err
	}
	filesystem.mu.RUnlock()

	filesystem.mu.Lock()
	defer filesystem.mu.Unlock()
	if filesystem.closed {
		return nil, os.ErrClosed
	}
	if filesystem.root == nil {
		root, err := os.OpenRoot(filesystem.path)
		if err != nil {
			return nil, err
		}
		filesystem.root = root
	}
	return filesystem.root.Open(name)
}

func (filesystem *confinedDirFS) Close() error {
	filesystem.mu.Lock()
	defer filesystem.mu.Unlock()
	filesystem.closed = true
	if filesystem.root == nil {
		return nil
	}
	err := filesystem.root.Close()
	filesystem.root = nil
	return err
}

// staticPathName consumes URL.Path, which net/http has already decoded. Never
// decode it a second time or normalize it differently from authorization.
func staticPathName(requestPath string) (string, error) {
	if strings.ContainsAny(requestPath, "\\\x00") {
		return "", fs.ErrInvalid
	}
	name := strings.TrimPrefix(requestPath, "/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		return ".", nil
	}
	if !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}
	return name, nil
}

// serveStaticPath serves a directory only at its URL with a trailing slash,
// as http.FileServer does, so relative links in its index resolve inside it.
// The URL without the slash redirects there. A directory with nothing to
// serve is a 404 either way.
func serveStaticPath(c *Context, requestPath string, filesystem fs.FS, name string, cfg StaticConfig) error {
	w, r := c.Writer(), c.Request()
	file, stat, err := openStaticFile(filesystem, name)
	if err != nil {
		return err
	}
	defer file.Close()

	if !stat.IsDir() {
		return serveOpenedStaticFile(w, r, file, stat)
	}

	if !strings.HasSuffix(requestPath, "/") {
		if !staticDirectoryHasContent(filesystem, name, cfg) {
			return fs.ErrNotExist
		}
		return c.Status(http.StatusMovedPermanently).Redirect(staticDirectoryURL(r.URL))
	}

	if indexName := staticIndexName(name, cfg); indexName != "" {
		if err := serveStaticNamedFile(w, r, filesystem, indexName); err == nil {
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	if !cfg.Browse {
		return fs.ErrNotExist
	}

	return serveStaticDirectoryListing(w, r, filesystem, name)
}

func staticIndexName(dir string, cfg StaticConfig) string {
	if cfg.Index == "" || dir == "." {
		return cfg.Index
	}
	return path.Join(dir, cfg.Index)
}

// staticDirectoryServed reports whether requestPath names a directory the
// handler serves.
func staticDirectoryServed(filesystem fs.FS, requestPath string, cfg StaticConfig) bool {
	name, err := staticPathName(requestPath)
	if err != nil {
		return false
	}
	stat, err := fs.Stat(filesystem, name)
	return err == nil && stat.IsDir() && staticDirectoryHasContent(filesystem, name, cfg)
}

// staticDirectoryHasContent reports whether directory name has an index
// file, or is listed.
func staticDirectoryHasContent(filesystem fs.FS, name string, cfg StaticConfig) bool {
	if cfg.Browse {
		return true
	}
	indexName := staticIndexName(name, cfg)
	if indexName == "" {
		return false
	}
	stat, err := fs.Stat(filesystem, indexName)
	return err == nil && !stat.IsDir()
}

// staticDirectoryURL is the request URL with a trailing slash. It's built
// from the escaped path, so encoded bytes stay encoded, and keeps the query.
// Leading slashes and backslashes collapse to one slash, so it can't start
// with "//" or "/\" and leave this site.
func staticDirectoryURL(u *url.URL) string {
	target := "/" + strings.TrimLeft(u.EscapedPath(), `/\`)
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return target
}

func serveStaticNamedFile(w http.ResponseWriter, r *http.Request, filesystem fs.FS, name string) error {
	file, stat, err := openStaticFile(filesystem, name)
	if err != nil {
		return err
	}
	defer file.Close()
	if stat.IsDir() {
		return fs.ErrInvalid
	}
	return serveOpenedStaticFile(w, r, file, stat)
}

func openStaticFile(filesystem fs.FS, name string) (fs.File, fs.FileInfo, error) {
	file, err := filesystem.Open(name)
	if err != nil {
		return nil, nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, stat, nil
}

// serveOpenedStaticFile preserves range and conditional request support through
// http.ServeContent. Non-seekable files are buffered as a compatibility fallback.
func serveOpenedStaticFile(w http.ResponseWriter, r *http.Request, file fs.File, stat fs.FileInfo) error {
	if rs, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), rs)
		return nil
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	reader := strings.NewReader(string(data))
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), reader)
	return nil
}

// serveStaticDirectoryListing escapes display names and never emits filesystem
// paths outside the requested directory.
func serveStaticDirectoryListing(w http.ResponseWriter, r *http.Request, filesystem fs.FS, name string) error {
	entries, err := fs.ReadDir(filesystem, name)
	if err != nil {
		return err
	}

	header := w.Header()
	if header.Get(contentType) == "" {
		header.Set(contentType, htmlType)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return nil
	}

	var body strings.Builder
	body.WriteString("<!doctype html><html><body><ul>")
	for _, entry := range entries {
		display := entry.Name()
		if entry.IsDir() {
			display += "/"
		}
		body.WriteString("<li>")
		body.WriteString(html.EscapeString(display))
		body.WriteString("</li>")
	}
	body.WriteString("</ul></body></html>")
	_, err = io.WriteString(w, body.String())
	return err
}
