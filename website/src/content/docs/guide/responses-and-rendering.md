---
title: Responses and Rendering
description: Send JSON, text, HTML, templates, files, downloads, streams and server-sent events, with the right status and headers.
---

Response helpers on `*zinc.Context` send the body, set `Content-Type`, and return an error you pass straight back. Every handler ends with one of them.

```go
app.Post("/users", func(c *zinc.Context) error {
	user := User{ID: 42, Name: "Ada"} // your data layer
	return c.
		Status(zinc.StatusCreated).
		SetHeader(zinc.HeaderLocation, "/users/42").
		JSON(user)
})
```

```bash
curl -i -X POST http://localhost:8080/users
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
# Location: /users/42
#
# {"id":42,"name":"Ada"}
```

The status defaults to `200 OK`. Set it and any headers first, then call one body helper.

## Quick reference

| To send | Use |
|---|---|
| JSON | `c.JSON(v)`, or `c.JSONPretty(v, "  ")` |
| XML | `c.XML(v)` |
| Another format, such as YAML | `c.Encode(mediaType, v)`, with an [encoder](/guide/customization/#body-formats) |
| Text or HTML | `c.String(s)`, `c.HTML(s)` |
| Nothing | `c.NoContent()` (204) |
| Bytes that are already encoded | `c.Data(contentType, b)` |
| A template | `c.Render(name, data)` |
| A file | `c.File(path)`, `c.FileFS(name, fsys)` |
| A download | `c.Attachment(path, filename)`, `c.Inline(path)` |
| A stream | `c.Stream(contentType, reader)` |
| Server-sent events | `c.SSE(event)` |
| A redirect | `c.Redirect(url)` (302), or `c.Status(code).Redirect(url)` |

## Set the status and headers

```go
app.Post("/jobs", func(c *zinc.Context) error {
	c.Status(zinc.StatusAccepted)
	c.SetHeader("Cache-Control", "no-store")
	c.AppendHeader("Vary", "Accept-Language")
	c.Location("/jobs/81")
	return c.JSON(zinc.Map{"id": 81})
})
```

```bash
curl -i -X POST http://localhost:8080/jobs
# HTTP/1.1 202 Accepted
# Cache-Control: no-store
# Content-Type: application/json; charset=utf-8
# Location: /jobs/81
# Vary: Accept-Language
#
# {"id":81}
```

`SetHeader` replaces a header and `AppendHeader` adds another value. These methods return the context, so you can chain them into the body helper as in the first example.

:::caution[Set headers before the body]
The status and headers are sent when the body helper runs. Anything you change after `c.JSON(...)` is ignored: a later `c.Status(500)` or `c.SetHeader(...)` never reaches the client.
:::

## Send JSON, XML or another format

```go
return c.JSON(zinc.Map{"ok": true})                 // {"ok":true}
return c.XML(invoice)                               // <invoice><id>7</id></invoice>
return c.Encode("application/yaml", config)         // needs an encoder in Config.Encoders
```

`zinc.Map` is shorthand for `map[string]any`. `JSONPretty` indents the output, which helps in debug endpoints:

```go
return c.JSONPretty(zinc.Map{"ok": true, "n": 1}, "  ")
// {
//   "n": 1,
//   "ok": true
// }
```

If the bytes are already encoded, such as a cached JSON document, `c.Data` sends them unchanged with the type you give:

```go
return c.Data(zinc.MIMEJSON, cachedJSON)
return c.Status(zinc.StatusOK).Data("application/vnd.api+json", payload)
```

## Send text, HTML or nothing

```go
return c.String("hello")      // Content-Type: text/plain; charset=utf-8
return c.HTML("<h1>Hi</h1>")  // Content-Type: text/html; charset=utf-8
return c.NoContent()          // 204 No Content, no body
```

`NoContent` sends `204` unless you've set another status with `c.Status`.

## Render a template

Configure a renderer once, then render by name:

```go
views := template.Must(template.ParseGlob("views/*.html"))

app := zinc.New(zinc.Config{Renderer: zinc.NewHTMLTemplateRenderer(views)})

app.Get("/dashboard", func(c *zinc.Context) error {
	return c.Render("dashboard.html", zinc.Map{"Title": "Overview"})
})
```

The page is sent as `text/html; charset=utf-8`. [Templates](/guide/templates/) covers `text/template`, custom engines and Templ.

## Send a file or download

```go
return c.File("./public/report.pdf")                            // Content-Type from the extension
return c.FileFS("report.pdf", embeddedFiles)                    // from any fs.FS, such as embed.FS
return c.Attachment("./exports/users.csv", "users-2026-09.csv") // browser saves it under this name
return c.Inline("./public/report.pdf")                          // browser displays it
```

```bash
curl -i http://localhost:8080/download
# HTTP/1.1 200 OK
# Accept-Ranges: bytes
# Content-Disposition: attachment; filename="users-2026-09.csv"
# Content-Type: text/csv; charset=utf-8
# Last-Modified: Mon, 28 Sep 2026 00:05:16 GMT
#
# id,name
# 1,Ada
```

Files are served by Go's `http.ServeContent`, so range requests and `If-Modified-Since` work. A missing file returns a `404` error, which goes through your [error handler](/guide/errors/).

:::caution[Paths from users]
Never build a file path from request input without checking it. The [File Download](/cookbook/file-download/) recipe shows a safe pattern.
:::

## Stream a response

`c.Stream` copies from any `io.Reader` to the client, without holding the whole body in memory:

```go
return c.Stream("text/csv", exportReader) // exportReader: your io.Reader
```

## Stream server-sent events

Call `c.SSE` once per event. Each event is sent to the client as soon as it's written:

```go
app.Get("/events", func(c *zinc.Context) error {
	for msg := range updates(c.Context()) { // updates: your channel of messages
		if err := c.SSE(zinc.Event{Event: "update", Data: msg}); err != nil {
			return err
		}
	}
	return nil
})
```

```bash
curl -N http://localhost:8080/events
# event: update
# data: hello
#
# event: update
# data: {"n":2}
```

Strings and bytes are sent as they are; other values are sent as JSON. `zinc.Event` also has `ID` and `Retry` fields, and a multi-line `Data` becomes several `data:` lines.

The loop ends when the client disconnects, because `c.Context()` is cancelled. `Config.WriteTimeout` (10 seconds by default) limits how long each event may take to write, not the whole stream, so a stream can stay open as long as you keep sending. See the [Server-Sent Events](/cookbook/sse/) recipe for a complete program.

## Pick a format from the Accept header

Use `c.Accepts` to see which of your formats the client prefers:

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	user := User{ID: 42, Name: "Ada"} // your data layer
	switch c.Accepts("application/json", "text/html") {
	case "application/json":
		return c.JSON(user)
	case "text/html":
		return c.Render("user.html", user)
	default:
		return zinc.ErrNotAcceptable
	}
})
```

```bash
curl -H "Accept: text/html;q=0.5, application/json" http://localhost:8080/users/42
# {"id":42,"name":"Ada"}

