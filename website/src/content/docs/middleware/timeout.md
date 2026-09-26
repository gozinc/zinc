---
title: Context Timeout
description: Attach per-request deadlines to the handler chain.
---

`timeout` gives each request a deadline. Database calls, HTTP clients, and anything else that honours `c.Context()` stop when it passes, and the request fails with `503 Service Unavailable`.

```go
import "github.com/0mjs/zinc/middleware/timeout"

app.Use(timeout.New(timeout.Config{Timeout: 2 * time.Second}))
```

`Timeout` is required. The deadline only helps if downstream work uses the request context: code that ignores `c.Context()` keeps running. The handler is not moved to another goroutine, so cancellation stays ordinary Go.

## Config

| Field | Meaning |
|---|---|
| `Timeout` | Deadline for the rest of the chain |
| `ErrorHandler` | Turns a `*timeout.Error` into a response; the default answers 503 |

```go
app.Use(timeout.New(timeout.Config{
	Timeout: 2 * time.Second,
	ErrorHandler: func(c *zinc.Context, err *timeout.Error) error {
		return zinc.ServiceUnavailable("request timed out")
	},
}))
```

## Reading the deadline

```go
info, ok := timeout.Get(c)            // info.Timeout, info.Deadline
remaining, ok := timeout.Remaining(c) // time left before the deadline
```

`timeout.MustGet(c)` panics when the middleware did not run.

## Error behavior

If the chain returns `context.DeadlineExceeded` after the deadline passes, Zinc wraps it in a `*timeout.Error`, which matches `timeout.ErrExceeded` and `context.DeadlineExceeded` with `errors.Is`.
