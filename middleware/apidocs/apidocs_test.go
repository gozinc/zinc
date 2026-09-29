// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package apidocs

import (
	"html"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func render(t *testing.T, method string, cfg ...Config) (int, string, string) {
	t.Helper()
	app := zinc.New()
	app.Get("/docs", New(cfg...)).Hidden()
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest(method, "/docs", nil))
	return w.Code, w.Header().Get("Content-Type"), w.Body.String()
}

func TestDefaultsToScalar(t *testing.T) {
	code, ct, body := render(t, "GET")
	if code != 200 || ct != "text/html; charset=utf-8" {
		t.Fatalf("%d %q", code, ct)
	}
	for _, want := range []string{
		"<title>API reference</title>",
		`data-spec="/openapi.json"`,
		`src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.72.1/dist/browser/standalone.js"`,
		"Scalar.createApiReference",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s:\n%s", want, body)
		}
	}
	if code, _, body := render(t, "HEAD"); code != 200 || body != "" {
		t.Fatalf("HEAD: %d, %q", code, body)
	}
}

// Every renderer loads each of its files with its integrity hash, and
// nothing else from outside the page.
func TestEveryUILoadsItsPinnedFiles(t *testing.T) {
	for _, ui := range []UI{Scalar, SwaggerUI, Stoplight, Redoc} {
		_, _, body := render(t, "GET", Config{UI: ui})
		files := Files(ui)
		if len(files) == 0 {
			t.Fatalf("%s: no files", ui)
		}
		for _, f := range files {
			if !strings.HasPrefix(f.URL, "https://cdn.jsdelivr.net/npm/") || !strings.Contains(f.URL, "@") {
				t.Errorf("%s: %s isn't a pinned jsdelivr URL", ui, f.URL)
			}
			if !strings.HasPrefix(f.Integrity, "sha384-") {
				t.Errorf("%s: %s has no sha384 hash", ui, f.Name)
			}
			tag := `src="` + f.URL + `" integrity="` + f.Integrity + `" crossorigin="anonymous"`
			if f.Stylesheet {
				tag = `href="` + f.URL + `" integrity="` + f.Integrity + `" crossorigin="anonymous"`
			}
			if !strings.Contains(html.UnescapeString(body), tag) {
				t.Errorf("%s: page lacks %s", ui, tag)
			}
		}
		if n := strings.Count(body, "https://"); n != len(files) {
			t.Errorf("%s: page loads %d external URLs, want %d", ui, n, len(files))
		}
	}
}

func TestAssetsURLServesFilesYourself(t *testing.T) {
	_, _, body := render(t, "GET", Config{UI: SwaggerUI, AssetsURL: "/static/docs/"})
	body = html.UnescapeString(body)
	for _, f := range Files(SwaggerUI) {
		if !strings.Contains(body, `"/static/docs/`+f.Name+`" integrity="`+f.Integrity+`"`) {
			t.Errorf("page doesn't load %s from AssetsURL:\n%s", f.Name, body)
		}
	}
	if strings.Contains(body, "cdn.jsdelivr.net") {
		t.Fatal("page still loads from the CDN")
	}
}

// The spec URL and title are escaped, and the inline script never changes,
// so a Content-Security-Policy can allow it by hash.
func TestEscapingAndConstantScripts(t *testing.T) {
	_, _, body := render(t, "GET", Config{Spec: `/spec?a=1&b="x"`, Title: `<Shop> & co`})
	if !strings.Contains(body, `<title>&lt;Shop&gt; &amp; co</title>`) || !strings.Contains(body, `data-spec="/spec?a=1&amp;b=&#34;x&#34;"`) {
		t.Fatalf("not escaped:\n%s", body)
	}
	for _, ui := range []UI{Scalar, SwaggerUI} {
		_, _, a := render(t, "GET", Config{UI: ui, Spec: "/a.json"})
		_, _, b := render(t, "GET", Config{UI: ui, Spec: "/b.json"})
		if inlineScript(a) == "" || inlineScript(a) != inlineScript(b) {
			t.Fatalf("%s: inline script depends on the spec URL:\n%s\n%s", ui, inlineScript(a), inlineScript(b))
		}
	}
}

func inlineScript(body string) string {
	i := strings.Index(body, "<script>")
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i:], "</script>")
	return body[i : i+j]
}

func TestMisuse(t *testing.T) {
	defer func() {
		if got, _ := recover().(string); !strings.Contains(got, `unknown UI "rapidoc"`) {
			t.Fatalf("panic %q", got)
		}
	}()
	if Files("rapidoc") != nil {
		t.Fatal("Files for an unknown UI")
	}
	// Files returns a copy: changing it doesn't change the page.
	files := Files(Scalar)
	files[0].URL = "https://evil.example/x.js"
	if _, _, body := render(t, "GET"); strings.Contains(body, "evil.example") {
		t.Fatal("Files shares its slice with the renderer")
	}
	New(Config{UI: "rapidoc"})
}
