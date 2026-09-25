---
title: Customization
description: Replace Zinc's error handler, validator, renderer, JSON codec, or request binder with your own.
---

Each part of Zinc that makes a policy decision can be replaced through configuration: how errors are written, how input is validated, how JSON is encoded, how templates render. Each extension point is a small interface, so a replacement is usually a few lines.

```go
app := zinc.New(zinc.Config{
	ErrorHandler: writeJSONError,
	Validator:    structValidator{v: validator.New()},
	JSONCodec:    sonicCodec{},
	Renderer:     zinc.NewHTMLTemplateRenderer(views),
})
```

## Error handler

```go
type ErrorHandler func(c *zinc.Context, err error)
```

Called once for every error that reaches the top of the chain. [Errors](/guide/errors/#a-custom-error-handler) has a complete JSON example, including mapping binding errors to `400`.

## Validator

```go
type Validator interface {
	Validate(any) error
}
```

Runs after every successful bind. An adapter for go-playground/validator is three lines, shown in [Binding](/guide/binding/#validation).

## JSON codec

```go
type JSONCodec interface {
	Encode(w io.Writer, v any, indent string) error
	Decode(r io.Reader, v any) error
}
```

Used by `c.JSON`, `c.JSONPretty`, and JSON binding. Swap in a faster library, or tune `encoding/json`:

```go
type strictJSON struct{}

func (strictJSON) Encode(w io.Writer, v any, indent string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", indent)
	return enc.Encode(v)
}

func (strictJSON) Decode(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields() // reject fields the struct does not declare
	return dec.Decode(v)
}
```

## Renderer

```go
type Renderer interface {
	Render(w io.Writer, name string, data any, c *zinc.Context) error
}
```

Zinc's template renderers cover `html/template`, `text/template`, and any engine with an `ExecuteTemplate` method. See [Templates](/guide/templates/).

## Request binder

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

Replacing the binder changes how every `c.Bind()` method decodes. Most apps never need this; a custom `JSONCodec` or `Validator` usually covers the requirement.

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
