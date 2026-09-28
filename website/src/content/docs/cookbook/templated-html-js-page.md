---
title: Templated HTML + JS Page
description: Render a page on the server with html/template, then add behavior in the browser with a small script.
---

This program renders an HTML page on the server and serves a JavaScript file that adds behavior once the page loads. You'd use this for pages that should show content straight away and need only a little interactivity, without a front-end framework or build step. It shows a template renderer, `c.Render` and `app.Static` working together.

## Run it

```bash
go mod init example.com/webpage
go get github.com/0mjs/zinc
mkdir -p templates public
```

The program expects this layout:

```text
.
├── main.go
├── public
│   └── app.js
└── templates
    └── home.html
```

```html title="templates/home.html"
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>{{ .Title }}</title>
  </head>
  <body>
    <main id="app" data-user="{{ .User }}" data-theme="{{ .Theme }}">
      <h1>{{ .Heading }}</h1>
      <p>The first render comes from Zinc templates.</p>
      <p id="status">Loading browser behavior...</p>
    </main>

    <script src="/static/app.js" defer></script>
  </body>
</html>
```

```js title="public/app.js"
const app = document.querySelector("#app");
const status = document.querySelector("#status");

if (app && status) {
  const user = app.dataset.user || "friend";
  const theme = app.dataset.theme || "default";

  status.textContent = `Ready. Welcome, ${user}. Theme: ${theme}.`;
}
```

## The program

```go title="main.go"
package main

import (
	"html/template"
	"log"

	"github.com/0mjs/zinc"
)

func main() {
	views := template.Must(template.ParseGlob("templates/*.html"))

	app := zinc.New(zinc.Config{
		Renderer: zinc.NewHTMLTemplateRenderer(views, zinc.WithTemplateSuffixes(".html")),
	})

	app.Static("/static", "./public")

	app.Get("/", func(c *zinc.Context) error {
		return c.Render("home", zinc.Map{
			"Title":   "Zinc Web Page",
			"Heading": "Hello from Zinc",
			"User":    zinc.QueryOr(c, "user", "friend"),
			"Theme":   "graphite",
		})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Open `http://localhost:8080/?user=Ada` in a browser. The page first says "Loading browser behavior...", then the script replaces it with "Ready. Welcome, Ada. Theme: graphite."

With curl you see the HTML the server sends, before any script runs:

```bash
curl -i 'http://localhost:8080/?user=Ada'
# HTTP/1.1 200 OK
# Content-Type: text/html; charset=utf-8
#
# <!doctype html>
# ...
#     <main id="app" data-user="Ada" data-theme="graphite">
#       <h1>Hello from Zinc</h1>
# ...
#     <script src="/static/app.js" defer></script>

curl -I http://localhost:8080/static/app.js
# HTTP/1.1 200 OK
# Content-Type: text/javascript; charset=utf-8
```

`html/template` escapes values for where they appear, so a hostile `user` can't break out of the attribute:

```bash
curl -s 'http://localhost:8080/?user=%22%3E%3Cscript%3E' | grep data-user
#     <main id="app" data-user="&#34;&gt;&lt;script&gt;" data-theme="graphite">
```

## How it works

- `template.ParseGlob("templates/*.html")` parses every template once, at startup. Each is named after its file, so `templates/home.html` becomes `home.html`.
- `zinc.WithTemplateSuffixes(".html")` lets `c.Render("home", ...)` find `home.html`. Zinc tries the exact name first, then each suffix.
- `c.Render` runs the template with your data, sets `Content-Type: text/html; charset=utf-8` and sends it.
- `app.Static("/static", "./public")` serves `public/app.js` at `/static/app.js`.
- The template passes server data to the script through `data-` attributes, which the script reads from `app.dataset`.

## Good to know

### Missing templates stop the program at startup

If `templates/` has no `.html` files, `template.Must` panics with `pattern matches no files` before the server starts. A name that matches no template at request time comes back as a `500`, and the error wraps `zinc.ErrTemplateNotFound`.

### Template changes need a restart

Templates are parsed once. Restart the program to see edits, or parse them inside the handler while developing.

## See also

- [Templates](/guide/templates/): renderers, layouts, and other template engines.
- [Static Files](/guide/static-files/): `app.Static` and its options.
- [Server-Rendered UI with Templ UI](/cookbook/templ-ui/): typed components instead of template files.
