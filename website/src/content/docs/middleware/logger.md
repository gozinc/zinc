---
title: Request Logger
description: Write one structured log line per request with slog, or send the values to your own logger.
---

The logger writes one line for every request: method, path, route, status, latency and a few more fields. Add it to any service you run, so you can see what it answered and how long it took.

## Usage

```go
import (
	"github.com/0mjs/zinc/middleware/logger"
	"github.com/0mjs/zinc/middleware/requestid"
)

app.Use(requestid.New(), logger.New())

app.Get("/users/{id}", func(c *zinc.Context) error {
	if c.Param("id") == "0" {
		return zinc.NotFound("user not found")
	}
	return c.JSON(zinc.Map{"id": c.Param("id")})
})
```

```bash
curl http://localhost:8080/users/42
curl http://localhost:8080/users/0
```

```text
2026/09/28 01:18:28 INFO REQUEST method=GET uri=/users/42 route=/users/{id} status=200 latency=157µs host=localhost:8080 bytes_in="" bytes_out=12 user_agent=curl/8.7.1 remote_ip=::1 request_id=cad891ee0a72092ded6fb64d17c87158
2026/09/28 01:18:28 INFO REQUEST method=GET uri=/users/0 route=/users/{id} status=404 latency=15.916µs host=localhost:8080 bytes_in="" bytes_out=52 user_agent=curl/8.7.1 remote_ip=::1 request_id=c059b23c10bde4b7bee10dbe1ec2d912 error="user not found"
```

A request that ends in a `5xx` logs at `ERROR` with the message `REQUEST_ERROR`; everything else logs at `INFO` with `REQUEST`. The line is written after your error handler has run, so `status` is the status the client received. A `404` is normal traffic, so it stays at `INFO`, with an `error` key saying why.

Register [`requestid`](/middleware/requestid/) too, so `request_id` is filled in. Put the logger before [`recover`](/middleware/recover/), so it logs the `500` a panic turns into.

### What each line contains

| Key | Value |
|---|---|
| `method` | Request method |
| `uri` | Path and query string, as sent |
| `route` | The matched route pattern, such as `/users/{id}`. Left out when no route matched. |
| `status` | Status code sent to the client |
| `latency` | Time spent in the rest of the chain |
| `host` | `Host` header |
| `bytes_in` | The request's `Content-Length` header, as a string. Empty when there is none. |
| `bytes_out` | Response body bytes written |
| `user_agent` | `User-Agent` header |
| `remote_ip` | Client IP from [`c.IP()`](/guide/ip-address/) |
| `request_id` | `X-Request-ID` from the request, or else from the response |
| `headers` | Headers you listed in `Headers`, when any were present |
| `query` | Query parameters you listed in `QueryParams`, when any were present |
| `error` | The error's message, when the chain returned an error |

## Defaults

| Setting | Default |
|---|---|
| Logger | `slog.Default()` |
| Line | the keys above, at `INFO` or `ERROR` |
| Extra headers | none |
| Extra query parameters | none |

## Configuration

This logs JSON to standard output and adds one header and one query parameter to each line:

```go
app.Use(logger.New(logger.Config{
	Logger:      slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	Headers:     []string{"X-Forwarded-For"},
	QueryParams: []string{"page"},
}))
```

```bash
curl -H 'X-Forwarded-For: 203.0.113.7' 'http://localhost:8080/users?page=2'
```

```json
{"time":"2026-09-28T01:19:05.307406+01:00","level":"INFO","msg":"REQUEST","method":"GET","uri":"/users?page=2","route":"/users","status":200,"latency":159250,"host":"localhost:8080","bytes_in":"","bytes_out":12,"user_agent":"curl/8.7.1","remote_ip":"::1","request_id":"c13b1b2533404734b308d967325e22f2","headers":{"X-Forwarded-For":["203.0.113.7"]},"query":{"page":["2"]}}
```

slog's JSON handler writes `latency` in nanoseconds.

| Field | Default | Meaning |
|---|---|---|
| `Logger` | `slog.Default()` | The `*slog.Logger` that receives the default line, logged with the request's context so a handler can read trace IDs from it. Ignored when `Log` is set. |
| `Log` | the line shown above | `func(*zinc.Context, logger.Values) error`. Replaces the default line and receives every request's values. |
| `Headers` | none | Request headers to copy into `Values.Headers` and the `headers` key. Names are matched in canonical form, so `x-forwarded-for` works too. |
| `QueryParams` | none | Query parameters to copy into `Values.QueryParams` and the `query` key. Names must match exactly. |

## Send the values to your own logger

Set `Log` to write the line yourself, with your own keys or your own logging library:

```go
app.Use(logger.New(logger.Config{
	Headers: []string{"X-Forwarded-For"},
	Log: func(c *zinc.Context, v logger.Values) error {
		slog.Info("http",
			"method", v.Method,
			"route", v.RoutePath,
			"status", v.Status,
			"latency", v.Latency,
			"forwarded_for", v.Headers["X-Forwarded-For"],
		)
		return nil
	},
}))
```

```text
time=2026-09-28T01:15:25.669+01:00 level=INFO msg=http method=GET route=/users/{id} status=200 latency=47.416µs forwarded_for=[203.0.113.7]
```

`logger.Values` has these fields: `StartTime`, `Latency`, `Method`, `URI`, `RoutePath`, `Status`, `Error`, `RemoteIP`, `Host`, `UserAgent`, `RequestID`, `ContentLength` (the header, as a string), `ResponseSize`, `Headers` and `QueryParams`. `Headers` and `QueryParams` are nil unless you listed names in the config.

## Leave out noisy routes

Wrap the middleware with `zinc.Skip` to leave some requests out of the log, such as health probes:

```go
app.Use(zinc.Skip(func(c *zinc.Context) bool {
	return c.Path() == "/healthz"
}, logger.New()))
```

## Errors

- The level follows the status the client received: `5xx` is `ERROR` as `REQUEST_ERROR`, anything else is `INFO` as `REQUEST`. A `4xx` your handler returns, such as `zinc.NotFound(...)`, is `INFO` with an `error` key.
- The `error` key holds the error's own message, which may differ from what the client saw. A plain `errors.New("db down")` logs `error="db down"`, while the client gets `Internal Server Error`.
- A request that matched no route logs at `INFO` with `error="Not Found"` or `error="Method Not Allowed"`, and no `route` key, whichever error handler you use.
- An error returned from your `Log` function becomes the request's error.

## Related

- [Request ID](/middleware/requestid/) fills in `request_id`, so you can match a log line to a client report.
- [Recover](/middleware/recover/) turns panics into `500` responses the logger can record.
- [Structured logging](/cookbook/structured-logging/) shows a full logging setup with slog.
- [Client IP and proxies](/guide/ip-address/) makes `remote_ip` accurate behind a load balancer.
