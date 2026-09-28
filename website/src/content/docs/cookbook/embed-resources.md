---
title: Embed Resources
description: Compile HTML, CSS and images into your Zinc binary, so one file is all you deploy.
---

This program serves a website from files compiled into the binary. You'd use it when you want to deploy one executable, with no folder of assets to copy next to it. Go's `embed` package puts the files in; `app.StaticFS` serves them.

## Run it

Create a `public/` folder next to `main.go` and put your site in it:

```text
.
├── main.go
└── public
    ├── index.html
    └── css
        └── app.css
```

```html title="public/index.html"
<!doctype html>
<title>Embedded</title>
<link rel="stylesheet" href="/css/app.css">
<h1>Served from inside the binary</h1>
```

```css title="public/css/app.css"
h1 { color: teal; }
```

The program won't compile until `public/` exists, because `//go:embed` needs something to embed.

## The program

```go title="main.go"
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/0mjs/zinc"
)

//go:embed public
var assets embed.FS

func main() {
	public, err := fs.Sub(assets, "public")
	if err != nil {
		log.Fatal(err)
	}

	app := zinc.New()
	app.StaticFS("/", public)

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Build the binary, then run it from anywhere. The files travel with it:

```bash
go build -o site .
mv site /tmp && cd /tmp && ./site
```

In another terminal:

```bash
curl -i http://localhost:8080/
# HTTP/1.1 200 OK
# Content-Type: text/html; charset=utf-8
#
# <!doctype html>
# <title>Embedded</title>
# ...

curl http://localhost:8080/css/app.css
# h1 { color: teal; }
```

A file that isn't in `public/` gets Zinc's normal 404:

```bash
curl -i http://localhost:8080/missing.txt
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":404,"message":"Not Found"}}
```

## How it works

- `//go:embed public` compiles the `public/` folder into the `assets` variable when you run `go build`.
- `fs.Sub(assets, "public")` drops the `public/` prefix, so `public/css/app.css` is served at `/css/app.css` rather than `/public/css/app.css`.
- `app.StaticFS("/", public)` serves the files below `/`. A request for a folder serves its `index.html`.
- Files are served with `http.ServeContent`, so range requests and `Content-Type` from the file extension work as they would from disk.

## Good to know

### Change the index file or list folders

`StaticFS` takes the same options as `Static`. Use `zinc.WithStaticIndex("home.html")` for a different index filename, or `zinc.WithStaticBrowse(true)` to show a listing for folders that have no index file:

```go
app.StaticFS("/", public, zinc.WithStaticIndex("home.html"))
```

### Hidden files aren't embedded

`//go:embed public` skips files whose names start with `.` or `_`. Write `//go:embed all:public` to include them.

### Edits need a rebuild

The files are fixed when you build. While you're working on the site, serve the folder from disk with `app.Static("/", "./public")` so a browser refresh picks up your changes.

## See also

- [Static files](/guide/static-files/): `Static`, `StaticFS` and their options.
- [Single-Page App](/cookbook/spa-fallback/): serve a built front end and answer its client-side routes.
- [File Download](/cookbook/file-download/): send one file, inline or as a download.
