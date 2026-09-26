---
title: Trailing Slash
description: Add, remove, or redirect trailing slash variants.
---

Zinc already treats `/users` and `/users/` as the same route unless `StrictRouting` is on. `trailingslash` is for strict apps, or when clients should be sent to one canonical URL. By default it removes the trailing slash before routing.

```go
import "github.com/0mjs/zinc/middleware/trailingslash"

app.Use(trailingslash.New())
```

This lets `/users/` match a route registered as `/users`.

| Field | Default | Meaning |
|---|---|---|
| `Add` | `false` | Appends a trailing slash instead of removing it |
| `Redirect` | `false` | Redirects the client to the normalized URL instead of routing it directly |
| `StatusCode` | `301` | Redirect status |

```go
app.Use(trailingslash.New(trailingslash.Config{
	Redirect:   true,
	StatusCode: zinc.StatusPermanentRedirect,
}))
```
