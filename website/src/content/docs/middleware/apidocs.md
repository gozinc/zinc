---
title: API Docs
description: Serve a browsable reference page for your API's OpenAPI spec, with Scalar, Swagger UI, Stoplight Elements or ReDoc.
---

API Docs serves a reference page for your API that people can read and try requests from. It renders the [OpenAPI](/guide/openapi/) spec every Zinc app serves at `/openapi.json`, so the page is always in step with your routes.

## Usage

```go
import "github.com/0mjs/zinc/middleware/apidocs"

app := zinc.New(zinc.Config{
	OpenAPI: zinc.OpenAPIConfig{Title: "Pet Store", Version: "1.0.0"},
})
app.Get("/docs", apidocs.New()).Hidden()
```

Open `http://localhost:8080/docs` in a browser. The page loads [Scalar](https://github.com/scalar/scalar) from a CDN, which fetches `/openapi.json` and renders every route, its parameters, bodies and responses.

```bash
curl http://localhost:8080/docs
# <!doctype html>
# ...
# <title>API reference</title>
# </head>
# <body>
# <div id="app" data-spec="/openapi.json"></div>
# <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.72.1/dist/browser/standalone.js" integrity="sha384-U11tb2XnKvmwt8RlTvnwUnYgrN&#43;ur4Xyh9htLhjajWNR/Oyl5AX5DEz00qRmlrmK" crossorigin="anonymous"></script>
# <script>Scalar.createApiReference('#app', { url: document.getElementById('app').dataset.spec })</script>
```

`.Hidden()` keeps the docs route out of the spec it displays. The page is rendered once, when you call `New`, and every request sends the same bytes.

## Defaults

| Setting | Default |
|---|---|
| Spec | `/openapi.json` (`apidocs.DefaultSpec`) |
| UI | `apidocs.Scalar` |
| Title | `API reference` |
| Assets | pinned versions from `cdn.jsdelivr.net`, each with an integrity hash |

## Configuration

Pick another renderer with `UI`:

```go
app.Get("/docs", apidocs.New(apidocs.Config{
	UI:    apidocs.SwaggerUI,
	Spec:  "/api/openapi.json",
	Title: "Pet Store API",
})).Hidden()
```

| Field | Default | Meaning |
|---|---|---|
| `Spec` | `/openapi.json` | URL of the spec the page loads. It can be relative to the page or on another host. |
| `UI` | `apidocs.Scalar` | The renderer: `Scalar`, `SwaggerUI`, `Stoplight` or `Redoc`. |
| `Title` | `API reference` | The page's `<title>`. |
| `AssetsURL` | none | Load the renderer's files from this URL on your own server instead of the CDN. See [Serve the files yourself](#serve-the-files-yourself). |

| UI | Version | Files |
|---|---|---|
| `apidocs.Scalar` | `@scalar/api-reference` 1.72.1 | `standalone.js` |
| `apidocs.SwaggerUI` | `swagger-ui-dist` 5.33.0 | `swagger-ui.css`, `swagger-ui-bundle.js` |
| `apidocs.Stoplight` | `@stoplight/elements` 9.0.25 | `styles.min.css`, `web-components.min.js` |
| `apidocs.Redoc` | `redoc` 2.5.4 | `redoc.standalone.js` |

## Serve the files yourself

Set `AssetsURL` when the docs must work offline, or when your Content-Security-Policy doesn't allow the CDN. Download the files `apidocs.Files(ui)` lists, at the pinned versions, and serve them under that URL:

```go
for _, f := range apidocs.Files(apidocs.Scalar) {
	fmt.Println(f.Name, f.URL)
}
// standalone.js https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.72.1/dist/browser/standalone.js
```

```go
app.Static("/assets/apidocs", "./assets/apidocs") // holds standalone.js
app.Get("/docs", apidocs.New(apidocs.Config{AssetsURL: "/assets/apidocs"})).Hidden()
```

The page keeps each file's integrity hash, so a file from a different version doesn't run.

## Errors

`New` panics on an unknown `UI`, like other setup mistakes. Nothing fails per request: the page is fixed bytes.

When the page loads but shows no routes, the browser couldn't fetch the spec. Check that `Spec` matches the path you gave `app.OpenAPI`, and that any auth on the spec route lets the browser through.

## Security

- **Hiding the docs isn't access control.** Anyone who can reach your routes can call them. Protect private APIs with auth middleware, and protect `/docs` and `/openapi.json` too if their contents are private. `app.OpenAPI` serves the spec behind middleware in place of the default: `app.OpenAPI("/openapi.json", cfg, requireAPIKey)`.
- **Scripts from a CDN.** Every file is pinned to one version and loaded with a Subresource Integrity hash, so the browser refuses a file that has changed. Serve the files yourself with `AssetsURL` if you'd rather not depend on the CDN at all.
- **Content-Security-Policy.** Scalar and Swagger UI start with a short inline script. It's the same for every page, so a policy can allow it by hash rather than with `'unsafe-inline'`. The renderers also inject styles and load fonts and images, so start from a policy that works in your browser's console and tighten it from there. [Secure Headers](/middleware/secure/) sets the header.

## Related

- [Secure Headers](/middleware/secure/): send a Content-Security-Policy for your pages.
- [Key Auth](/middleware/keyauth/): require an API key, including on the docs and spec routes.
- [Static Files](/guide/static-files/): serve the renderer's files with `AssetsURL`.
