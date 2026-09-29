---
title: Binding
description: Read path parameters, query strings, headers and request bodies into a typed Go struct in one call, then validate it.
---

Binding reads request data into a Go struct in one call. Use it when a handler needs more than a value or two, so you don't parse and convert each one by hand.

```go
type ListOrders struct {
	Customer int    `path:"customer"`
	Status   string `query:"status"`
	Page     int    `query:"page"`
}

app.Get("/customers/{customer}/orders", func(c *zinc.Context) error {
	var in ListOrders
	if err := c.Bind().All(&in); err != nil {
		return err
	}
	return c.JSON(in)
})
```

```bash
curl "http://localhost:8080/customers/7/orders?status=open&page=2"
# {"Customer":7,"Status":"open","Page":2}

curl "http://localhost:8080/customers/7/orders?page=two"
# {"error":{"status":400,"message":"invalid query parameter","fields":{"page":"must be an integer"}}}
```

Struct tags say where each field comes from, and Zinc converts the text to the field's type. When a value doesn't fit, return the error: the client gets a `400` that names the field.

## Where values come from

| Tag | Reads from | Bind method |
|---|---|---|
| `path:"id"` | Route parameters | `Path` |
| `query:"page"` | Query string | `Query` |
| `header:"X-Tenant"` | Request headers | `Header` |
| `cookie:"session"` | Cookies | `Cookie` |
| `form:"name"` | URL-encoded or multipart form | `Form` |
| `json`, `xml` | Request body | `JSON`, `XML`, or `Body` for [other formats](/guide/customization/#body-formats) |

A field is set from the path, query, headers, cookies or a form only if it has that tag. A field with no `query` tag can't be set from the query string, even by `All`.

:::tip[Bind into an input struct]
Declare a struct for each handler's input rather than binding into your database model. Then a client can only set the fields you list, and not, say, an `IsAdmin` column.
:::

## Bind everything at once

`c.Bind().All(&in)` is the usual choice for API handlers. It reads, in order:

1. the body, choosing JSON, XML, text, form or a [configured decoder](/guide/customization/#body-formats) from `Content-Type`,
2. query values,
3. route parameters,

and then runs your [validator](#validation), if you've set one.

```go
type CreateOrder struct {
	Customer int      `path:"customer"`
	DryRun   bool     `query:"dry_run"`
	Items    []string `json:"items"`
	Note     string   `json:"note"`
}

app.Post("/customers/{customer}/orders", func(c *zinc.Context) error {
	var in CreateOrder
	if err := c.Bind().All(&in); err != nil {
		return err
	}
	return c.Status(zinc.StatusCreated).JSON(zinc.Map{
		"customer": in.Customer,
		"dry_run":  in.DryRun,
		"items":    in.Items,
	})
})
```

```bash
curl -X POST "http://localhost:8080/customers/7/orders?dry_run=true" \
  -H "Content-Type: application/json" \
  -d '{"items":["tea","milk"]}'
# {"customer":7,"dry_run":true,"items":["tea","milk"]}
```

The URL is read last, so it always wins. Go's JSON decoder matches `Customer` to a `"customer"` key whatever the case, but a body of `{"customer":99,"dry_run":false}` still binds customer `7` and `dry_run=true` from the URL.

`All` doesn't read headers. Bind them with `c.Bind().Header(&in)`:

```go
var in struct {
	Tenant string `header:"X-Tenant"`
}
if err := c.Bind().Header(&in); err != nil {
	return err
}
// curl -H "X-Tenant: acme" ... → in.Tenant == "acme"
```

## Bind one source

When a handler should accept input from one place only, name it:

```go
var in CreateOrder
if err := c.Bind().JSON(&in); err != nil {
	return err
}
```

The methods are `All`, `Path`, `Query`, `Header`, `Cookie`, `Form`, `Body` (chosen by `Content-Type`), and the explicit body formats `JSON`, `XML` and `Text`. For YAML, TOML or any other format, configure a [decoder](/guide/customization/#body-formats) and use `Body` or `All`.

:::caution[Each call runs the validator]
`Path`, `Query`, `JSON` and the other single-source methods each validate straight away. If you call `Path` and then `JSON` on the same struct, the first call validates before the body is read, so a `required` body field fails:

```text
PUT /users/5  {"name":"Ada"}
→ 422 {"error":{"status":422,"message":"validation failed","fields":{"Name":"failed required"}}}
```

Use `All` for input that spans sources. It validates once, after everything is read.
:::

## Default values

A `default` tag gives a query, header, cookie or form field the value to use when the request leaves it out:

```go
type ListPets struct {
	Limit int      `query:"limit" default:"20"`
	Sort  string   `query:"sort" default:"name"`
	Kinds []string `query:"kind" default:"cat,dog"`
}
```

A request to `/pets` gets `Limit` 20, `Sort` "name" and `Kinds` `[cat dog]`; `/pets?limit=5&kind=bird` gets 5, "name" and `[bird]`. A slice's default separates its values with commas.

The default is checked against the field's type when the struct is first used: at registration for a [typed handler](/guide/typed-handlers/), so `default:"lots"` on an `int` panics at startup. [OpenAPI](/guide/openapi/) specs list each default.

## Tell a missing value from zero

Make a field a pointer when "not sent" and "sent as zero" mean different things. It stays `nil` when the value is absent:

```go
type ListUsers struct {
	Limit *int   `query:"limit"`
	Sort  string `query:"sort"`
}

app.Get("/users", func(c *zinc.Context) error {
	var in ListUsers
	if err := c.Bind().Query(&in); err != nil {
		return err
	}
	limit := 20
	if in.Limit != nil {
		limit = *in.Limit
	}
	return c.JSON(zinc.Map{"limit": limit})
})
```

```bash
curl http://localhost:8080/users           # {"limit":20}
curl "http://localhost:8080/users?limit=0" # {"limit":0}
```

Path, query, header and form fields also accept any type that implements `encoding.TextUnmarshaler`, such as `time.Time` or your own ID type. `?since=2026-09-01T00:00:00Z` fills a `time.Time` field tagged `query:"since"`.

## Accept file uploads

Multipart file fields bind like any other form field:

```go
type UploadInput struct {
	Title  string                  `form:"title"`
	Avatar *multipart.FileHeader   `form:"avatar"`
	Files  []*multipart.FileHeader `form:"files"`
}
```

```bash
curl -F title=Profile -F avatar=@me.png -F files=@a.txt -F files=@b.txt \
  http://localhost:8080/upload
# in.Title == "Profile", in.Avatar.Filename == "me.png", len(in.Files) == 2
```

Both `multipart.FileHeader` and `*multipart.FileHeader` work, as single values or slices. [Request Data](/guide/request/) shows how to open and save the files.

## Handle binding errors

A failed bind returns a `*zinc.BindError`. Return it unchanged and the default error handler answers `400`, naming the field but not the decoder's internal message:

```bash
curl -X POST http://localhost:8080/customers/7/orders \
  -H "Content-Type: application/json" -d '{"items":"tea"}'
# {"error":{"status":400,"message":"invalid request body","fields":{"items":"must be an array"}}}
```

To inspect the error yourself, for logging or a custom format, use `errors.As`:

```go
var be *zinc.BindError
if errors.As(err, &be) {
	// be.Source: "path", "query", "header", "form" or "body"
	// be.Name:   the request name, such as "page"
	// be.Field:  the Go field name, for logs
	// be.Reason: safe to show a client, such as "must be an integer"
}
```

What the client gets depends on the error:

- **A value that doesn't convert**, such as `page=two`: `400`, with the field in `fields`.
- **A malformed body**, including an error from a [configured decoder](/guide/customization/#body-formats) or a type's own `UnmarshalJSON`: `400` `invalid request body`. If that error carries its own HTTP status, Zinc uses that status.
- **A body over the [size limit](#limit-the-body-size)**: `413`, as long as you return the error unchanged.
- **An unsupported `Content-Type`**: `400` `invalid request body`.
- **A bad destination**, such as a non-pointer: `500`. That's a bug in your code, not the client's.

If you wrap the error as `zinc.BadRequest("send a JSON order")`, the client gets your message and status instead. The `fields` go, and so does a `413`: an oversized body then answers `400` too.

## Validation

Zinc doesn't include a validator. Plug in any library by implementing one method:

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

app := zinc.New(zinc.Config{Validator: structValidator{v: validator.New()}})
```

```go
type SignUp struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
}
```

Every bind method runs the validator after reading, so handlers stay short. A failure comes back from the bind call as a `*zinc.ValidationError`. Return it and the client gets a `422`:

```json
{"error":{"status":422,"message":"validation failed"}}
```

### List the failing fields

To name the fields in the response, return an error with a `Fields() map[string]string` method. For go-playground/validator:

```go
type fieldErrors map[string]string

func (f fieldErrors) Error() string             { return "validation failed" }
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

```bash
curl -X POST http://localhost:8080/signup \
  -H "Content-Type: application/json" -d '{"password":"correct-horse-battery"}'
# {"error":{"status":422,"message":"validation failed","fields":{"Email":"failed required"}}}
```

The keys are whatever your adapter returns. `fe.Field()` gives the Go field name (`Email`); register a tag-name function with the validator if you want the JSON name instead.

:::note[Choose a different status]
If your validator returns a Zinc HTTP error, such as `zinc.NewError(409, "email already registered")`, or any error with a `StatusCode() int` method, the client gets that status instead of `422`.
:::

## Limit the body size

Binding reads at most `Config.BodyLimit` bytes: 4 MiB (`4 << 20`) by default. A larger body answers `413`:

```bash
# {"error":{"status":413,"message":"Request Entity Too Large"}}
```

Change the limit for the whole app in [configuration](/guide/configuration/), or for some routes with the [Body Limit](/middleware/bodylimit/) middleware.

## Good to know

### Later sources overwrite earlier ones

`All` always reads the body, then query, then path, whatever the body format. If a field is tagged for more than one source, the value read last wins: with `path:"id" query:"id"` on a `/orders/{id}` route, a request to `/orders/7?id=9` gets `7`.

### Maps, strings and plain text

`All` needs a pointer to a struct for JSON, XML and form bodies. A `map` target returns an error, which answers `500`; use `c.Bind().Body(&m)` or `JSON` to decode into a map. Two cases read the body only, skipping path and query: a `text/plain` body, and a [configured decoder](/guide/customization/#body-formats) with a non-struct target. A `text/plain` body binds into a `string`, a `[]byte`, a scalar or a `TextUnmarshaler`.

### A tag with no name

A tag with options but no name, such as `query:",omitempty"`, uses the field name in lower case: a `Sort` field reads `?sort=`.

### Errors keep their source

A conversion error remembers where it came from, so `be.Source` and `be.Name` tell you whether `id` failed in the path or the query.

## Next steps

- [Errors](/guide/errors/): turn binding and validation failures into consistent responses.
- [Request Data](/guide/request/): read single values without a struct.
- [Typed Handlers](/guide/typed-handlers/): let Zinc bind, validate and send the response for you.
- [Customization](/guide/customization/#body-formats): add YAML, TOML or a different JSON library.
