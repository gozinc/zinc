---
title: Binding
description: Decode path parameters, query strings, headers, and request bodies into typed Go structs, then validate them.
---

Binding turns request data into a typed struct in one call. Struct tags say where each field comes from, and Zinc converts strings to the field types for you.

```go
type ListOrders struct {
	Customer int    `path:"customer"`
	Status   string `query:"status"`
	Page     int    `query:"page"`
}

app.Get("/customers/{customer}/orders", func(c *zinc.Context) error {
	var in ListOrders
	if err := c.Bind().All(&in); err != nil {
		return zinc.ErrBadRequest.WithMessage("invalid request").WithCause(err)
	}
	return c.JSON(in)
})
```

`GET /customers/7/orders?status=open&page=2` fills `Customer: 7`, `Status: "open"`, and `Page: 2`.

## Where values come from

| Tag | Source | Bind method |
|---|---|---|
| `path:"id"` | Route parameters | `Path` |
| `query:"page"` | Query string | `Query` |
| `header:"x-tenant"` | Request headers | `Header` |
| `form:"name"` | URL-encoded or multipart form | `Form` |
| `json`, `xml`, `yaml`, `toml` | Request body | `JSON`, `XML`, `YAML`, `TOML` |

## Bind everything at once

`c.Bind().All(&in)` is the usual choice for API handlers. It binds, in order:

1. route parameters,
2. query values,
3. the body, choosing JSON, XML, YAML, TOML, text, or form decoding from `Content-Type`,

and then runs your [validator](#validation), if one is configured.

```go
type CreateOrder struct {
	Customer int      `path:"customer"`
	DryRun   bool     `query:"dry_run"`
	Items    []string `json:"items"`
	Note     string   `json:"note"`
}
```

:::note[Headers are separate]
`All` does not read headers. Bind them explicitly with `c.Bind().Header(&in)`.
:::

## Bind one source

When a handler should accept input from exactly one place, name it:

```go
var in CreateOrder
if err := c.Bind().Path(&in); err != nil {
	return zinc.ErrBadRequest.WithCause(err)
}
if err := c.Bind().JSON(&in); err != nil {
	return zinc.ErrBadRequest.WithCause(err)
}
```

The available methods are `All`, `Path`, `Query`, `Header`, `Form`, `Body` (chosen by `Content-Type`), and the explicit body formats `JSON`, `XML`, `YAML`, `TOML`, and `Text`.

## Handle binding errors

A failed bind returns a `*zinc.BindError` that names the source and field:

```go
var be *zinc.BindError
if errors.As(err, &be) {
	// be.Source is "path", "query", "header", "form", or "body"; be.Field is the struct field.
}
```

:::note[Binding error responses]
The default handler returns a generic 400 for `BindError`, preserving HTTP causes such as a 413 body limit. Known JSON syntax/type errors are classified as binding errors. Invalid JSON destinations and opaque codec failures remain 500 errors. Validation errors follow your validator's error policy; wrap them in an HTTP error or handle their type centrally.
:::

## Validation

Zinc does not ship a validator. Plug in any library by implementing one method:

```go
type Validator interface {
	Validate(any) error
}
```

For example, with [go-playground/validator](https://github.com/go-playground/validator):

```go
type structValidator struct{ v *validator.Validate }

func (s structValidator) Validate(target any) error {
	return s.v.Struct(target)
}

cfg := zinc.DefaultConfig
cfg.Validator = structValidator{v: validator.New()}
app := zinc.NewWithConfig(cfg)
```

```go
type SignUp struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
}
```

Every bind method runs the validator after decoding, so handlers stay short. Validation errors come back from the bind call; wrap or map them the same way as binding errors, often as `422 Unprocessable Entity`.

## File uploads

Multipart file fields bind directly:

```go
type UploadInput struct {
	Title  string                  `form:"title"`
	Avatar *multipart.FileHeader   `form:"avatar"`
	Files  []*multipart.FileHeader `form:"files"`
}
```

Both `multipart.FileHeader` and `*multipart.FileHeader` work, as single values or slices.

## Body size limit

Binding reads at most `Config.BodyLimit` bytes, 4 MB by default. Larger bodies return `413 Request Entity Too Large`. Raise or lower the limit in [configuration](/guide/configuration/), or per route with the [Body Limit](/middleware/body-limit/) middleware.

## Next steps

- [Errors](/guide/errors/) turns binding and validation failures into consistent responses.
- [Request Data](/guide/request/) reads single values without a struct.
- [Binding API](/api/binding/) documents `RequestBinder` for replacing the decoder.

Struct targets passed to `All` consistently merge path, query, then body for JSON, XML, YAML, and TOML. Scalar/map YAML and TOML targets and plain text remain body-only. Validation runs once after the combined bind. Source-specific operations each validate immediately; use `All` for the supported combined phase or a custom `RequestBinder` when composing other sources.

Path, query, header, and form scalar fields support `encoding.TextUnmarshaler`, including `time.Time` and custom IDs. Optional scalar pointers stay nil when absent and are allocated when present; explicit zero values remain distinguishable from absence. Conversion errors retain their source and field. Use request DTOs: exported untagged fields still participate, and later `All` sources can overwrite earlier values.
