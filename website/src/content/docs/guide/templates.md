---
title: Templates
description: Render html/template, text/template, Templ, or any other engine from Zinc handlers.
---

Configure a renderer once when you build the app, then render templates by name from any handler.

```go
package main

import (
	"html/template"
	"log"

	"github.com/0mjs/zinc"
)

func main() {
	views := template.Must(template.ParseGlob("views/*.html"))

	app := zinc.New(zinc.Config{Renderer: zinc.NewHTMLTemplateRenderer(views)})

	app.Get("/", func(c *zinc.Context) error {
		return c.Render("home.html", zinc.Map{"Title": "Zinc"})
	})

	log.Fatal(app.Listen(":8080"))
}
```

`c.Render` executes the named template, sets `Content-Type: text/html; charset=utf-8`, and writes the result. Use `c.Status(code).Render(...)` to send a status other than `200`.

## Other engines

| Engine | Renderer |
|---|---|
| `html/template` | `zinc.NewHTMLTemplateRenderer(t)` |
| `text/template` | `zinc.NewTextTemplateRenderer(t)` |
| Anything with `ExecuteTemplate(w io.Writer, name string, data any) error` | `zinc.NewTemplateRenderer(engine)` |

## Names without file suffixes

Some engines register templates as `home` instead of `home.html`. Tell the renderer which suffixes to try so handlers can use either form:

```go
renderer := zinc.NewHTMLTemplateRenderer(views,
	zinc.WithTemplateSuffixes(".html", ".tmpl"),
)
```

## Templ

[Templ](https://templ.guide) components render themselves, so they do not need a renderer. Call the component with the request context and response writer:

```go
app.Get("/", func(c *zinc.Context) error {
	c.SetHeader(zinc.HeaderContentType, "text/html; charset=utf-8")
	return views.Home("Zinc").Render(c.Context(), c.Writer())
})
```

The [Templ UI](/cookbook/templ-ui/) recipe builds a full page with components and Tailwind.

## Next steps

- [Templated HTML + JS](/cookbook/templated-html-js-page/) serves a page with assets and progressive enhancement.
- [Responses and Rendering](/guide/responses-and-rendering/) for every other response type.
