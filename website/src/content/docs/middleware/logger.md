---
title: Request Logger
description: Log one structured line per request with slog, or hand the values to your own logger.
---

`logger` logs each request after the rest of the chain has run.

```go
import "github.com/0mjs/zinc/middleware/logger"

app.Use(logger.New())
```

It writes to `slog.Default()` at `INFO`, or at `ERROR` when the request ended with an error. A returned error goes through your error handler before the line is written, so the logged status is the one the client received.

Each line carries the method, URI, route pattern, status, latency, host, request and response sizes, user agent, remote IP, and request ID.

## Config

| Field | Meaning |
|---|---|
| `Logger` | The `*slog.Logger` for the default line. Nil uses `slog.Default()`. |
| `Log` | Replaces the default line. It receives every request's `logger.Values`. |
| `Headers` | Request headers to copy into `Values.Headers` |
| `QueryParams` | Query parameters to copy into `Values.QueryParams` |

## Send the values to your own logger

```go
app.Use(logger.New(logger.Config{
	Headers: []string{"X-Forwarded-For"},
	Log: func(c *zinc.Context, v logger.Values) error {
		slog.Info("http",
			"method", v.Method,
			"route", v.RoutePath,
			"status", v.Status,
			"latency", v.Latency,
		)
		return nil
	},
}))
```

An error returned from `Log` becomes the request's error.

## Skip noisy routes

Wrap the middleware with `zinc.Skip` to leave some requests out of the log:

```go
app.Use(zinc.Skip(func(c *zinc.Context) bool {
	return c.Path() == "/healthz"
}, logger.New()))
```
