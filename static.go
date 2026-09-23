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

// Static serves root from the operating-system filesystem below prefix.
func (a *App) Static(prefix, root string, opts ...StaticOption) error {
	return a.StaticFS(prefix, os.DirFS(root), opts...)
}

// StaticFS serves filesystem below prefix.
func (a *App) StaticFS(prefix string, filesystem fs.FS, opts ...StaticOption) error {
	if filesystem == nil {
		return errors.New("filesystem is nil")
	}
	cfg := StaticConfig{Index: "index.html"}
	for _, opt := range opts {
		opt(&cfg)
	}
	a.Mount(prefix, newStaticHandler(filesystem, cfg))
	return nil
}

// File serves one operating-system file at path.
func (a *App) File(path, file string) error {
	a.Get(path, func(c *Context) error {
		return c.File(file)
	})
	return nil
}

// FileFS serves one file from filesystem at path.
func (a *App) FileFS(path, file string, filesystem fs.FS) error {
	a.Get(path, func(c *Context) error {
		return c.FileFS(file, filesystem)
	})
	return nil
}

// newStaticHandler limits methods before resolving paths so unsupported
// requests never touch the filesystem.
func newStaticHandler(filesystem fs.FS, cfg StaticConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set(HeaderAllow, "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name, err := staticPathName(r.URL.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if err := serveStaticPath(w, r, filesystem, name, cfg); err != nil {
			switch {
			case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrInvalid), errors.Is(err, fs.ErrPermission):
				http.NotFound(w, r)
			default:
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}
	})
}

// staticPathName converts a URL path to a valid fs.FS name. Backslashes and
// explicit parent segments are rejected before path cleaning to prevent
// platform-dependent traversal.
func staticPathName(requestPath string) (string, error) {
	if requestPath == "" {
		return ".", nil
	}

	unescaped, err := url.PathUnescape(requestPath)
	if err != nil {
		return "", fs.ErrInvalid
	}

	if strings.IndexByte(unescaped, '\x00') >= 0 {
		return "", fs.ErrInvalid
	}

	unescaped = strings.ReplaceAll(unescaped, "\\", "/")
	for _, segment := range strings.Split(unescaped, "/") {
		if segment == ".." {
			return "", fs.ErrInvalid
		}
	}

	cleaned := path.Clean("/" + unescaped)
	name := strings.TrimPrefix(cleaned, "/")
	if name == "" || name == "." {
		return ".", nil
	}
	if !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}
	return name, nil
}

func serveStaticPath(w http.ResponseWriter, r *http.Request, filesystem fs.FS, name string, cfg StaticConfig) error {
	file, stat, err := openStaticFile(filesystem, name)
	if err != nil {
		return err
	}
	defer file.Close()

	if !stat.IsDir() {
		return serveOpenedStaticFile(w, r, file, stat)
	}

	if cfg.Index != "" {
		indexName := cfg.Index
		if name != "." {
			indexName = path.Join(name, cfg.Index)
		}
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
