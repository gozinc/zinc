---
title: Responses and Rendering
description: Send JSON, text, HTML, templates, files, downloads, streams, and server-sent events, with the right status and headers.
---

Response helpers on `*zinc.Context` write the body, set `Content-Type`, and return an error you pass straight back. Set the status and headers first, then call exactly one body helper.

```go
return c.
	Status(zinc.StatusCreated).
	SetHeader(zinc.HeaderLocation, "/users/42").
	JSON(user)
```

## Quick reference

| To send | Use |
|---|---|
| JSON | `c.JSON(v)`, or `c.JSONPretty(v, "  ")` |
| XML, YAML, TOML | `c.XML(v)`, `c.YAML(v)`, `c.TOML(v)` |
| Text or HTML | `c.String(s)`, `c.HTML(s)` |
| Nothing | `c.NoContent()` (204) |
| Pre-encoded bytes | `c.JSONBlob`, `c.XMLBlob`, `c.HTMLBlob`, `c.Blob` |
| A template | `c.Render(name, data)` |
| A file | `c.File(path)`, `c.FileFS(name, fsys)` |
| A download | `c.Download(path, filename)`, `c.Inline(path)` |
| A stream | `c.Stream(contentType, reader)` |
| An event stream | `c.SSE(event)` |
| A redirect | `c.Redirect(code, url)` |

The status defaults to `200 OK`.

## Status and headers

```go
c.Status(zinc.StatusAccepted)
c.SetHeader("Cache-Control", "no-store")
c.AppendHeader("Vary", "Accept-Language")
c.Location("/jobs/81")
```

These methods return the context, so they chain into the body helper. Once a body helper runs, the response is committed. Changing headers afterwards has no effect.

## Structured data

```go
return c.JSON(zinc.Map{"ok": true})
return c.XML(invoice)
return c.YAML(config)
```

`zinc.Map` is shorthand for `map[string]any`. For bytes that are already encoded, the blob helpers write them unchanged:

```go
return c.JSONBlob(zinc.StatusOK, cachedJSON)
return c.Blob(zinc.StatusOK, "application/vnd.api+json", payload)
```

## Templates

Configure a renderer once, then render by name:

```go
views := template.Must(template.ParseGlob("views/*.html"))

app := zinc.New(zinc.Config{Renderer: zinc.NewHTMLTemplateRenderer(views)})

app.Get("/dashboard", func(c *zinc.Context) error {
	return c.Render("dashboard.html", zinc.Map{"Title": "Overview"})
})
```

[Templates](/guide/templates/) covers `text/template`, custom engines, and Templ.

## Files and downloads

```go
return c.File("./public/report.pdf")          // served with a detected content type
return c.FileFS("report.pdf", embeddedFiles)  // from any fs.FS
return c.Download("./exports/users.csv", "users-2026-09.csv") // "Save as" with a filename
return c.Inline("./public/report.pdf")        // display in the browser
```

:::caution[Paths from users]
Never build a file path from request input without validating it. The [File Download](/cookbook/file-download/) recipe shows a safe pattern.
:::

## Streams and server-sent events

`c.Stream` copies from any `io.Reader` without buffering the whole body:

```go
return c.Stream("text/csv", exportReader)
```

For server-sent events, call `c.SSE` once per event. Each event is flushed to the client as it is written:

```go
app.Get("/events", func(c *zinc.Context) error {
	for msg := range updates(c.Context()) {
		if err := c.SSE(zinc.SSEvent{Event: "update", Data: msg}); err != nil {
			return err
		}
	}
	return nil
})
```

The loop ends when the client disconnects and `c.Context()` is cancelled. `Config.WriteTimeout` applies to each event rather than to the whole stream, so a stream can stay open indefinitely. See the [Server-Sent Events](/cookbook/sse/) recipe for a complete program.

## Content negotiation

Branch on what the client accepts:

```go
switch c.Accepts("application/json", "text/html") {
case "application/json":
	return c.JSON(user)
case "text/html":
	return c.Render("user.html", user)
default:
	return zinc.ErrNotAcceptable
}
```

`Accepts` honours quality values and wildcards in the `Accept` header, and returns the first offer when the header is missing. When each type has a ready-made body, `Negotiate` picks and sends it in one call:

```go
return c.Negotiate(zinc.StatusOK, zinc.Map{
	"application/json": zinc.Map{"ok": true},
	"text/plain":       "ok",
})
```

## Cookies and redirects

```go
c.SetCookie(&http.Cookie{Name: "theme", Value: "dark", Path: "/"})
return c.Redirect(zinc.StatusSeeOther, "/dashboard")
```

[Cookies](/guide/cookies/) covers reading, clearing, and secure defaults.

## Next steps

- [Errors](/guide/errors/) for failure responses.
- [Static Files](/guide/static-files/) for serving whole directories.
- [Response Writer](/api/response-writer/) for middleware that inspects responses.
