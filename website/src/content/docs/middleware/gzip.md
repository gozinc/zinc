---
title: Gzip
description: Compress responses for clients that accept gzip.
---

`Gzip` compresses responses for clients that send `Accept-Encoding: gzip`, which cuts transfer size for JSON and HTML considerably. Skip it when a CDN or reverse proxy in front of the app already compresses responses.

```go
app.Use(middleware.Gzip())
```

Zinc skips compression when:

- the client does not accept gzip
- the response status cannot include a body
- the response already has a `Content-Encoding`
- the request method is `HEAD`

Use `GzipWithConfig` to set the compression level or skip small responses.

```go
app.Use(middleware.GzipWithConfig(middleware.GzipConfig{
	Level:     gzip.BestSpeed,
	MinLength: 1024,
}))
```

`Gzip` also adds `Vary: Accept-Encoding` when it compresses a response.
