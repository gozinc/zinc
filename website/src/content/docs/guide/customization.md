---
title: Customization
description: Swap Zinc's error format, validator, template engine or JSON library, and add body formats such as YAML.
---

Zinc's defaults are easy to swap: the error format, the validation library, the template engine, extra body formats such as YAML, or a different JSON package. Each one is a field on `zinc.Config`, so a change is a few lines in the place you create the app.

```go
app := zinc.New(zinc.Config{
	ErrorHandler: logServerErrors,                                            // your function
	Validator:    structValidator{v: validator.New()},                        // an adapter, see Binding
	Decoders:     map[string]zinc.Decoder{"application/yaml": yaml.Unmarshal}, // go.yaml.in/yaml/v3
	Renderer:     zinc.NewHTMLTemplateRenderer(views),                        // a *template.Template
})
```

| I want to… | Set | See |
|---|---|---|
| Change how errors look, or log them | `ErrorHandler` | [Change the error response](#change-the-error-response) |
| Validate input after every bind | `Validator` | [Validate every bind](#validate-every-bind) |
| Accept or send YAML, TOML or another format | `Decoders`, `Encoders` | [Body formats](#body-formats) |
| Use a faster JSON library | `Decoders`, `Encoders` | [Use a different JSON library](#use-a-different-json-library) |
| Render HTML templates | `Renderer` | [Render templates](#render-templates) |

## Change the error response

An error handler runs once for each request whose handler or middleware returns an error. It has this shape:

```go
type ErrorHandler func(*zinc.Context, error)
```

To add to the default behavior, do your work and then call `zinc.DefaultErrorHandler`. This one logs server errors and keeps the usual JSON response:

```go
app := zinc.New(zinc.Config{
	ErrorHandler: func(c *zinc.Context, err error) {
		if zinc.StatusCode(err) >= 500 {
			slog.Error("request failed", "path", c.Path(), "err", err)
		}
		zinc.DefaultErrorHandler(c, err)
	},
})

app.Get("/report", func(c *zinc.Context) error {
	return errors.New("database is down")
})
```

```bash
curl http://localhost:8080/report
# {"error":{"status":500,"message":"Internal Server Error"}}
```

```text
2026/09/28 10:15:02 ERROR request failed path=/report err="database is down"
```

`zinc.StatusCode(err)` returns the status the default handler would send. For plain-text error bodies, set `ErrorHandler: zinc.TextErrors`. [Errors](/guide/errors/) shows a handler that writes your own JSON shape.

## Validate every bind

A validator runs after every successful bind. It's any type with this method:

```go
type Validator interface {
	Validate(any) error
}
```

This small one calls a `Validate() error` method on the input, if the type has one:

```go
type selfValidator struct{}

func (selfValidator) Validate(v any) error {
	if s, ok := v.(interface{ Validate() error }); ok {
		return s.Validate()
	}
	return nil
}

type Signup struct {
	Email string `json:"email"`
}

func (s *Signup) Validate() error {
	if s.Email == "" {
		return errors.New("email is required")
	}
	return nil
}

app := zinc.New(zinc.Config{Validator: selfValidator{}})
```

```bash
curl -X POST http://localhost:8080/signup -H 'Content-Type: application/json' -d '{"email":""}'
# {"error":{"status":422,"message":"validation failed"}}
```

For go-playground/validator, [Binding](/guide/binding/) has a three-line adapter and shows how to name the failing fields in the response.

## Body formats

Zinc reads and writes JSON, XML, forms, multipart and plain text with the standard library. For any other format, or a different JSON library, add a decoder, an encoder or both:

```go
type Decoder func(data []byte, v any) error // the shape of json.Unmarshal
type Encoder func(v any) ([]byte, error)    // the shape of json.Marshal
```

Most libraries' `Unmarshal` and `Marshal` functions already have these shapes, so you pass them in directly.

### Read and write YAML

```go
import "go.yaml.in/yaml/v3"

type Settings struct {
	Name  string `json:"name" yaml:"name"`
	Debug bool   `json:"debug" yaml:"debug"`
}

app := zinc.New(zinc.Config{
	Decoders: map[string]zinc.Decoder{
		"application/yaml":   yaml.Unmarshal,
		"application/x-yaml": yaml.Unmarshal,
		"text/yaml":          yaml.Unmarshal,
	},
	Encoders: map[string]zinc.Encoder{"application/yaml": yaml.Marshal},
})

app.Post("/settings", func(c *zinc.Context) error {
	var s Settings
	if err := c.Bind().Body(&s); err != nil {
		return err
	}
	return c.Negotiate(map[string]any{
		"application/json": s,
		"application/yaml": s,
	})
})
```

```bash
curl -X POST http://localhost:8080/settings \
  -H 'Content-Type: application/yaml' -H 'Accept: application/yaml' \
  --data-binary $'name: api\ndebug: true\n'
# name: api
# debug: true
```

`c.Bind().Body` and `c.Bind().All` pick the decoder from the request's `Content-Type`, and so do [typed handlers](/guide/typed-handlers/). To send YAML whatever the client asks for, use `c.Encode("application/yaml", s)`.

A YAML body binds through the library's own tags, usually `yaml:"name"`, not `json:"name"`. A struct that accepts both formats needs both tags, as `Settings` has.

:::note[List every media type]
YAML travels under several media types, so the example lists each one. A `Content-Type` with no matching entry isn't decoded as YAML.
:::

### Use a different JSON library

An entry for `application/json` replaces `encoding/json` everywhere Zinc reads or writes JSON: binding, `c.JSON`, `c.JSONPretty` and server-sent events.

```go
import "github.com/bytedance/sonic"

app := zinc.New(zinc.Config{
	Decoders: map[string]zinc.Decoder{"application/json": sonic.Unmarshal},
	Encoders: map[string]zinc.Encoder{"application/json": sonic.Marshal},
})
```

An `application/xml` entry replaces `encoding/xml` the same way.

### Reject unknown JSON fields

Wrap `encoding/json` in your own decoder:

```go
func strictJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

app := zinc.New(zinc.Config{
	Decoders: map[string]zinc.Decoder{"application/json": strictJSON},
})
```

```bash
curl -X POST http://localhost:8080/settings \
  -H 'Content-Type: application/json' -d '{"name":"api","colour":"red"}'
# {"error":{"status":400,"message":"invalid request body"}}
```

## Render templates

A renderer turns a template name and data into a response for `c.Render`:

```go
type Renderer interface {
	Render(w io.Writer, name string, data any, c *zinc.Context) error
}
```

You rarely write one. Zinc's built-in renderers cover `html/template`, `text/template` and any engine with an `ExecuteTemplate` method:

```go
views := template.Must(template.ParseGlob("views/*.html"))

app := zinc.New(zinc.Config{Renderer: zinc.NewHTMLTemplateRenderer(views)})
```

[Templates](/guide/templates/) covers the rest.

## Good to know

### How media types match

`Decoders` and `Encoders` keys match the base media type, ignoring case and parameters. So `application/yaml; charset=utf-8` finds the `application/yaml` entry.

### Decoder errors are the client's

A decoder's error answers `400` with `invalid request body`. The error itself is kept for your logs, not sent to the client. If the failure is yours rather than the client's, return a Zinc error such as `zinc.InternalServerError(...)` and that status is used instead.

### Encoder output is sent as is

Zinc writes an encoder's bytes exactly as it returns them. The built-in JSON encoder ends each body with a newline; `json.Marshal` and most libraries don't. `c.JSONPretty` indents a custom encoder's output with `json.Indent`.

### An unknown format is a server error

`c.Encode` with a media type that has no encoder returns an error, and the client gets a `500`. JSON and XML always work: without an entry, Zinc uses the standard library for them.

### Mistakes stop the app at startup

`zinc.New` panics on a key that isn't a media type (such as `"yaml"`), on a `nil` decoder or encoder, and on a decoder for `application/x-www-form-urlencoded` or `multipart/form-data`. Forms are bound field by field, so they can't be replaced.

## Next steps

- [Configuration](/guide/configuration/): every setting and its default.
- [Errors](/guide/errors/): the default error response and how to shape your own.
- [Zinc and net/http](/guide/http-interoperability/): run the app on your own `http.Server`, with your own TLS and listeners.
