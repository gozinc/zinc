---
title: Static Files
description: Serve a folder of assets, a single file, or files embedded in your binary.
---

Static files let you serve CSS, JavaScript, images and other assets straight from disk or from your binary. Use them for a site's assets, a `robots.txt`, or a small front end that ships with your API.

```go
app.Static("/assets", "./public")
```

```bash
curl -i http://localhost:8080/assets/css/app.css
# HTTP/1.1 200 OK
# Accept-Ranges: bytes
# Content-Length: 22
# Content-Type: text/css; charset=utf-8
# Last-Modified: Mon, 28 Sep 2026 00:04:18 GMT
#
# body { color: #222; }
```

`GET /assets/css/app.css` serves `./public/css/app.css`. Zinc sets the content type from the file extension and answers `GET` and `HEAD`.

`Range` and conditional requests work too, because Zinc serves files with `http.ServeContent`:

```bash
curl -i -H "Range: bytes=0-3" http://localhost:8080/assets/css/app.css
# HTTP/1.1 206 Partial Content
# Content-Range: bytes 0-3/22
# Content-Type: text/css; charset=utf-8
#
# body
```

## What clients get for bad requests

A missing file, or a path that tries to leave the folder, gets a `404`. Any method other than `GET` or `HEAD` gets a `405`:

```bash
curl -i http://localhost:8080/assets/nope.css
# HTTP/1.1 404 Not Found
# {"error":{"status":404,"message":"Not Found"}}

curl -i --path-as-is http://localhost:8080/assets/../go.mod
# HTTP/1.1 404 Not Found
# {"error":{"status":404,"message":"Not Found"}}

curl -i -X POST http://localhost:8080/assets/css/app.css
# HTTP/1.1 405 Method Not Allowed
# Allow: GET, HEAD
# {"error":{"status":405,"message":"Method Not Allowed"}}
```

Both go through your [error handler](/guide/errors/), like any other error.

## Serve a single file

```go
app.File("/robots.txt", "./public/robots.txt")
```

```bash
curl http://localhost:8080/robots.txt
# User-agent: *
# Disallow:
```

`File` registers an ordinary `GET` route, so it returns a `Route` you can name.

## Serve embedded files

Compile your assets into the binary with `embed`, then serve any `fs.FS` with `StaticFS` and `FileFS`:

```go
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/0mjs/zinc"
)

//go:embed public
var public embed.FS

func main() {
	app := zinc.New()

	assets, err := fs.Sub(public, "public")
	if err != nil {
		log.Fatal(err)
	}
	app.StaticFS("/assets", assets)
	app.FileFS("/favicon.ico", "favicon.ico", assets)

	log.Fatal(app.Listen(":8080"))
}
```

`fs.Sub` strips the `public/` prefix, so `/assets/css/app.css` reads `public/css/app.css` from the binary. The [Embed Resources](/cookbook/embed-resources/) recipe has a runnable version.

## Choose the index file, or list a folder

A request for a folder serves `index.html` from it. Change the file name, or list the folder's contents when there's no index:

```go
app.Static("/docs", "./site",
	zinc.WithStaticIndex("home.html"), // served for folder requests; default "index.html"
	zinc.WithStaticBrowse(true),       // list files when there is no index
)
```

```bash
curl http://localhost:8080/docs/
# <h1>Home</h1>

curl http://localhost:8080/docs/guides/
# <!doctype html><html><body><ul><li>a.md</li><li>b.md</li></ul></body></html>
```

Folder listing is off by default: without an index file, the folder gets a `404`. Turn listing on only for content you intend to publish.

A folder's URL ends with a slash, so relative links in its index, such as `href="style.css"`, resolve inside the folder. The URL without the slash redirects there, keeping the query string, as `http.FileServer` does:

```bash
curl -i "http://localhost:8080/docs/guides?page=2"
# HTTP/1.1 301 Moved Permanently
# Location: /docs/guides/?page=2
```

[Trailing Slash](/middleware/trailingslash/) leaves a folder's slash alone, so removing slashes never loops with this redirect.

## Serve files from a group

`Group.Static` and `Group.StaticFS` put the folder under the group's prefix and run the group's middleware first:

```go
admin := app.Group("/admin", requireAdmin) // requireAdmin: your middleware
admin.Static("/files", "./private")        // GET /admin/files/report.pdf
```

## Use an existing file server

A standard `http.FileServer`, or any other handler, can own a path instead:

```go
app.Mount("/files", http.FileServer(http.Dir("./shared"))) // Mount strips "/files" itself
```

Prefer `Static` and `StaticFS` in new code. They reject methods other than `GET` and `HEAD`, never list folders unless you ask, and serve an index file by default.

## Share paths between files and routes

A route always wins over a static folder. With `app.Static("/assets", "./public")`, a `GET /assets/version` route still runs your handler.

To serve files from the site root next to your routes, let routes match first and serve files from the not-found handler:

```go
files := http.FileServerFS(os.DirFS("./public"))
app.NotFound(func(c *zinc.Context) error {
	files.ServeHTTP(c.Writer(), c.Request())
	return nil
})
```

```bash
curl http://localhost:8080/api/health   # ok (your route)
curl http://localhost:8080/css/app.css  # body { color: #222; }
curl http://localhost:8080/nope         # 404 page not found
```

:::caution[Folders without an index are listed]
Unlike `Static`, `http.FileServer` lists any folder that has no index file. From the not-found handler the listing comes back with a `404` status, but the file names are still sent. Keep such folders out of `./public`.
:::

## Good to know

### A missing folder isn't caught at startup

`Static` doesn't check the folder when you call it. If the path is wrong, every request gets a `404`. To stop the process on a bad deploy path, check it yourself:

```go
if _, err := os.Stat("./public"); err != nil {
	log.Fatal(err)
}
app.Static("/assets", "./public")
```

### Zinc keeps the folder open

- On the first file request, `Static` opens the folder once and reuses that handle for every later request.
- Symlinks are followed only while they stay inside the folder.
- If you rename or replace the folder after the first request, Zinc keeps serving the original until you create the app again. If assets must change without a restart, use `StaticFS` with a filesystem you manage.

### Releasing the folder

`app.Shutdown(ctx)` releases the folder handle after requests drain. If you serve the app from your own `http.Server` as an `http.Handler`, call `app.Close()` after that server stops.

`StaticFS` uses the filesystem you pass in. Zinc never closes it: it stays yours.

## Next steps

- [Embed Resources](/cookbook/embed-resources/): ship a single binary with its assets.
- [Responses and Rendering](/guide/responses-and-rendering/): send one file from a handler with `c.File` and `c.FileFS`.
- [Zinc and net/http](/guide/http-interoperability/): mount standard handlers and middleware.