curl -H "Accept: image/png" http://localhost:8080/users/42
# {"error":{"status":406,"message":"Not Acceptable"}}
```

`Accepts` follows the client's preference weights (`q=0.5`) and wildcards such as `*/*` and `text/*`. With no `Accept` header, it returns the first type you listed.

When each format has a ready-made body, `Negotiate` picks one and sends it in one call. It returns `zinc.ErrNotAcceptable` (406) if nothing matches:

```go
return c.Negotiate(zinc.Map{
	"application/json": zinc.Map{"ok": true},
	"text/plain":       "ok",
})
```

```bash
curl -H "Accept: text/plain" http://localhost:8080/health   # ok
curl http://localhost:8080/health                           # {"ok":true}
```

## Redirect

```go
app.Post("/login", func(c *zinc.Context) error {
	c.SetCookie(&http.Cookie{Name: "theme", Value: "dark", Path: "/"})
	return c.Status(zinc.StatusSeeOther).Redirect("/dashboard")
})
```

```bash
curl -i -X POST http://localhost:8080/login
# HTTP/1.1 303 See Other
# Location: /dashboard
# Set-Cookie: theme=dark; Path=/
```

:::note[Which redirect status]
`Redirect` sends `302 Found` unless you set a 3xx status first. Use `303 See Other` after a form post, so the browser follows with a `GET`. Use `307` or `308` to keep the method and body, and `301` or `308` for a permanent move.
:::

[Cookies](/guide/cookies/) covers reading, clearing and secure defaults.

## Good to know

### One body per response

A second body helper in the same handler returns `zinc.ErrResponseAlreadySent`, and the client still sees only the first response. `c.SSE` is the exception: call it as many times as you like.

### Status codes without a body

For a `HEAD` request, or a `204` or `304` status, body helpers send the status and headers and skip the body.

### Negotiate with no Accept header

`Negotiate` offers your types in alphabetical order. With no `Accept` header, or `Accept: */*`, the first one alphabetically wins: `application/json` before `text/plain`. Use `Accepts` if you need a different default.

### Other helpers

- `c.Send(v)` chooses by type: a `string` as text, `[]byte` as `application/octet-stream`, anything else as JSON.
- `c.Type("css")` sets `Content-Type` from a file extension (`text/css; charset=utf-8`).
- `c.Vary("Accept")` adds to the `Vary` header.

## Next steps

- [Errors](/guide/errors/): send failure responses in one consistent format.
- [Static Files](/guide/static-files/): serve whole directories.
- [Templates](/guide/templates/): render HTML pages.
- [Response Writer](/api/response-writer/): write middleware that inspects responses.
