---
title: Rewrite
description: Rewrite exact or wildcard paths before route dispatch.
---

`rewrite` changes the request path before routing, without telling the client. Use it to serve old URLs from new handlers, or to alias one path to another.

```go
import "github.com/0mjs/zinc/middleware/rewrite"

app.Use(rewrite.New(rewrite.Config{Rules: map[string]string{
	"/old":  "/new",
	"/v1/*": "/api/v1/*",
}}))
```

The client does not receive a redirect; handlers see the rewritten path, and query parameters are left alone. A rule ending in `*` matches a prefix, and a `*` in the target is replaced by the rest of the path.
