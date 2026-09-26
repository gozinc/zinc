---
title: Decompress
description: Decompress gzip request bodies before handlers read them.
---

`decompress` accepts request bodies sent with `Content-Encoding: gzip` and decompresses them before handlers and binding read them.

```go
import "github.com/0mjs/zinc/middleware/decompress"

app.Use(decompress.New())
```

Handlers then read the body normally:

```go
app.Post("/ingest", func(c *zinc.Context) error {
	var input Event
	if err := c.Bind().JSON(&input); err != nil {
		return err // 400 with the failing field
	}
	return c.NoContent()
})
```

Unsupported content encodings return `415 Unsupported Media Type`. Invalid gzip bodies return `400 Bad Request`.

A small compressed body can expand enormously, so the decompressed size is capped. By default the cap is the app's `Config.BodyLimit`, or 4 MiB when the app has none. Set `MaxDecompressedSize` to choose another:

```go
app.Use(decompress.New(decompress.Config{MaxDecompressedSize: 16 << 20}))
```

Requests that expand past the limit return `413 Payload Too Large`.
