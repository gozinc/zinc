---
title: Binding
description: Reference for c.Bind(), struct tags, BindError, Validator, and body decoders.
---

`c.Bind()` returns a binder for the current request. Each method decodes into a pointer to a struct, then runs the configured `Validator`. See the [Binding guide](/guide/binding/) for examples.

## Methods

| Method | Reads |
|---|---|
| `All(&v)` | Route parameters, then query values, then the body |
| `Path(&v)` | Route parameters (`path` tags) |
| `Query(&v)` | The query string (`query` tags) |
| `Header(&v)` | Request headers (`header` tags) |
| `Form(&v)` | URL-encoded or multipart forms (`form` tags) |
| `Body(&v)` | The body, decoded according to `Content-Type` |
| `JSON(&v)`, `XML(&v)`, `Text(&v)` | The body in one format, regardless of `Content-Type`. An empty body is an error. |

## Struct tags

| Tag | Example |
|---|---|
| `path` | `` ID int `path:"id"` `` |
| `query` | `` Page int `query:"page"` `` |
| `header` | `` Tenant string `header:"X-Tenant"` `` |
| `form` | `` Avatar *multipart.FileHeader `form:"avatar"` `` |
| `json`, `xml` | Standard encoding tags for the body; a configured decoder uses its library's tags, such as `yaml` |

A field binds from path, query, header, or form only when it has that source's tag. `query:",omitempty"` opts in under the lower-cased field name, and `query:"-"` is the same as no tag.

Values convert to strings, booleans, signed and unsigned integers, floats, pointers to those, and slices. Multipart fields accept `multipart.FileHeader`, `*multipart.FileHeader`, and slices of either.

## BindError

```go
type BindError struct {
	Source string // "path", "query", "header", "form", or "body"
	Field  string // the Go struct field, for logs
	Name   string // the request-facing name: the tag value, or the JSON field path
	Reason string // a client-safe description, such as "must be an integer"
	Err    error  // the underlying decode or conversion error
}
```

A `BindError` answers `400 Bad Request` through the default error handler, naming the field and reason without exposing decoder text. It unwraps to `Err`. A body over `Config.BodyLimit` produces a `BindError` that wraps `zinc.ErrRequestEntityTooLarge`, and answers 413.

## Validator

```go
type Validator interface {
	Validate(any) error
}
```

Set `Config.Validator` to run validation after every bind. `c.Validate(v)` calls it directly.

## Decoders

```go
type Decoder func(data []byte, v any) error
```

Set `Config.Decoders` to read other body formats, keyed by media type. `Body` and `All` pick one from `Content-Type`; an entry for `application/json` or `application/xml` also replaces the decoder `JSON(&v)` and `XML(&v)` use. See [Body formats](/guide/customization/#body-formats).
