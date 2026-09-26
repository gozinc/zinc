---
title: Compress
description: Compress responses for clients that accept gzip.
---

`compress` compresses responses for clients that send `Accept-Encoding: gzip`, which cuts transfer size for JSON and HTML considerably. Skip it when a CDN or reverse proxy in front of the app already compresses responses.

```go
import "github.com/0mjs/zinc/middleware/compress"

app.Use(compress.New())
```

Zinc skips compression when:

- the client does not accept gzip
- the response status cannot include a body
- the response already has a `Content-Encoding`
- the request method is `HEAD`

It adds `Vary: Accept-Encoding` when it compresses a response, and keeps the `Flusher` and `Hijacker` interfaces that streaming and WebSockets rely on.

## Config

| Field | Default | Meaning |
|---|---|---|
| `Level` | default compression | A `compress/gzip` level, from `HuffmanOnly` (-2) to `BestCompression` (9) |
| `MinLength` | `0` | Responses shorter than this are sent uncompressed |

Use the standard library's `compress/gzip` constants for the level:

```go
import (
	"compress/gzip"

	"github.com/0mjs/zinc/middleware/compress"
)

app.Use(compress.New(compress.Config{Level: gzip.BestSpeed, MinLength: 1024}))
```

Gzip is the only encoding today; the package is named `compress` so that others, such as Brotli or Zstandard, can join it without a new package.
