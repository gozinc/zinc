---
title: Structured Request Logs with slog
description: Send Zinc request data through Go's standard structured logger.
---

Zinc's request logger uses `log/slog` directly. Supply the logger your service
already uses and choose the request fields that belong in production logs.

## Setup

```bash
mkdir zinc-logging
cd zinc-logging
go mod init example.com/zinc-logging
go get github.com/0mjs/zinc
```

## Application

```go
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
		return c.JSON(zinc.Map{
			"id":         c.Param("id"),
			"request_id": requestid.Get(c),
		})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl http://localhost:8080/users/42
```

The log record includes the method, URI, route pattern, status, latency, remote
IP, request ID, request and response sizes, and the selected `Traceparent`
header. Avoid logging authorization, cookie, or other secret-bearing headers.

For complete control, set `logger.Config.Log`. It receives a `logger.Values`
snapshot for every request and can map fields into an existing logging or
observability pipeline.
