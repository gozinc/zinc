---
title: Content Type
description: Reject request bodies in media types or encodings an endpoint doesn't accept.
---

Content Type turns away request bodies your endpoint can't read, with `415 Unsupported Media Type`, before a handler or binder sees them. Add it to JSON-only endpoints so a form post or an XML body fails with a clear status instead of a confusing binding error.

## Usage

```go
import "github.com/0mjs/zinc/middleware/contenttype"

app.Post("/events", contenttype.New(contenttype.Config{
	Types: []string{"application/json"},
}), createEvent) // createEvent: your handler
```

```bash
curl -i http://localhost:8080/events \
  -H "Content-Type: application/json" -d '{"name":"signup"}'
# HTTP/1.1 202 Accepted

curl -i http://localhost:8080/events \
  -H "Content-Type: text/plain" -d 'signup'
# HTTP/1.1 415 Unsupported Media Type
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":415,"message":"Unsupported Media Type"}}
```

Attach it to the routes that take a body. It checks every request it runs on, including a `GET` with no `Content-Type` (see [Errors](#errors)).

## Defaults

Nothing is checked by default. An empty list allows anything, so you can check types without encodings, or the other way round.

| Setting | Default |
|---|---|
| Media types | any |
| Content codings | any |

## Configuration

Accept JSON, sent either plain or gzip-compressed:

```go
app.Post("/events", contenttype.New(contenttype.Config{
	Types:     []string{"application/json"},
	Encodings: []string{"identity", "gzip"},
}), createEvent)
```

```bash
curl -i http://localhost:8080/events \
  -H "Content-Type: application/json" -H "Content-Encoding: br" -d '{}'
# HTTP/1.1 415 Unsupported Media Type
```

| Field | Default | Meaning |
|---|---|---|
| `Types` | any | Accepted media types. Parameters and case are ignored, so `application/json; charset=utf-8` matches `application/json`. |
| `Encodings` | any | Accepted content codings, such as `gzip`. A request without `Content-Encoding` counts as `identity`, so list `identity` to allow plain bodies. |

`contenttype` only checks the header. To accept gzip bodies, add [Decompress](/middleware/decompress/) as well.

## Errors

A request that fails either check gets `415 Unsupported Media Type` (`zinc.ErrUnsupportedMediaType`), and the handler doesn't run.

:::caution[Requests without a body]
With `Types` set, a request with no `Content-Type` header fails too. That includes a `GET`, so attach `contenttype` to the routes that take a body, not to a whole group of mixed routes.
:::

## Related

- [Decompress](/middleware/decompress/): accepts gzip request bodies.
- [Body Limit](/middleware/bodylimit/): caps the size of request bodies.
- [Binding](/guide/binding/): reads the body into a struct once it's accepted.
