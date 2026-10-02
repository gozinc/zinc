// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package apidocs serves a browsable reference page for an OpenAPI spec,
// such as the one every Zinc app serves at /openapi.json. Every app already
// serves this page at /docs; use apidocs to serve it elsewhere, behind
// middleware, with another renderer, or from your own copy of its files.
package apidocs

import (
	"fmt"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/internal/docspage"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// UI is a docs renderer.
type UI = docspage.UI

// The renderers apidocs can serve. Each loads a pinned version from
// cdn.jsdelivr.net, with a Subresource Integrity hash.
const (
	Scalar    = docspage.Scalar    // @scalar/api-reference 1.72.1, the default
	SwaggerUI = docspage.SwaggerUI // swagger-ui-dist 5.33.0
	Stoplight = docspage.Stoplight // @stoplight/elements 9.0.25
	Redoc     = docspage.Redoc     // redoc 2.5.4
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
type Asset = docspage.Asset

// Files lists the files a renderer loads, for serving them yourself with
// Config.AssetsURL. It returns nil for an unknown UI.
func Files(ui UI) []Asset { return docspage.Files(ui) }

// New renders the docs page once and serves it. Register it on a GET route
// and hide that route from the spec:
//
//	app := zinc.New(zinc.Config{DocsPath: "-"}) // turn off the built-in page
//	app.Get("/reference", apidocs.New(apidocs.Config{UI: apidocs.Redoc})).Hidden()
//
// It panics on an unknown UI.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("apidocs", configs)
	if config.UI == "" {
		config.UI = Scalar
	}
	if config.Spec == "" {
		config.Spec = DefaultSpec
	}
	if config.Title == "" {
		config.Title = "API reference"
	}
	html, err := docspage.Render(config.UI, config.Spec, config.Title, config.AssetsURL)
	if err != nil {
		panic(fmt.Sprintf("apidocs: unknown UI %q; use apidocs.Scalar, SwaggerUI, Stoplight or Redoc", config.UI))
	}
	return func(c *zinc.Context) error {
		return c.Data("text/html; charset=utf-8", html)
	}
}
