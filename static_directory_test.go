// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/trailingslash"
)

// A static directory is served at its URL with a trailing slash, as
// http.FileServer serves it, so relative links in its index resolve inside
// the directory. The URL without the slash redirects there.

func staticSite() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                &fstest.MapFile{Data: []byte(`root`)},
		"docs/index.html":           &fstest.MapFile{Data: []byte(`<link rel="stylesheet" href="style.css">`)},
		"docs/style.css":            &fstest.MapFile{Data: []byte(`body{}`)},
		"style.css":                 &fstest.MapFile{Data: []byte(`wrong`)},
		"empty/notes.txt":           &fstest.MapFile{Data: []byte(`notes`)},
		"evil.example/index.html":   &fstest.MapFile{Data: []byte(`evil`)},
		"docs/sub dir/index.html":   &fstest.MapFile{Data: []byte(`sub`)},
		"docs/sub dir/nested/a.txt": &fstest.MapFile{Data: []byte(`a`)},
	}
}

func strictModes(t *testing.T, f func(t *testing.T, strict bool)) {
	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprintf("strict=%v", strict), func(t *testing.T) { f(t, strict) })
	}
}

func TestStaticDirectoryRedirectsToTrailingSlash(t *testing.T) {
	strictModes(t, func(t *testing.T, strict bool) {
		app := zinc.New(zinc.Config{StrictRouting: strict})
		app.StaticFS("/assets", staticSite())

		cases := []struct{ method, target, want string }{
			{"GET", "/assets/docs", "/assets/docs/"},
			{"GET", "/assets/docs?v=1&x=%2F", "/assets/docs/?v=1&x=%2F"},
			{"HEAD", "/assets/docs", "/assets/docs/"},
			{"GET", "/assets", "/assets/"},
			{"GET", "/assets/docs/sub%20dir?q", "/assets/docs/sub%20dir/?q"},
		}
		for _, tc := range cases {
			w := serve(app, tc.method, tc.target, "")
			if w.Code != http.StatusMovedPermanently || w.Header().Get(zinc.HeaderLocation) != tc.want {
				t.Errorf("%s %s: %d Location %q, want 301 %q", tc.method, tc.target, w.Code, w.Header().Get(zinc.HeaderLocation), tc.want)
			}
		}

		if w := serve(app, "GET", "/assets/docs/", ""); w.Code != http.StatusOK || w.Body.String() != `<link rel="stylesheet" href="style.css">` {
			t.Fatalf("index: %d %q", w.Code, w.Body)
		}
		if w := serve(app, "GET", "/assets/", ""); w.Code != http.StatusOK || w.Body.String() != "root" {
			t.Fatalf("mount root: %d %q", w.Code, w.Body)
		}
		// Files are unaffected.
		if w := serve(app, "GET", "/assets/docs/style.css", ""); w.Code != http.StatusOK || w.Body.String() != "body{}" {
			t.Fatalf("file: %d %q", w.Code, w.Body)
		}
		// A directory with nothing to serve is a 404, with or without the slash.
		for _, target := range []string{"/assets/empty", "/assets/empty/", "/assets/docs/sub%20dir/nested"} {
			if w := serve(app, "GET", target, ""); w.Code != http.StatusNotFound || w.Header().Get(zinc.HeaderLocation) != "" {
				t.Errorf("%s: %d Location %q", target, w.Code, w.Header().Get(zinc.HeaderLocation))
			}
		}
	})
}

// The relative stylesheet in the index resolves inside the directory once a
// browser follows the redirect.
func TestStaticDirectoryRelativeLinksResolve(t *testing.T) {
	app := zinc.New()
	app.StaticFS("/assets", staticSite())

	final, w, _ := follow(t, app, "/assets/docs")
	href := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(w.Body.String())
	if href == nil {
		t.Fatalf("no link in %q", w.Body)
	}
	base, _ := url.Parse("https://app.example" + final)
	ref, _ := url.Parse(href[1])
	asset := base.ResolveReference(ref)
	if asset.Path != "/assets/docs/style.css" {
		t.Fatalf("style.css resolves to %s", asset.Path)
	}
	if w := serve(app, "GET", asset.RequestURI(), ""); w.Code != http.StatusOK || w.Body.String() != "body{}" {
		t.Fatalf("asset: %d %q", w.Code, w.Body)
	}
}

