---
title: Recover
description: Turn a panic in a handler into a 500 response instead of a dropped connection.
---

Recover catches a panic in any later middleware or handler and answers `500 Internal Server Error`, so one bad request doesn't drop the connection. Add it to every app; the only question is where it goes in the chain.

## Usage

```go
import "github.com/0mjs/zinc/middleware/recover"

app.Use(logger.New(), recover.New())

app.Get("/boom", func(c *zinc.Context) error {
	var counts map[string]int
	counts["x"]++ // panics: assignment to entry in nil map
	return nil
})
```

```bash
curl -i http://localhost:8080/boom
# HTTP/1.1 500 Internal Server Error
# {"error":{"status":500,"message":"Internal Server Error"}}
```

```text
2026/09/28 01:15:56 ERROR REQUEST_ERROR method=GET uri=/boom route=/boom status=500 ... error="Internal Server Error\nrecover: panic recovered: assignment to entry in nil map"
```

The panic becomes an error that goes through your error handler like any other. Register it after [`logger`](/middleware/logger/), so the logger sees and records the `500`.

:::note[The package name hides the builtin]
In a file that imports this package, `recover` means the package, not Go's builtin, and a call to `recover()` won't compile. If that file needs the builtin, import the package under another name: `recovermw "github.com/0mjs/zinc/middleware/recover"`.
:::

## Defaults

| Setting | Default |
|---|---|
| Response | `500 Internal Server Error` |
| Stack trace | captured, up to 4 KiB |

## Configuration

This logs the panic value and stack, and sends its own message:

```go
app.Use(recover.New(recover.Config{
	Handler: func(c *zinc.Context, err *recover.Error) error {
		log.Printf("panic: %v\n%s", err.Value, err.Stack)
		return zinc.InternalServerError("something went wrong")
	},
}))
```

```bash
curl http://localhost:8080/boom
# {"error":{"status":500,"message":"something went wrong"}}
```

```text
2026/09/28 01:15:56 panic: out of widgets
goroutine 8 [running]:
github.com/0mjs/zinc/middleware/recover.captureRecoverStack(...)
...
```

| Field | Default | Meaning |
|---|---|---|
| `Handler` | returns a `500` error | `func(*zinc.Context, *recover.Error) error`. Turns the recovered panic into the request's error or response. |
| `StackSize` | 4 KiB | Largest stack trace captured, in bytes. `0` keeps the default; a negative value panics when the middleware is created. |
| `DisableStack` | `false` | Skips stack capture, leaving `Error.Stack` nil |

A `*recover.Error` has two fields: `Value`, what was passed to `panic`, and `Stack`, the current goroutine's stack.

## Errors

The default handler returns an error that matches both `zinc.ErrInternalServerError` and `*recover.Error`:

```go
errors.Is(err, zinc.ErrInternalServerError) // → true
errors.As(err, &recoverErr)                 // → true; recoverErr.Value holds the panic value
```

Two panics are not recovered:

- **`http.ErrAbortHandler`** is panicked again, so `net/http` can abort the response as it intends.
- **Panics in goroutines your handler starts** can't be caught by any middleware. Recover them inside the goroutine.

## Related

- [Request Logger](/middleware/logger/) records the `500` and the panic message.
- [Errors](/guide/errors/) covers the error handler that writes the response.
- [Request ID](/middleware/requestid/) lets you match a panic in the logs to a client report.
