---
title: Binding
description: Reference for c.Bind(), struct tags, BindError, Validator, and RequestBinder.
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
| `JSON(&v)`, `XML(&v)`, `YAML(&v)`, `TOML(&v)`, `Text(&v)` | The body in one format, regardless of `Content-Type`. An empty body is an error. |

## Struct tags

| Tag | Example |
|---|---|
| `path` | `` ID int `path:"id"` `` |
| `query` | `` Page int `query:"page"` `` |
| `header` | `` Tenant string `header:"X-Tenant"` `` |
| `form` | `` Avatar *multipart.FileHeader `form:"avatar"` `` |
| `json`, `xml`, `yaml`, `toml` | Standard encoding tags for the body |

A field binds from path, query, header, or form only when it has that source's tag. `query:",omitempty"` opts in under the lower-cased field name, and `query:"-"` is the same as no tag.

Values convert to strings, booleans, signed and unsigned integers, floats, pointers to those, and slices. Multipart fields accept `multipart.FileHeader`, `*multipart.FileHeader`, and slices of either.

## BindError

```go
type BindError struct {
	Source string // "path", "query", "header", "form", or "body"
	Field  string // the struct field, when known
	Err    error  // the underlying decode or conversion error
}
```

`BindError` unwraps to `Err`. It is not an HTTP error, so return it as `zinc.ErrBadRequest.Wrap(err)` or map it in the [error handler](/guide/errors/#map-binding-errors-to-400). A body over `Config.BodyLimit` produces a `BindError` that wraps `zinc.ErrRequestEntityTooLarge`.

## Validator

```go
type Validator interface {
	Validate(any) error
}
```

Set `Config.Validator` to run validation after every bind. `c.Validate(v)` calls it directly.

## RequestBinder

```go
type RequestBinder interface {
	Bind(*zinc.Context, any) error
	BindBody(*zinc.Context, any) error
	BindQuery(*zinc.Context, any) error
	BindForm(*zinc.Context, any) error
	BindHeader(*zinc.Context, any) error
	BindPath(*zinc.Context, any) error
}
```

Set `Config.RequestBinder` to replace decoding entirely. The format-specific methods (`JSON`, `XML`, and so on) use the configured `JSONCodec` or the standard decoders directly.
