---
title: No Cache
description: Stop browsers and proxies from caching responses.
---

`nocache` sets `Cache-Control`, `Pragma`, and `Expires` so that no browser or intermediary stores or reuses the response.

```go
import "github.com/0mjs/zinc/middleware/nocache"

account := app.Group("/account", nocache.New())
```

Use it for auth pages, dashboards, and any route where cached browser history is not wanted. It has nothing to configure.
