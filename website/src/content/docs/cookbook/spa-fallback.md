---
title: Single-Page App
description: Serve a built React, Vue or Svelte app from Zinc, answer its client-side routes with index.html, and keep API 404s as JSON.
---

This program serves a built single-page app next to a JSON API. A browser that opens a client-side route such as `/settings/profile` gets `index.html`, so the app's router can draw the page. You'd use it to ship a front end and its API from one Go server, on one origin, with no CORS setup.

## Run it

Put your front end's build output in `dist/` next to `main.go`. With Vite, that's `npm run build`. To try the program without a front end, create a stand-in:

```bash
mkdir -p dist/assets
cat > dist/index.html <<'HTML'
<!doctype html>
<title>My App</title>
<div id="root"></div>
<script type="module" src="/assets/index-4f2a.js"></script>
HTML
echo 'document.querySelector("#root").textContent = "Route: " + location.pathname;' > dist/assets/index-4f2a.js
```

## The program

```go title="main.go"
package main

import (
	"errors"
	"log"
	"path"
	"strings"

	"github.com/0mjs/zinc"
)

// spaFallback answers a missed page route, such as /settings/profile, with
// index.html so the front end's router can take over. Missing files (paths
// with an extension) and anything under /api still get a 404.
func spaFallback(c *zinc.Context) error {
	err := c.Next()
	if !errors.Is(err, zinc.ErrNotFound) {
		return err
	}
	p := strings.ToLower(c.Path())
	if path.Ext(p) != "" || p == "/api" || strings.HasPrefix(p, "/api/") {
		return err
	}
	return c.File("dist/index.html")
}

func main() {
	app := zinc.New()

	// The API. Routes win over the static files below.
	app.Get("/api/me", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"name": "Ada"})
	})

	// The built front end, with index.html for client-side routes.
	site := app.Group("", spaFallback)
	site.Static("/", "./dist")

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Open `http://localhost:8080/settings/profile` in a browser. The page shows `Route: /settings/profile`, drawn by the script, not by the server.

With curl, a client-side route gets `index.html` with a `200`:

```bash
curl -i http://localhost:8080/settings/profile
# HTTP/1.1 200 OK
# Content-Length: 120
# Content-Type: text/html; charset=utf-8
#
# <!doctype html>
# <title>My App</title>
# <div id="root"></div>
# <script type="module" src="/assets/index-4f2a.js"></script>
```

Files that exist and API routes are served as usual:

```bash
curl http://localhost:8080/assets/index-4f2a.js
# document.querySelector("#root").textContent = "Route: " + location.pathname;

curl http://localhost:8080/api/me
# {"name":"Ada"}
```

A missing file or an unknown API route still gets a `404`. Without that, a stale script URL would load HTML, and an API client would get a web page instead of an error:

```bash
curl -i http://localhost:8080/assets/old.js
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":404,"message":"Not Found"}}

curl -i http://localhost:8080/api/nope
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":404,"message":"Not Found"}}
```

## How it works

- Zinc tries routes before static files, so `/api/me` reaches its handler even though `site.Static("/", ...)` covers every path.
- `app.Group("", spaFallback)` adds middleware without a path prefix. Middleware on a group also runs for the group's static files, so `spaFallback` wraps every file lookup below `/`.
- `c.Next()` returns the static handler's error. A miss is a `404`, so `errors.Is(err, zinc.ErrNotFound)` is true.
- Paths with a file extension, such as `.js` or `.ico`, are missing files, and paths under `/api` belong to the API. Both keep their `404`. The path is lowercased first because Zinc matches `/API/nope` like `/api/nope`.
- Everything else is a page, so `c.File("dist/index.html")` sends the app's entry point with a `200`.

## Before production

- Send `Cache-Control: no-cache` with `index.html`, so users pick up a new build straight away. Files under `assets/` have a content hash in their names and can be cached for a long time.
- If a client-side route has a dot in it, such as `/users/ada.lovelace`, the extension check treats it as a file. Change the check, or avoid dots in page URLs.
- Pages the app doesn't know about still get `index.html` with a `200`. Show a "not found" page from your front-end router.

## Good to know

### Ship one binary

To embed the build, give the group an `fs.FS` instead of a folder, and send `index.html` from it:

```go
//go:embed all:dist
var files embed.FS

dist, _ := fs.Sub(files, "dist")
site.StaticFS("/", dist)

// in spaFallback:
return c.FileFS("index.html", dist)
```

`all:` includes files whose names start with `_` or `.`, which some build tools produce. See [Embed Resources](/cookbook/embed-resources/).

### Other methods

The static files answer only `GET` and `HEAD`. A `POST` to a page path gets `405 Method Not Allowed` with `Allow: GET, HEAD`, and the fallback leaves it alone.

## See also

- [Static Files](/guide/static-files/): `Static`, `StaticFS` and how misses are reported.
- [Groups and Middleware](/guide/groups-and-middleware/): where group middleware runs.
- [CORS for a Browser Front End](/cookbook/cors-browser-frontend/): when the front end is served from a different origin instead.
