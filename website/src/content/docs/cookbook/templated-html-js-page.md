---
title: Templated HTML + JS Page
description: Render a simple HTML page with Zinc templates and enhance it with a small browser script.
---

This recipe shows a normal server-rendered page shape:

- Zinc renders the initial HTML
- Zinc serves browser assets from `/static`
- a small JavaScript file progressively enhances the page after load

## Setup

```bash
go mod init zinc-web-page
go get github.com/0mjs/zinc
```

## Project layout

```text
.
├── main.go
├── public
│   └── app.js
└── templates
    └── home.html
```

## Application

```go
package main

import (
	"html/template"
	"log"

	"github.com/0mjs/zinc"
)

func main() {
	views := template.Must(template.ParseGlob("templates/*.html"))

	cfg := zinc.DefaultConfig
	cfg.Renderer = zinc.NewHTMLTemplateRenderer(views, zinc.WithTemplateSuffixes(".html"))
	app := zinc.NewWithConfig(cfg)

	if err := app.Static("/static", "./public"); err != nil {
		log.Fatal(err)
	}

	app.Get("/", func(c *zinc.Context) error {
		return c.Render("home", zinc.Map{
			"Title":   "Zinc Web Page",
			"Heading": "Hello from Zinc",
			"User":    "Matt",
			"Theme":   "graphite",
		})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Template

```html
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

## Browser script

```js
const app = document.querySelector("#app");
const status = document.querySelector("#status");

if (app && status) {
  const user = app.dataset.user || "friend";
  const theme = app.dataset.theme || "default";

  status.textContent = `Ready. Welcome, ${user}. Theme: ${theme}.`;
}
```

## Run it

```bash
go run .
# open http://localhost:8080
```

## Why this recipe matters

This is the cleanest pattern when you want:

- server-rendered HTML
- a little client-side behavior
- normal static asset handling
- no SPA runtime overhead
