// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package docspage renders the API reference page for an OpenAPI spec. Zinc
// uses it for the docs page every app serves, and middleware/apidocs for the
// page it serves, so the two can't drift apart.
package docspage

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// UI is a docs renderer.
type UI string

// The renderers. Each loads a pinned version from
// cdn.jsdelivr.net, with a Subresource Integrity hash.
const (
	Scalar    UI = "scalar"     // @scalar/api-reference 1.72.1, the default
	SwaggerUI UI = "swagger-ui" // swagger-ui-dist 5.33.0
	Stoplight UI = "stoplight"  // @stoplight/elements 9.0.25
	Redoc     UI = "redoc"      // redoc 2.5.4
)

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
{{template "assets" .}}<script>Scalar.createApiReference('#app', { url: document.getElementById('app').dataset.spec, baseServerURL: location.origin, agent: { disabled: true }, mcp: { disabled: true }, showDeveloperTools: 'never' })</script>`,
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

// Files lists the files a renderer loads. It returns nil for an unknown UI.
func Files(ui UI) []Asset {
	r, ok := renderers[ui]
	if !ok {
		return nil
	}
	return append([]Asset(nil), r.assets...)
}

// Render returns the page for ui, loading the spec from spec. A non-empty
// assetsURL serves the renderer's files from there instead of the CDN.
func Render(ui UI, spec, title, assetsURL string) ([]byte, error) {
	r, ok := renderers[ui]
	if !ok {
		return nil, fmt.Errorf("unknown UI %q", ui)
	}
	assets := Files(ui)
	if base := strings.TrimSuffix(assetsURL, "/"); assetsURL != "" {
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
	}{title, spec, assets}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
