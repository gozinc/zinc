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
	"github.com/0mjs/zinc/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	loggerConfig := middleware.DefaultRequestLoggerConfig()
	loggerConfig.Logger = logger
	loggerConfig.LogRoutePath = true
	loggerConfig.LogHeaders = []string{"Traceparent"}

	app := zinc.New()
	app.Use(
		middleware.RequestID(),
		middleware.RequestLoggerWithConfig(loggerConfig),
		middleware.Recover(),
	)

	app.Get("/users/{id}", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{
			"id":         c.Param("id"),
			"request_id": c.Header(zinc.HeaderXRequestID),
		})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl http://localhost:8080/users/42
```

The log record includes latency, method, URI, status, remote IP, request ID,
content length, response size, and the selected `Traceparent` header. Avoid
logging authorization, cookie, or other secret-bearing headers.

For complete control, set `RequestLoggerConfig.LogValuesFunc`. It receives a
`RequestLoggerValues` snapshot and can map fields into an existing logging or
observability pipeline.
