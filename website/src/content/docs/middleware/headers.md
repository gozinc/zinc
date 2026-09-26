---
title: Headers
description: Set static response headers and choose middleware by request header.
---

`headers` sets response headers before handlers run, and can run a different middleware when a request carries a particular header.

```go
import "github.com/0mjs/zinc/middleware/headers"

app.Use(headers.New(headers.Config{
	Set: map[string]string{"X-App": "zinc"},
}))
```

For common browser security headers, use [Secure Headers](/middleware/secure/) instead.

## Header routes

```go
app.Use(headers.New(headers.Config{
	Routes: []headers.Route{{
		Header: "X-Admin",
		Value:  "1",
		Middleware: func(c *zinc.Context) error {
			c.Set("admin", true)
			return c.Next()
		},
	}},
}))
```

Routes are checked in order, and the first match runs in place of the rest of the chain; that middleware decides whether to call `c.Next()`. If nothing matches, the request continues normally. An empty `Value` matches any non-empty value for the header. A route without a `Header` or `Middleware` panics when the middleware is created.
