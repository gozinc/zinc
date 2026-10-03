---
title: Context Timeout
description: Give each request a deadline, so slow database and HTTP calls stop and the client gets a 503.
---

Context Timeout gives each request a deadline. Database queries, HTTP clients and anything else that takes `c.Context()` stop when it passes, and the client gets `503 Service Unavailable`. Add it when a slow dependency shouldn't hold a request open indefinitely.

## Usage

```go
import "github.com/0mjs/zinc/middleware/timeout"

app.Use(timeout.New(timeout.Config{Timeout: 2 * time.Second}))

app.Get("/report", func(c *zinc.Context) error {
	rows, err := db.QueryContext(c.Context(), reportSQL) // db, reportSQL: your data layer
	if err != nil {
		return err
	}
	defer rows.Close()
	// ...
	return c.String("done")
})
```

```bash
curl -i http://localhost:8080/report   # when the query takes longer than 2s
# HTTP/1.1 503 Service Unavailable
# {"error":{"status":503,"message":"Service Unavailable"}}
```

The deadline only stops work that watches the request context. Code that ignores `c.Context()` keeps running after the deadline, and its result is still sent.

:::note[Your handler stays on its own goroutine]
The middleware doesn't run your handler on a separate goroutine or write a response while it's still working. It sets a deadline on `c.Context()` and waits for your handler to return, so cancellation works like it does anywhere else in Go.
:::

## Defaults

| Setting | Default |
|---|---|
| Timeout | none: **required** |
| Response when the deadline passes | `503 Service Unavailable` |

## Configuration

This sets a `Retry-After` header and names the timeout in the message:

```go
app.Use(timeout.New(timeout.Config{
	Timeout: 2 * time.Second,
	ErrorHandler: func(c *zinc.Context, err *timeout.Error) error {
		c.SetHeader("Retry-After", "5")
		return zinc.ServiceUnavailable("report took longer than " + err.Info.Timeout.String())
	},
}))
```

```bash
curl -i http://localhost:8080/report
# HTTP/1.1 503 Service Unavailable
# Retry-After: 5
# {"error":{"status":503,"message":"report took longer than 2s"}}
```

| Field | Default | Meaning |
|---|---|---|
| `Timeout` | **required** | Deadline for the rest of the chain. Zero or negative panics when the middleware is created. |
| `ErrorHandler` | returns a `503` error | `func(*zinc.Context, *timeout.Error) error`. Turns a deadline error into the request's error or response. |

## Reading state

Read the deadline from a handler, for example to pass a shorter budget to a downstream call:

```go
info, ok := timeout.Get(c)            // info.Timeout → 2s, info.Deadline → when it expires
remaining, ok := timeout.Remaining(c) // time left; 0 once the deadline has passed
info = timeout.MustGet(c)             // like Get, but panics when the middleware didn't run
```

Without the middleware, `Get` and `Remaining` return `ok == false`.

## Errors

When the chain returns an error that matches `context.DeadlineExceeded`, the middleware wraps it in a `*timeout.Error` and passes it to `ErrorHandler`. Other errors pass through unchanged.

The default handler's error matches all of these with `errors.Is` and `errors.As`:

- `zinc.ErrServiceUnavailable`, which makes the response a `503`
- `timeout.ErrExceeded` and `context.DeadlineExceeded`
- `*timeout.Error`, whose `Info` holds the timeout and deadline, and `Cause` the original error

:::caution[Any deadline error counts]
The middleware checks the error, not the clock. If your handler sets its own shorter deadline with `context.WithTimeout` and returns its `context.DeadlineExceeded`, the client also gets a `503`.
:::

To list the `503` in the [OpenAPI](/guide/openapi/#describe-what-middleware-adds) spec, pass `timeout.Doc()` to `Document` beside the middleware:

```go
app.Use(timeout.New())
app.Document(timeout.Doc())
```

## Related

- [Request timeouts](/cookbook/timeout/) walks through deadlines for database and HTTP calls.
- [Configuration](/guide/configuration/) covers the server's `ReadTimeout` and `WriteTimeout`.
- [Rate Limiter](/middleware/limiter/) caps how many slow requests can run at once.
