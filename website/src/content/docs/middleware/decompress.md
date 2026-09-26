---
title: Decompress
description: Decompress gzip request bodies before handlers read them.
---

`Decompress` accepts request bodies sent with `Content-Encoding: gzip` and decompresses them before handlers and binding read them.

```go
app.Use(middleware.Decompress())
```

Handlers can then read the body normally.

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

A small compressed body can expand enormously. Set `MaxDecompressedSize` on any public endpoint to cap the decompressed size:

```go
app.Use(middleware.DecompressWithConfig(middleware.DecompressConfig{
	MaxDecompressedSize: 4 << 20,
}))
```

Requests that expand past the limit return `413 Payload Too Large`.
