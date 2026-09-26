---
title: Customization
description: Replace Zinc's error handler, validator, or renderer, and add body formats such as YAML or a faster JSON library.
---

Each part of Zinc that makes a policy decision can be replaced through configuration: how errors are written, how input is validated, which body formats the app reads and writes, how templates render. Each extension point is a function or a small interface, so a replacement is usually a few lines.

```go
app := zinc.New(zinc.Config{
	ErrorHandler: writeJSONError,
	Validator:    structValidator{v: validator.New()},
	Decoders:     map[string]zinc.Decoder{"application/yaml": yaml.Unmarshal},
	Renderer:     zinc.NewHTMLTemplateRenderer(views),
})
```

## Error handler

```go
type ErrorHandler func(*zinc.Context, error)
```

Called once for every error that reaches the top of the chain. [Errors](/guide/errors/#a-custom-error-handler) has a complete JSON example, including mapping binding errors to `400`.

## Validator

```go
type Validator interface {
	Validate(any) error
}
```

Runs after every successful bind. An adapter for go-playground/validator is three lines, shown in [Binding](/guide/binding/#validation).

## Body formats

Zinc reads and writes JSON, XML, forms, multipart, and plain text itself, using only the standard library. Any other format is two map entries away, and so is a different JSON library:

```go
type Decoder func(data []byte, v any) error // the shape of json.Unmarshal
type Encoder func(v any) ([]byte, error)    // the shape of json.Marshal
```

Because those are the shapes of every library's own `Unmarshal` and `Marshal`, you pass them in directly, with no adapter.

### YAML, TOML, and others

```go
import "go.yaml.in/yaml/v3"

app := zinc.New(zinc.Config{
	Decoders: map[string]zinc.Decoder{
		"application/yaml":   yaml.Unmarshal,
		"application/x-yaml": yaml.Unmarshal,
		"text/yaml":          yaml.Unmarshal,
	},
	Encoders: map[string]zinc.Encoder{"application/yaml": yaml.Marshal},
})
```

Then `c.Bind().Body` and `c.Bind().All` pick the decoder from the request's `Content-Type`, and so do [typed handlers](/guide/typed-handlers/). Write a response with `c.Encode`, or offer it through `c.Negotiate`:

```go
return c.Encode("application/yaml", config)

return c.Negotiate(map[string]any{
	"application/json": config,
	"application/yaml": config,
})
```

Keys match on the base media type, case-insensitively and without parameters, so `application/yaml; charset=utf-8` finds the `application/yaml` entry. Formats can travel under several media types, as YAML does, so list each one you accept.

A YAML body binds through the library's own tags, usually `yaml:"name"`, not `json:"name"`. A struct that accepts both formats needs both tags.

An error from a decoder answers `400 Bad Request` with the message "invalid request body"; the error itself is kept for logs, not sent to the client. Return a Zinc error, such as `zinc.InternalServerError(...)`, when the failure is yours rather than the client's.

### A different JSON library

An entry for `application/json` replaces `encoding/json` everywhere Zinc reads or writes JSON: binding, `c.JSON`, `c.JSONPretty`, and server-sent events.

```go
import "github.com/bytedance/sonic"

app := zinc.New(zinc.Config{
	Decoders: map[string]zinc.Decoder{"application/json": sonic.Unmarshal},
	Encoders: map[string]zinc.Encoder{"application/json": sonic.Marshal},
})
```

The same works for `application/xml`. Zinc writes an encoder's bytes exactly as returned: the built-in JSON encoder ends each body with a newline, and a custom one may not. `c.JSONPretty` indents a custom encoder's output with `json.Indent`.

Without an `application/json` entry, Zinc uses its own JSON path, and an app with no `Decoders` or `Encoders` never consults either map.

To reject unknown fields, wrap `encoding/json` yourself:

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

## Renderer

```go
type Renderer interface {
	Render(w io.Writer, name string, data any, c *zinc.Context) error
}
```

Zinc's template renderers cover `html/template`, `text/template`, and any engine with an `ExecuteTemplate` method. See [Templates](/guide/templates/).

## Owning the server

For listeners, TLS, and lifecycle, you do not need configuration at all. The app is an `http.Handler`:

```go
server := &http.Server{Addr: ":8443", Handler: app, TLSConfig: tlsConfig}
log.Fatal(server.ListenAndServeTLS("", ""))
```

`app.Serve(listener)` runs the app on a listener you created, such as a systemd socket or a Unix socket.

## Next steps

- [Configuration](/guide/configuration/) for every setting and its default.
- [Zinc and net/http](/guide/http-interoperability/) for standard middleware and handlers.
