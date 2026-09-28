---
title: Templates
description: Send HTML pages from Zinc handlers with html/template, text/template, Templ or another engine.
---

Templates let your server send HTML pages instead of JSON. You parse them once when the app starts, hand them to Zinc as a renderer, and call `c.Render` with a template name in any handler.

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
		return c.Render("home.html", zinc.Map{
			"Title": "Zinc",
			"Name":  zinc.QueryOr(c, "name", "world"),
		})
	})

	log.Fatal(app.Listen(":8080"))
}
```

```html
<!-- views/home.html -->
<!doctype html>
<title>{{.Title}}</title>
<h1>Hello, {{.Name}}</h1>
```

```bash
curl -i 'localhost:8080/?name=Ada'
# HTTP/1.1 200 OK
# Content-Type: text/html; charset=utf-8
#
# <!doctype html>
# <title>Zinc</title>
# <h1>Hello, Ada</h1>
```

`template.ParseGlob` names each template after its file, so `views/home.html` becomes `home.html`. `c.Render` runs that template with your data, sets `Content-Type: text/html; charset=utf-8` and sends the result.

`html/template` escapes what you pass in, so user input can't inject markup:

```bash
curl 'localhost:8080/?name=<b>Ada</b>'
# ...
# <h1>Hello, &lt;b&gt;Ada&lt;/b&gt;</h1>
```

## Send a different status

`c.Render` sends `200` by default. Set another status first:

```go
app.NotFound(func(c *zinc.Context) error {
	return c.Status(zinc.StatusNotFound).Render("404.html", nil)
})
```

## Leave off the file extension

To write `c.Render("home")` instead of `c.Render("home.html")`, tell the renderer which extensions to try:

```go
renderer := zinc.NewHTMLTemplateRenderer(views,
	zinc.WithTemplateSuffixes(".html", ".tmpl"),
)
```

Zinc looks for the exact name first, then tries each suffix in order: `home`, then `home.html`, then `home.tmpl`. Both `c.Render("home")` and `c.Render("home.html")` keep working.

## Share a header or footer

Put a piece of HTML you reuse in its own `{{define}}` block, and include it with `{{template}}`:

```html
<!-- views/partials/nav.html -->
{{define "nav"}}<nav><a href="/">Home</a> <a href="/about">About</a></nav>{{end}}
```

```html
<!-- views/home.html -->
<!doctype html>
{{template "nav" .}}
<h1>Hello, {{.Name}}</h1>
```

Parse the partials into the same set as your pages. Each `ParseGlob` call reads one directory:

```go
views := template.Must(template.ParseGlob("views/*.html"))
template.Must(views.ParseGlob("views/partials/*.html"))
```

## Wrap pages in a layout

A layout is a page skeleton that each page fills in. The layout leaves gaps with `{{template}}` or `{{block}}`, and each page defines what goes in them:

```html
<!-- views/layout.html -->
<!doctype html>
<html>
<head><title>{{block "title" .}}My site{{end}}</title></head>
<body>
{{template "nav" .}}
<main>{{template "content" .}}</main>
</body>
</html>
```

```html
<!-- views/pages/home.html -->
{{define "title"}}Home{{end}}
{{define "content"}}<h1>Hello, {{.Name}}</h1>{{end}}
```

Every page defines `content`, so the pages can't share one template set: the last one parsed would win. Give each page its own set with a small renderer. Zinc accepts any type with a `Render(w io.Writer, name string, data any, c *zinc.Context) error` method:

```go
// pages holds one template set per page: the layout, the partials and that page.
type pages map[string]*template.Template

func loadPages() pages {
	base := template.Must(template.ParseFiles("views/layout.html"))
	template.Must(base.ParseGlob("views/partials/*.html"))

	files, err := filepath.Glob("views/pages/*.html")
	if err != nil {
		panic(err)
	}
	p := pages{}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".html")
		p[name] = template.Must(template.Must(base.Clone()).ParseFiles(file))
	}
	return p
}

func (p pages) Render(w io.Writer, name string, data any, _ *zinc.Context) error {
	page, ok := p[name]
	if !ok {
		return fmt.Errorf("unknown page %q", name)
	}
	return page.ExecuteTemplate(w, "layout.html", data)
}

app := zinc.New(zinc.Config{Renderer: loadPages()})

app.Get("/", func(c *zinc.Context) error {
	return c.Render("home", zinc.Map{"Name": "Ada"})
})
```

```bash
curl localhost:8080/
# <!doctype html>
# <html>
# <head><title>Home</title></head>
# <body>
# <nav><a href="/">Home</a> <a href="/about">About</a></nav>
# <main><h1>Hello, Ada</h1></main>
# </body>
# </html>
```

A page that doesn't define `title` gets the default from the `{{block}}`: `My site`.

## Use another engine

| Engine | Renderer |
|---|---|
| `html/template` | `zinc.NewHTMLTemplateRenderer(t)` |
| `text/template` | `zinc.NewTextTemplateRenderer(t)` |
| Anything with `ExecuteTemplate(w io.Writer, name string, data any) error` | `zinc.NewTemplateRenderer(engine)` |
| Anything else | Your own type with a `Render` method, like `pages` above |

All three constructors accept `zinc.WithTemplateSuffixes`.

## Render Templ components

[Templ](https://templ.guide) components render themselves, so they don't need a renderer. Set the content type, then render the component to the response writer with the request's context:

```go
app.Get("/", func(c *zinc.Context) error {
	c.SetHeader(zinc.HeaderContentType, "text/html; charset=utf-8")
	return views.Home("Zinc").Render(c.Context(), c.Writer()) // views is your generated Templ package
})
```

The [Templ UI](/cookbook/templ-ui/) recipe builds a full page with components and Tailwind.

## Good to know

### Errors send a 500, not half a page

`c.Render` renders the whole template before it sends anything. If the template fails, or the name doesn't exist, the handler returns the error and the client gets a `500` from your [error handler](/guide/errors/). No partial HTML reaches the browser.

```go
app.Get("/oops", func(c *zinc.Context) error {
	return c.Render("nope.html", nil) // no template has this name
})
```

```bash
curl localhost:8080/oops
# {"error":{"status":500,"message":"Internal Server Error"}}
```

Calling `c.Render` with no `Renderer` in `zinc.Config` also returns an error, and a `500`.

### text/template output is still sent as HTML

`c.Render` sets `Content-Type: text/html; charset=utf-8` unless the response already has one. To send plain text from a `text/template`, set the header first:

```go
c.SetHeader(zinc.HeaderContentType, "text/plain; charset=utf-8")
return c.Render("hello.txt", data)
```

`text/template` doesn't escape anything, so don't use it for HTML that includes user input.

## Next steps

- [Templated HTML + JS](/cookbook/templated-html-js-page/): a page with assets and a small browser script.
- [Responses and Rendering](/guide/responses-and-rendering/): JSON, files, streams and every other response type.
- [Static Files](/guide/static-files/): serve the CSS, JavaScript and images your pages link to.
