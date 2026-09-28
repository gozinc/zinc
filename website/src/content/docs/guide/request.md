---
title: Request Data
description: Read path parameters, query strings, headers, forms, uploaded files, raw bodies, and cancellation from a request.
---

This page shows how to read one value at a time from a request: a path parameter, a query value, a header, a form field or a file. To read many values into a struct in one call, use [binding](/guide/binding/) instead.

| To read | Use |
|---|---|
| A path parameter | `c.Param("id")`, or `zinc.Param[int64](c, "id")` for a typed value |
| A query value | `c.Query("q")`, `zinc.QueryOr(c, "page", 1)`, `zinc.Query[T](c, "since")` |
| A repeated query value | `c.QueryArray("tag")` |
| Bracket-style query keys | `c.QueryMap("filter")` |
| A header | `c.Header(zinc.HeaderAuthorization)`, `c.ContentType()` |
| A form field | `c.FormValue("name")`, `zinc.Form[T]`, `zinc.FormOr` |
| An uploaded file | `c.FormFile("document")`, `c.FormFiles`, `c.MultipartForm()` |
| The raw body | `c.BodyBytes()`, `c.BodyString()` |
| Cancellation and deadlines | `c.Context()` |
| Anything else | `c.Request()`, the standard `*http.Request` |

## Read a path parameter

`c.Param` returns a parameter as a string. `zinc.Param` parses it to the type you ask for:

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	id, err := zinc.Param[int64](c, "id")
	if err != nil {
		return err
	}
	return c.JSON(zinc.Map{"id": id})
})
```

```bash
curl http://localhost:8080/users/42
# {"id":42}

curl http://localhost:8080/users/abc
# {"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}
```

## Read query values

```go
// GET /search?q=zinc&tag=go&tag=http&page=2
app.Get("/search", func(c *zinc.Context) error {
	q := c.Query("q")                  // "zinc"
	page := zinc.QueryOr(c, "page", 1) // 2, as an int
	tags := c.QueryArray("tag")        // ["go", "http"]
	return c.JSON(zinc.Map{"q": q, "page": page, "tags": tags})
})
```

```bash
curl 'http://localhost:8080/search?q=zinc&tag=go&tag=http&page=2'
# {"page":2,"q":"zinc","tags":["go","http"]}

curl 'http://localhost:8080/search?q=zinc&page=abc'
# {"page":1,"q":"zinc","tags":[]}
```

`zinc.QueryOr` takes its type from the fallback: `1` is an `int`, so `page` is an `int`. When the value is missing or doesn't parse, you get the fallback. `c.Query` returns `""` for a missing value.

### Require a query value

When the request must include a value, use `zinc.Query`. A missing or unparsable value comes back as a `400` that names the parameter:

```go
app.Get("/events", func(c *zinc.Context) error {
	since, err := zinc.Query[time.Time](c, "since")
	if err != nil {
		return err
	}
	return c.JSON(zinc.Map{"since": since})
})
```

```bash
curl 'http://localhost:8080/events?since=2026-09-01T00:00:00Z'
# {"since":"2026-09-01T00:00:00Z"}

curl http://localhost:8080/events
# {"error":{"status":400,"message":"invalid query parameter","fields":{"since":"is required"}}}

curl 'http://localhost:8080/events?since=yesterday'
# {"error":{"status":400,"message":"invalid query parameter","fields":{"since":"invalid value"}}}
```

### Read bracket-style keys

For keys such as `?filter[status]=open&filter[owner]=me`, `c.QueryMap` collects everything under one name:

```go
filter := c.QueryMap("filter") // map[string]string{"owner": "me", "status": "open"}
```

## Read a header

```go
token := c.Header(zinc.HeaderAuthorization) // "Bearer abc"
contentType := c.ContentType()              // "application/json", without "; charset=utf-8"
```

Zinc defines constants for common header names, such as `zinc.HeaderAuthorization` and `zinc.HeaderContentType`. Any other name works as a plain string.

## Read form fields

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

```bash
curl http://localhost:8080/profile -d name=Ada -d age=36
# {"age":36,"name":"Ada"}

