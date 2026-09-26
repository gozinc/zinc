---
title: Request ID
description: Generate or propagate request IDs through X-Request-ID.
---

`requestid` gives every request an ID and returns it in the `X-Request-ID` response header. Put it first in the chain so logs, errors, and upstream calls can all carry the same ID.

```go
import "github.com/0mjs/zinc/middleware/requestid"

app.Use(requestid.New())
```

If the request already includes `X-Request-ID`, Zinc reuses it. Otherwise Zinc generates a 16-byte random hex ID, writes it back to the request header, and publishes it on the response.

Read the ID in a handler with `requestid.Get`:

```go
app.Get("/events", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"request_id": requestid.Get(c)})
})
```

Without the middleware, `Get` falls back to the request's `X-Request-ID` header.

## Config

| Field | Default | Meaning |
|---|---|---|
| `Header` | `X-Request-ID` | Header carrying the ID on the request and the response |
| `Generate` | `requestid.Random` | Creates an ID when the request has none |

```go
app.Use(requestid.New(requestid.Config{
	Header:   "X-Correlation-ID",
	Generate: requestid.Static("local-dev"),
}))
```

`requestid.Static` always returns the same ID, which keeps test output stable.
