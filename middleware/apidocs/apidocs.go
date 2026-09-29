// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package apidocs serves a browsable reference page for an OpenAPI spec,
// such as the one every Zinc app serves at /openapi.json.
package apidocs

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// UI is a docs renderer.
type UI string

// The renderers apidocs can serve. Each loads a pinned version from
// cdn.jsdelivr.net, with a Subresource Integrity hash.
const (
	Scalar    UI = "scalar"     // @scalar/api-reference 1.72.1, the default
	SwaggerUI UI = "swagger-ui" // swagger-ui-dist 5.33.0
	Stoplight UI = "stoplight"  // @stoplight/elements 9.0.25
	Redoc     UI = "redoc"      // redoc 2.5.4
)

// DefaultSpec is the spec URL used when Config.Spec is empty. It's where a
// Zinc app serves its spec unless zinc.Config.OpenAPIPath says otherwise.
const DefaultSpec = "/openapi.json"

// Config controls the docs page.
type Config struct {
	// Spec is the URL of the OpenAPI spec the page loads. Empty means
	// DefaultSpec.
	Spec string
	// UI picks the renderer. Empty means Scalar.
	UI UI
	// Title is the page title. Empty means "API reference".
	Title string
	// AssetsURL serves the renderer's files from your own server instead of
	// the CDN, such as "/assets/apidocs", for offline use or a strict
	// Content-Security-Policy. The page loads each file by its name below
	// this URL; Files lists the names. The files must be the pinned
	// versions: the integrity hashes still apply.
	AssetsURL string
}

// Asset is one file a renderer loads.
type Asset struct {
	// Name is the file name the page loads below Config.AssetsURL.
	Name string
	// URL is the pinned CDN URL the file comes from by default.
	URL string
	// Integrity is the file's Subresource Integrity hash.
	Integrity string
	// Stylesheet is true for CSS, false for JavaScript.
	Stylesheet bool
}

type renderer struct {
	assets []Asset
	// body is the page body. Its script is constant, so a CSP can allow it
	// by hash; it reads the spec URL from the data-spec attribute.
	body template.HTML
}

const cdn = "https://cdn.jsdelivr.net/npm/"

var renderers = map[UI]renderer{
	Scalar: {
		assets: []Asset{{
			Name:      "standalone.js",
			URL:       cdn + "@scalar/api-reference@1.72.1/dist/browser/standalone.js",
			Integrity: "sha384-U11tb2XnKvmwt8RlTvnwUnYgrN+ur4Xyh9htLhjajWNR/Oyl5AX5DEz00qRmlrmK",
		}},
		body: `<div id="app" data-spec="{{.Spec}}"></div>
{{template "assets" .}}<script>Scalar.createApiReference('#app', { url: document.getElementById('app').dataset.spec })</script>`,
	},
	SwaggerUI: {
		assets: []Asset{
			{
				Name:       "swagger-ui.css",
				URL:        cdn + "swagger-ui-dist@5.33.0/swagger-ui.css",
				Integrity:  "sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW",
				Stylesheet: true,
			},
			{
				Name:      "swagger-ui-bundle.js",
				URL:       cdn + "swagger-ui-dist@5.33.0/swagger-ui-bundle.js",
				Integrity: "sha384-YDALVcy8kj8yltLBVi1vBiBAUqdxvus673gM8XKwiy6aDUJFXivF/KCufekjYbVf",
			},
		},
		body: `<div id="swagger-ui" data-spec="{{.Spec}}"></div>
{{template "assets" .}}<script>SwaggerUIBundle({ url: document.getElementById('swagger-ui').dataset.spec, dom_id: '#swagger-ui' })</script>`,
	},
	Stoplight: {
		assets: []Asset{
			{
				Name:       "styles.min.css",
				URL:        cdn + "@stoplight/elements@9.0.25/styles.min.css",
				Integrity:  "sha384-NzdOiocfnINlXfuCXi4OpL/xvdbgLiKaLHQ07Z+IwhVaxHqLShn5rVD5OHt/LYgz",
				Stylesheet: true,
			},
			{
				Name:      "web-components.min.js",
				URL:       cdn + "@stoplight/elements@9.0.25/web-components.min.js",
				Integrity: "sha384-X5kH2B8aH81JEl8IfSBwwnr8FYcCqMzdxpqjmmlRbhIl7SsQ9Zn0xk+csQmU37zN",
			},
		},
		body: `{{template "assets" .}}<elements-api apiDescriptionUrl="{{.Spec}}" router="hash" layout="sidebar"></elements-api>`,
	},
	Redoc: {
		assets: []Asset{{
			Name:      "redoc.standalone.js",
			URL:       cdn + "redoc@2.5.4/bundles/redoc.standalone.js",
			Integrity: "sha384-w447zOpYfw/1Tv/5AK9NfHTlQIqE3RVR6KY62jCyy9zNDgO64cMwGGP1Fj0zJVf5",
		}},
		body: `<redoc spec-url="{{.Spec}}"></redoc>
{{template "assets" .}}`,
	},
}

const page = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
{{range .Assets}}{{if .Stylesheet}}<link rel="stylesheet" href="{{.URL}}" integrity="{{.Integrity}}" crossorigin="anonymous">
{{end}}{{end}}</head>
<body>
{{template "body" .}}
</body>
</html>
{{define "assets"}}{{range .Assets}}{{if not .Stylesheet}}<script src="{{.URL}}" integrity="{{.Integrity}}" crossorigin="anonymous"></script>
{{end}}{{end}}{{end}}`

// Files lists the files a renderer loads, for serving them yourself with
// Config.AssetsURL. It returns nil for an unknown UI.
func Files(ui UI) []Asset {
	r, ok := renderers[ui]
	if !ok {
		return nil
	}
	return append([]Asset(nil), r.assets...)
}

// New renders the docs page once and serves it. Register it on a GET route
// and hide that route from the spec:
//
//	app := zinc.New(zinc.Config{OpenAPI: zinc.OpenAPIConfig{Title: "Shop"}})
//	app.Get("/docs", apidocs.New()).Hidden()
//
// It panics on an unknown UI.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("apidocs", configs)
	if config.UI == "" {
		config.UI = Scalar
	}
	r, ok := renderers[config.UI]
	if !ok {
		panic(fmt.Sprintf("apidocs: unknown UI %q; use apidocs.Scalar, SwaggerUI, Stoplight or Redoc", config.UI))
	}
	if config.Spec == "" {
		config.Spec = DefaultSpec
	}
	if config.Title == "" {
		config.Title = "API reference"
	}
	assets := Files(config.UI)
	if base := strings.TrimSuffix(config.AssetsURL, "/"); config.AssetsURL != "" {
		for i := range assets {
			assets[i].URL = base + "/" + assets[i].Name
		}
	}

	t := template.Must(template.New("page").Parse(page))
	template.Must(t.New("body").Parse(string(r.body)))
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		Title  string
		Spec   string
		Assets []Asset
	}{config.Title, config.Spec, assets}); err != nil {
		panic("apidocs: " + err.Error())
	}
	html := buf.Bytes()

	return func(c *zinc.Context) error {
		return c.Data("text/html; charset=utf-8", html)
	}
}
