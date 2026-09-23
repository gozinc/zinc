---
title: Request Data
description: Read path parameters, query strings, headers, forms, uploaded files, raw bodies, and cancellation from a request.
---

Everything about the incoming request is available on `*zinc.Context`. This page covers reading values one at a time. To decode many values into a struct at once, use [binding](/guide/binding/).

## Path parameters

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	id := c.Param("id")
	return c.String(id)
})
```

`c.ParamOr("id", "me")` returns a fallback when a parameter is empty, which is useful with wildcards.

## Query strings

```go
// GET /search?q=zinc&tag=go&tag=http&page=2
app.Get("/search", func(c *zinc.Context) error {
	q := c.Query("q")               // "zinc"
	page := c.QueryOr("page", "1")  // "2", or "1" when missing
	tags := c.QueryArray("tag")     // ["go", "http"]
	return c.JSON(zinc.Map{"q": q, "page": page, "tags": tags})
})
```

For bracket-style keys such as `?filter[status]=open&filter[owner]=me`, `c.QueryMap("filter")` returns `map[string]string{"status": "open", "owner": "me"}`.

## Headers

```go
token := c.GetHeader(zinc.HeaderAuthorization)
contentType := c.ContentType() // media type without parameters, such as "application/json"
```

Zinc defines constants for common header names, such as `zinc.HeaderAuthorization` and `zinc.HeaderContentType`.

## Forms

```go
app.Post("/profile", func(c *zinc.Context) error {
	name := c.PostForm("name")
	roles := c.PostFormArray("roles")
	return c.JSON(zinc.Map{"name": name, "roles": roles})
})
```

`PostForm` reads URL-encoded and multipart bodies. `PostFormOr` and `PostFormMap` mirror their query counterparts.

## Uploaded files

```go
app.Post("/documents", func(c *zinc.Context) error {
	file, err := c.FormFile("document")
	if err != nil {
		return zinc.ErrBadRequest.WithMessage("document is required")
	}

	// Never trust the client's filename. Keep only its base name, or generate your own.
	name := filepath.Base(file.Filename)
	if err := c.SaveFile(file, filepath.Join("uploads", name)); err != nil {
		return err
	}
	return c.Status(zinc.StatusCreated).JSON(zinc.Map{"saved": name})
})
```

Use `c.FormFiles("documents")` for several files under one field, and `c.MultipartForm()` for full access to every value and file. The [File Upload](/cookbook/file-upload/) recipe shows a complete program.

## Raw body

```go
body, err := c.BodyBytes()
if err != nil {
	return err
}
```

`BodyBytes` and `BodyString` cache the body, so middleware can read it and binding still works afterwards.

## Cancellation and deadlines

The request's `context.Context` is cancelled when the client disconnects or a deadline passes. Pass it to anything that can block:

```go
app.Get("/report", func(c *zinc.Context) error {
	rows, err := db.QueryContext(c.Context(), reportSQL)
	if err != nil {
		return err
	}
	defer rows.Close()
	// ...
	return c.JSON(report)
})
```

Middleware can attach values or a tighter deadline with `c.SetContext(ctx)`. [Context Timeout](/middleware/context-timeout/) does exactly that.

## The underlying request

Zinc never hides the standard request. `c.Request()` returns the `*http.Request`, so anything in the standard library works:

```go
agent := c.Request().UserAgent()
host := c.Request().Host
```

## Next steps

- [Binding](/guide/binding/) decodes path, query, headers, and bodies into typed structs.
- [Client IP and Proxies](/guide/ip-address/) reads the client address safely.
- [Context API](/api/context/) lists every request helper.
