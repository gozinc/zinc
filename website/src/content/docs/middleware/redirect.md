---
title: Redirect
description: Redirect exact or wildcard paths before route dispatch.
---

`redirect` sends clients from an old path to a new one, before routing. Use it for moved pages and renamed API prefixes.

```go
import "github.com/0mjs/zinc/middleware/redirect"

app.Use(redirect.New(redirect.Config{Rules: map[string]string{
	"/old":  "/new",
	"/v1/*": "/api/v1/*",
}}))
```

A rule ending in `*` matches a prefix, and a `*` in the target is replaced by the rest of the path. Query strings are preserved.

Set `StatusCode` when the default `301 Moved Permanently` is not right:

```go
app.Use(redirect.New(redirect.Config{
	Rules:      map[string]string{"/login": "/signin"},
	StatusCode: zinc.StatusTemporaryRedirect,
}))
```
