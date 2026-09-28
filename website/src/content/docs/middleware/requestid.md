---
title: Request ID
description: Give every request an ID, or keep the one a client or proxy sent, and return it in X-Request-ID.
---

Request ID gives every request an identifier and sends it back in the `X-Request-ID` header. Add it when you want to match a client's bug report, a log line and an upstream call to the same request.

## Usage

```go
import "github.com/0mjs/zinc/middleware/requestid"

app.Use(requestid.New())
```

```bash
curl -i http://localhost:8080/events
# HTTP/1.1 200 OK
# X-Request-Id: b8e670b192233614cb0a62945d47cf4a

curl -i -H 'X-Request-ID: abc-123' http://localhost:8080/events
# HTTP/1.1 200 OK
# X-Request-Id: abc-123
```

When the request already carries an ID, Zinc keeps it. Otherwise it generates a random one and also sets it on the request header, so later middleware and your handlers see the same value.

Register it first, before [`logger`](/middleware/logger/), so every log line and error carries the ID.

## Defaults

| Setting | Default |
|---|---|
| Header | `X-Request-ID` |
| Generated ID | 32 hex characters (16 random bytes from `crypto/rand`) |

## Configuration

This uses a different header name, and a fixed ID so test output stays the same between runs:

```go
app.Use(requestid.New(requestid.Config{
	Header:   "X-Correlation-ID",
	Generate: requestid.Static("local-dev"),
}))
```

```bash
curl -i http://localhost:8080/events
# HTTP/1.1 200 OK
# X-Correlation-Id: local-dev
```

| Field | Default | Meaning |
|---|---|---|
| `Header` | `X-Request-ID` | Header that carries the ID on the request and the response |
| `Generate` | `requestid.Random` | `func(*zinc.Context) (string, error)` that creates an ID when the request has none |

The package provides two generators: `requestid.Random`, the default, and `requestid.Static(id)`, which always returns `id`.

## Reading state

Read the ID in a handler with `requestid.Get`:

```go
app.Get("/events", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"request_id": requestid.Get(c)})
})
```

```bash
curl -H 'X-Request-ID: abc-123' http://localhost:8080/events
# {"request_id":"abc-123"}
```

`Get` returns the ID the middleware chose, whatever header you configured. When the middleware didn't run, it falls back to the request's `X-Request-ID` header, and returns `""` if there is none.

## Errors

If `Generate` returns an error, the middleware returns it and the request fails. With the default error handler, the client gets `500 Internal Server Error`. `requestid.Random` only fails if the system's random source does.

## Security

An ID sent by the client is used as-is. Treat it as untrusted input: don't use it for authorization, and escape it where you would escape any other header value.

## Related

- [Request Logger](/middleware/logger/) writes the ID into each log line as `request_id`.
- [Recover](/middleware/recover/) keeps panics from dropping the request, so the ID reaches the client.
- [Middleware](/guide/groups-and-middleware/) covers where app-wide middleware runs.
