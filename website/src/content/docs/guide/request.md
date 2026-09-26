---
title: Request Data
description: Read path parameters, query strings, headers, forms, uploaded files, raw bodies, and cancellation from a request.
---

Everything about the incoming request is available on `*zinc.Context`. This page covers reading values one at a time. To decode many values into a struct at once, use [binding](/guide/binding/).

## Path parameters

`c.Param` returns a parameter as a string. `zinc.Param` parses it to the type you ask for:

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	id, err := zinc.Param[int64](c, "id")
	if err != nil {
		return err // 400: {"fields":{"id":"must be an integer"}}
	}
	return c.JSON(users.Find(id))
})
```

## Query strings

```go
// GET /search?q=zinc&tag=go&tag=http&page=2
app.Get("/search", func(c *zinc.Context) error {
	q := c.Query("q")                  // "zinc"
	page := zinc.QueryOr(c, "page", 1) // 2, or 1 when missing or not a number
	tags := c.QueryArray("tag")        // ["go", "http"]
	return c.JSON(zinc.Map{"q": q, "page": page, "tags": tags})
})
```

`zinc.QueryOr` infers the type from its fallback. For a value the request must include, use `zinc.Query`, which reports a missing or unparsable value as a 400 naming the parameter:

```go
since, err := zinc.Query[time.Time](c, "since")
if err != nil {
	return err
}
```

For bracket-style keys such as `?filter[status]=open&filter[owner]=me`, `c.QueryMap("filter")` returns `map[string]string{"status": "open", "owner": "me"}`.

## Typed values

`zinc.Param`, `zinc.Query`, and `zinc.Form` parse to:

- `string`, `bool`, any integer type, `float32`, and `float64`;
- any type implementing `encoding.TextUnmarshaler`, such as `time.Time` or a custom ID;
- named types over those, such as `type UserID int64`, and pointers to any of them.

They return a `*zinc.BindError` on failure. Return it, and the client gets a 400 naming the value. `zinc.QueryOr` and `zinc.FormOr` never fail; they fall back instead.

## Headers

```go
token := c.Header(zinc.HeaderAuthorization)
contentType := c.ContentType() // media type without parameters, such as "application/json"
```

Zinc defines constants for common header names, such as `zinc.HeaderAuthorization` and `zinc.HeaderContentType`.

## Forms

```go
app.Post("/profile", func(c *zinc.Context) error {
	name := c.FormValue("name")
	age, err := zinc.Form[int](c, "age")
	if err != nil {
		return err
	}
	return c.JSON(zinc.Map{"name": name, "age": age})
})
```

`c.FormValue`, `zinc.Form`, and `zinc.FormOr` read URL-encoded and multipart bodies, falling back to the query string as `http.Request.FormValue` does. For repeated fields, bind into a struct with a slice field; see [Binding](/guide/binding/).

## Uploaded files

```go
app.Post("/documents", func(c *zinc.Context) error {
	file, err := c.FormFile("document")
	if err != nil {
		return zinc.BadRequest("document is required")
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

Middleware can attach values or a tighter deadline with `c.SetContext(ctx)`. [Context Timeout](/middleware/timeout/) does exactly that.

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
