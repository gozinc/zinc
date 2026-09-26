---
title: Content Type
description: Reject request bodies in media types or content codings the endpoint does not accept.
---

`contenttype` stops unsupported request bodies before handlers or binders read them, answering `415 Unsupported Media Type`.

```go
import "github.com/0mjs/zinc/middleware/contenttype"

api.Use(contenttype.New(contenttype.Config{
	Types:     []string{"application/json"},
	Encodings: []string{"identity", "gzip"},
}))
```

| Field | Meaning |
|---|---|
| `Types` | Accepted media types. Parameters are ignored, so `application/json; charset=utf-8` matches `application/json`. |
| `Encodings` | Accepted content codings. A request without `Content-Encoding` counts as `identity`, so list it to allow plain bodies. |

An empty list allows anything, so you can check one without the other.
