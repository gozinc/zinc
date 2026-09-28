---
title: Body Limit
description: Reject request bodies over a size you choose with 413, before your handler reads them.
---

Body Limit rejects request bodies larger than a size you choose, with `413 Request Entity Too Large`. Add it to a route or group that needs a tighter limit than the rest of the app, such as an avatar upload.

## Usage

```go
import "github.com/0mjs/zinc/middleware/bodylimit"

app.Post("/avatars", bodylimit.New(bodylimit.Config{Limit: 2 * bodylimit.MB}), uploadAvatar) // uploadAvatar: your handler
```

```bash
curl -i -X POST --data-binary @big.png http://localhost:8080/avatars   # a 3 MB file
# HTTP/1.1 413 Request Entity Too Large
# {"error":{"status":413,"message":"Request Entity Too Large"}}
```

A request whose `Content-Length` is over the limit fails before your handler runs. A request without a length, such as a chunked upload, fails as soon as reading passes the limit.

The app-wide [`Config.BodyLimit`](/guide/configuration/) (4 MiB by default) already applies to binding, forms and `c.BodyBytes`. Use this middleware for a smaller limit on some routes, or to cover handlers that read `c.Request().Body` directly.

## Defaults

| Setting | Default |
|---|---|
| Limit | none: **required** |

## Configuration

```go
api := app.Group("/api", bodylimit.New(bodylimit.Config{Limit: 64 * bodylimit.KB}))
```

| Field | Default | Meaning |
|---|---|---|
| `Limit` | **required** | Largest body allowed, in bytes. Zero or negative panics when the middleware is created. |

The package has size constants for the limit: `bodylimit.B`, `KB`, `MB` and `GB`. They are powers of 1024, so `bodylimit.MB` is 1,048,576 bytes.

## Errors

A body over the limit fails with a `*bodylimit.Error`:

| Field | Meaning |
|---|---|
| `Limit` | The configured limit |
| `Observed` | The `Content-Length` sent, or the bytes read before reading stopped (the limit plus one) |
| `Source` | `bodylimit.SourceContentLength` or `bodylimit.SourceBodyRead` |

It matches `zinc.ErrRequestEntityTooLarge` and `bodylimit.ErrExceeded` with `errors.Is`, so the default error handler answers `413`. When the handler reads the body itself, the error comes back from the read: return it and the client still gets the `413`.

:::note[It can't raise the app limit]
This middleware only makes the limit smaller. Binding and `c.BodyBytes` still stop at `Config.BodyLimit`, so a route that accepts bodies over 4 MiB needs a larger `Config.BodyLimit` as well.
:::

## Related

- [Decompress](/middleware/decompress/) inflates gzip request bodies, with its own cap on the decompressed size.
- [Configuration](/guide/configuration/) covers `Config.BodyLimit`, the app-wide limit.
- [File uploads](/cookbook/file-upload/) shows a complete upload handler.