curl http://localhost:8080/profile -d name=Ada -d age=old
# {"error":{"status":400,"message":"invalid form field","fields":{"age":"must be an integer"}}}
```

`c.FormValue`, `zinc.Form` and `zinc.FormOr` read URL-encoded and multipart bodies. If the body doesn't have the field, they fall back to the query string, as `http.Request.FormValue` does. For a field that repeats, bind into a struct with a slice field; see [Binding](/guide/binding/).

## Accept a file upload

```go
app.Post("/documents", func(c *zinc.Context) error {
	file, err := c.FormFile("document")
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
		return zinc.UnprocessableEntity("document is required")
	}
	if err != nil {
		return err // for example 413 when the upload is over the body limit
	}

	// Never trust the client's filename. Keep only its base name, or generate your own.
	name := filepath.Base(file.Filename)
	if err := c.SaveFile(file, filepath.Join("uploads", name)); err != nil {
		return err
	}
	return c.Status(zinc.StatusCreated).JSON(zinc.Map{"saved": name})
})
```

```bash
curl http://localhost:8080/documents -F document=@report.pdf
# {"saved":"report.pdf"}

curl http://localhost:8080/documents -d x=1
# {"error":{"status":422,"message":"document is required"}}
```

`c.SaveFile` creates the `uploads` directory if it doesn't exist. Use `c.FormFiles("documents")` for several files under one field, and `c.MultipartForm()` for every value and file at once. The [File Upload](/cookbook/file-upload/) recipe shows a complete program.

## Read the raw body

```go
body, err := c.BodyBytes()
if err != nil {
	return err
}
```

`BodyBytes` and `BodyString` keep a copy of the body. Middleware can read it, and the handler can still bind it afterwards.

## Stop work when the client disconnects

`c.Context()` returns the request's `context.Context`. It's cancelled when the client disconnects or a deadline passes, so pass it to anything that can block:

```go
app.Get("/report", func(c *zinc.Context) error {
	rows, err := db.QueryContext(c.Context(), reportSQL) // db: your *sql.DB
	if err != nil {
		return err
	}
	defer rows.Close()

	report := buildReport(rows) // your code
	return c.JSON(report)
})
```

Middleware can attach values or a tighter deadline with `c.SetContext(ctx)`. The [Context Timeout](/middleware/timeout/) middleware uses it to give every request a deadline.

## Use the standard request

`c.Request()` returns the `*http.Request`, so anything in the standard library works:

```go
agent := c.Request().UserAgent() // "curl/8.7.1"
host := c.Request().Host         // "localhost:8080"
```

For the client's IP address, use `c.IP()` rather than `RemoteAddr`, so it's right behind a proxy; see [Client IP and Proxies](/guide/ip-address/).

## Good to know

### Which types the typed helpers accept

`zinc.Param`, `zinc.Query` and `zinc.Form` parse to:

- `string`, `bool`, any integer type, `float32` and `float64`.
- Any type with an `UnmarshalText` method (`encoding.TextUnmarshaler`), such as `time.Time` or your own ID type. `time.Time` expects RFC 3339, such as `2026-09-01T00:00:00Z`.
- Your own types built on those, such as `type UserID int64`, and pointers to any of them.

On failure they return a `*zinc.BindError`. Return it, and the client gets a `400` naming the value. `zinc.QueryOr` and `zinc.FormOr` never fail; they return the fallback instead.

### Bodies have a size limit

Zinc reads at most `BodyLimit` bytes of a body, 4 MiB by default. For a larger body, `BodyBytes`, `BodyString`, `FormFile`, `MultipartForm` and binding return an error: return it, and the client gets `413 Request Entity Too Large`. `c.FormValue` returns `""` instead, and `zinc.Form` reports the field as missing. Change the limit in [`zinc.Config`](/guide/configuration/).

## Next steps

- [Binding](/guide/binding/): read path, query, headers and body into a struct in one call.
- [Client IP and Proxies](/guide/ip-address/): read the client address safely behind a proxy.
- [Context API](/api/context/): every request helper, in one list.