func TestStaticDirectoryRedirectStaysOnSite(t *testing.T) {
	app := zinc.New()
	app.StaticFS("/", staticSite())
	for _, target := range []string{
		"/evil.example",
		"//evil.example",
		"///evil.example",
		"/%2Fevil.example",
		"/%2fevil.example",
		"/%5Cevil.example",
		"/%5C%5Cevil.example",
		"/\\evil.example",
		"/./evil.example",
		"/docs/../evil.example",
	} {
		w := serve(app, "GET", target, "")
		location := w.Header().Get(zinc.HeaderLocation)
		if location == "" {
			continue
		}
		if len(location) > 1 && (location[1] == '/' || location[1] == '\\') {
			t.Errorf("%s: Location %q leaves the site", target, location)
		}
		base, _ := url.Parse("https://app.example/here/")
		ref, err := url.Parse(location)
		if err != nil || base.ResolveReference(ref).Host != "app.example" {
			t.Errorf("%s: Location %q resolves off site", target, location)
		}
	}
	if w := serve(app, "GET", "/evil.example", ""); w.Header().Get(zinc.HeaderLocation) != "/evil.example/" {
		t.Fatalf("plain directory: %d %q", w.Code, w.Header().Get(zinc.HeaderLocation))
	}
}

// follow requests target and follows up to five redirects, as a browser
// would, failing on a sixth.
func follow(t *testing.T, app *zinc.App, target string) (string, *httptest.ResponseRecorder, int) {
	t.Helper()
	for hops := 0; hops <= 5; hops++ {
		w := serve(app, "GET", target, "")
		if w.Code < 300 || w.Code > 399 {
			return target, w, hops
		}
		next, err := url.Parse(w.Header().Get(zinc.HeaderLocation))
		if err != nil {
			t.Fatalf("%s: bad Location: %v", target, err)
		}
		base, _ := url.Parse("http://app.example" + target)
		resolved := base.ResolveReference(next)
		if resolved.Host != "app.example" {
			t.Fatalf("%s: redirected off site to %s", target, resolved)
		}
		target = resolved.RequestURI()
	}
	t.Fatalf("redirect loop, still redirecting at %s", target)
	return "", nil, 0
}

// trailingslash leaves a static directory's slash alone, in every mode, so
// it never loops with the directory's redirect.
func TestStaticDirectoryWithTrailingSlashDoesNotLoop(t *testing.T) {
	configs := map[string]trailingslash.Config{
		"remove":          {},
		"remove redirect": {Redirect: true},
		"add":             {Add: true},
		"add redirect":    {Add: true, Redirect: true},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			strictModes(t, func(t *testing.T, strict bool) {
				app := zinc.New(zinc.Config{StrictRouting: strict})
				app.Use(trailingslash.New(cfg))
				app.StaticFS("/assets", staticSite())
				starts := []string{"/assets/docs", "/assets/docs/", "/assets", "/assets/"}
				if !cfg.Add {
					starts = append(starts, "/assets/docs//")
				}
				for _, start := range starts {
					final, w, hops := follow(t, app, start)
					if w.Code != http.StatusOK {
						t.Errorf("%s → %s: %d", start, final, w.Code)
					}
					if hops > 2 {
						t.Errorf("%s: %d redirects", start, hops)
					}
				}
				// Files still get the configured treatment.
				final, w, _ := follow(t, app, "/assets/docs/style.css")
				if !cfg.Add && (w.Code != http.StatusOK || w.Body.String() != "body{}") {
					t.Errorf("file → %s: %d %q", final, w.Code, w.Body)
				}
				if !cfg.Add {
					final, w, _ = follow(t, app, "/assets/docs/style.css/")
					if w.Code != http.StatusOK || w.Body.String() != "body{}" {
						t.Errorf("file with slash → %s: %d %q", final, w.Code, w.Body)
					}
				}
			})
		})
	}
}
