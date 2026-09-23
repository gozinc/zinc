---
title: Request ID
description: Generate or propagate request IDs through X-Request-ID.
---

`RequestID` gives every request an ID and returns it in the `X-Request-ID` response header. Put it first in the chain so logs, errors, and upstream calls can all carry the same ID.

```go
app.Use(middleware.RequestID())
```

If the request already includes `X-Request-ID`, Zinc reuses it. Otherwise Zinc generates a 16-byte random hex ID, writes it back to the request header, and publishes it on the response.

```go
app.Get("/events", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"request_id": middleware.RequestIDValue(c),
	})
})
```

Use `RequestIDWithConfig` when a different header or generator is needed.

```go
app.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
	Header:    "X-Correlation-ID",
	Generator: middleware.StaticRequestID("local-dev"),
}))
```

Inside handlers, use:

- `RequestIDValue(c)`
- `RequestIDCurrent(c)`
- `MustRequestIDCurrent(c)`
