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
		return err // 400 with the failing field
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

Path, query, header, and form values bind only to fields that carry the matching tag. A field without a `query` tag can't be set from the query string, even by `All`, so a field hidden from JSON with `json:"-"` stays out of reach. A tag with options but no name, such as `query:",omitempty"`, opts in under the lower-cased field name.

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
	return err
}
if err := c.Bind().JSON(&in); err != nil {
	return err
}
```

The available methods are `All`, `Path`, `Query`, `Header`, `Form`, `Body` (chosen by `Content-Type`), and the explicit body formats `JSON`, `XML`, `YAML`, `TOML`, and `Text`.

## Handle binding errors

A failed bind returns a `*zinc.BindError` that names the source and field:

```go
var be *zinc.BindError
if errors.As(err, &be) {
	// be.Source is "path", "query", "header", "form", or "body".
	// be.Name is the request name ("page"); be.Field is the Go field, for logs.
	// be.Reason is client-safe, such as "must be an integer".
}
```

Return binding errors unchanged. The default handler answers 400 and names the field, without decoder details:

```json
{"error":{"status":400,"message":"invalid query parameter","fields":{"page":"must be an integer"}}}
```

Wrapping the error in `zinc.BadRequest(...)` replaces that body with your message and drops the field. An HTTP cause, such as a body over the limit, keeps its status (413). Known JSON syntax and type errors are binding errors; invalid JSON destinations and opaque codec failures remain 500 errors.

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

Every bind method runs the validator after decoding, so handlers stay short. A validation failure comes back from the bind call as a `*zinc.ValidationError`; return it, and the default handler answers `422 Unprocessable Entity`.

To list the failing fields in the response, return an error with a `Fields() map[string]string` method. For go-playground/validator:

```go
type fieldErrors map[string]string

func (f fieldErrors) Error() string              { return "validation failed" }
func (f fieldErrors) Fields() map[string]string { return f }

func (s structValidator) Validate(target any) error {
	err := s.v.Struct(target)
	var invalid validator.ValidationErrors
	if !errors.As(err, &invalid) {
		return err
	}
	fields := fieldErrors{}
	for _, fe := range invalid {
		fields[fe.Field()] = "failed " + fe.Tag()
	}
	return fields
}
```

```json
{"error":{"status":422,"message":"validation failed","fields":{"Email":"failed required"}}}
```

A validator that returns a Zinc HTTP error, or any `StatusCoder`, keeps that status instead.

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

Path, query, header, and form scalar fields support `encoding.TextUnmarshaler`, including `time.Time` and custom IDs. Optional scalar pointers stay nil when absent and are allocated when present; explicit zero values remain distinguishable from absence. Conversion errors retain their source and field. Only tagged fields bind from path, query, header, and form. Later `All` sources can overwrite earlier values when a field is tagged for more than one source.
