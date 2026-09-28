---
title: Structured Request Logs with slog
description: Write one JSON log line per request through Go's log/slog, with the request ID and the headers you choose.
---

This program writes one JSON log line for every request, with the method, route, status, latency and request ID. You'd want it when logs go to a system that searches by field, such as Loki, Datadog or CloudWatch. Zinc's [Request Logger](/middleware/logger/) takes a `*slog.Logger`, so it can use the one your service already has.

## The program

```go title="main.go"
package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/logger"
	"github.com/0mjs/zinc/middleware/recover"
	"github.com/0mjs/zinc/middleware/requestid"
)

func main() {
	jsonLog := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	app := zinc.New()
	app.Use(
		requestid.New(),
		logger.New(logger.Config{
			Logger:  jsonLog,
			Headers: []string{"Traceparent"},
		}),
		recover.New(),
	)

	app.Get("/users/{id}", func(c *zinc.Context) error {
		id, err := zinc.Param[int](c, "id")
		if err != nil {
			return err
		}
		return c.JSON(zinc.Map{
			"id":         id,
			"request_id": requestid.Get(c),
		})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl http://localhost:8080/users/42 \
  -H 'Traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'
# {"id":42,"request_id":"43bac861b22f6ca7968edfe2ac36583e"}
```

The server prints one line for that request (wrapped here to fit):

```json
{"time":"2026-09-28T01:16:57.531307+01:00","level":"INFO","msg":"REQUEST",
 "method":"GET","uri":"/users/42","route":"/users/{id}","status":200,
 "latency":163459,"host":"localhost:8080","bytes_in":"","bytes_out":58,
 "user_agent":"curl/8.7.1","remote_ip":"::1",
 "request_id":"43bac861b22f6ca7968edfe2ac36583e",
 "headers":{"Traceparent":["00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"]}}
```

A request that fails gets an `error` field. A client error such as this `400` stays at `INFO`:

```bash
curl http://localhost:8080/users/abc
# {"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}
```

```json
{"time":"2026-09-28T01:16:57.543319+01:00","level":"INFO","msg":"REQUEST",
 "method":"GET","uri":"/users/abc","route":"/users/{id}","status":400,
 "latency":280750,"host":"localhost:8080","bytes_in":"","bytes_out":97,
 "user_agent":"curl/8.7.1","remote_ip":"::1",
 "request_id":"4358504898202cd1b078a38940633cb4",
 "error":"bind path: strconv.Atoi: parsing \"abc\": invalid syntax"}
```

## How it works

- `requestid.New()` comes first, so the ID it sets is on the request by the time the logger reads it. `requestid.Get(c)` returns the same ID to your handler.
- `logger.Config{Logger: jsonLog}` sends each line through your `slog.Logger`, so its handler decides the format and destination.
- `Headers: []string{"Traceparent"}` copies only the request headers you list into a `headers` field.
- The logger runs your error handler before it writes the line, so `status` is the status the client received. `route` is the pattern, which groups `/users/42` and `/users/7` together.

## Before production

- Don't list `Authorization`, `Cookie` or other headers that carry secrets in `Headers`.
- Add the query parameters you want with `QueryParams`; the rest of the query string still appears in `uri`.
- To reshape the line completely, set `logger.Config.Log`. It receives a `logger.Values` snapshot for every request, which you can map into your existing logging or observability pipeline.

## Good to know

### Latency is in nanoseconds

`slog.NewJSONHandler` writes a `time.Duration` as an integer number of nanoseconds, so `"latency":163459` is about 0.16 ms. `slog.NewTextHandler` writes it as `163.459µs` instead.

### Only server errors log at ERROR

A `5xx` logs at `ERROR` with the message `REQUEST_ERROR`, so alerts on `ERROR` fire for faults in your server, not for bots probing missing pages. A `400` or `404` logs at `INFO` with an `error` field. To choose levels yourself, write your own `Log` function and pick from `Values.Status`.

## See also

- [Request Logger](/middleware/logger/): every field and the default attribute keys.
- [Request ID](/middleware/requestid/): where the ID comes from, and when an incoming one is reused.
- [Custom Middleware](/cookbook/middleware/): measure status and bytes in your own middleware.
