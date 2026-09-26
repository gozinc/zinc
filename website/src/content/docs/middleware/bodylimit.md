---
title: Body Limit
description: Reject oversized request bodies before handlers consume them.
---

`bodylimit` rejects request bodies over a size you choose with `413 Request Entity Too Large`, before handlers read them. Use it to set a tighter limit on particular routes than the app-wide `Config.BodyLimit`, which binding already enforces.

```go
import "github.com/0mjs/zinc/middleware/bodylimit"

app.Post("/avatars", bodylimit.New(bodylimit.Config{Limit: 2 * bodylimit.MB}), uploadAvatar)
```

`Limit` is required. The package has size constants `bodylimit.B`, `KB`, `MB`, and `GB`.

## Failure behavior

A request whose `Content-Length` is over the limit fails before its body is read. A chunked or dishonest request fails as soon as reading passes the limit. Either way the error is a `*bodylimit.Error` with:

- `Limit`, the configured maximum
- `Observed`, the size seen
- `Source`, either `bodylimit.SourceContentLength` or `bodylimit.SourceBodyRead`

The error unwraps to `zinc.ErrRequestEntityTooLarge`, so the default error handler answers `413`.
