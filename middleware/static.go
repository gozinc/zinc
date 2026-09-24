// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/0mjs/zinc"
)

// StaticConfig controls filesystem serving and fallthrough behavior.
type StaticConfig struct {
	Skipper        func(*zinc.Context) bool
	Filesystem     fs.FS
	Root           string
	Prefix         string
	NextOnNotFound bool
}

// Static serves root and falls through when a file is absent.
func Static(root string) zinc.Middleware {
	return StaticWithConfig(StaticConfig{
		Root:           root,
		NextOnNotFound: true,
	})
}

// StaticFrom serves root below a URL prefix.
func StaticFrom(prefix, root string) zinc.Middleware {
	return StaticWithConfig(StaticConfig{
		Root:           root,
		Prefix:         prefix,
		NextOnNotFound: true,
	})
}

// StaticFS serves an fs.FS and falls through when a file is absent.
func StaticFS(filesystem fs.FS) zinc.Middleware {
	return StaticWithConfig(StaticConfig{
		Filesystem:     filesystem,
		NextOnNotFound: true,
	})
}

// StaticWithConfig serves files through net/http.FileServer. Paths are converted
// to fs.ValidPath form and explicit parent traversal is rejected first.
func StaticWithConfig(config StaticConfig) zinc.Middleware {
	cfg := resolveStaticConfig(config)
	handler := http.FileServer(http.FS(cfg.Filesystem))

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		req := c.Request()
		if req == nil || req.URL == nil {
			return c.Next()
		}

		name, ok := staticRequestName(req.URL.Path, cfg.Prefix)
		if !ok {
			return c.Next()
		}
		if cfg.NextOnNotFound && !staticFileExists(cfg.Filesystem, name) {
			return c.Next()
		}

		cloned := req.Clone(req.Context())
		cloned.URL = cloneMiddlewareURL(req.URL)
		cloned.URL.Path = "/" + name
		cloned.URL.RawPath = cloned.URL.Path
		cloned.RequestURI = cloned.URL.RequestURI()

		handler.ServeHTTP(c.Writer(), cloned)
		return nil
	}
}

func resolveStaticConfig(config StaticConfig) StaticConfig {
	if config.Filesystem == nil {
		if config.Root == "" {
			panic("zincstatic: Root or Filesystem is required")
		}
		config.Filesystem = staticRootFS(config.Root)
	}
	config.Prefix = normalizeStaticPrefix(config.Prefix)
	return config
}

func staticRequestName(requestPath, prefix string) (string, bool) {
	if prefix != "" {
		if requestPath != prefix && !strings.HasPrefix(requestPath, prefix+"/") {
			return "", false
		}
		requestPath = strings.TrimPrefix(requestPath, prefix)
	}
	if strings.ContainsAny(requestPath, "\\\x00") {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(requestPath, "/"), "/")
	if name == "" {
		name = "."
	}
	if !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

type staticRootFS string

func (root staticRootFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return os.OpenInRoot(string(root), name)
}

func staticFileExists(filesystem fs.FS, name string) bool {
	file, err := filesystem.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	return true
}

func normalizeStaticPrefix(prefix string) string {
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

func cloneMiddlewareURL(u *url.URL) *url.URL {
	clone := *u
	return &clone
}
