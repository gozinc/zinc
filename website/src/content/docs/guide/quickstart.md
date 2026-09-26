---
title: Quickstart
description: Go from an empty folder to a running Zinc API in about five minutes.
---

This guide takes you from an empty folder to a running JSON API. You need a terminal and about five minutes.

## Before you start

Zinc requires **Go 1.25 or newer**. Check your version:

```bash
go version
```

If Go is missing or older than 1.25, install the latest release from [go.dev/dl](https://go.dev/dl/), then open a new terminal.

## 1. Create a project

Make a folder and initialize a Go module inside it:

```bash
mkdir hello-zinc
cd hello-zinc
go mod init example.com/hello-zinc
```

The module path names your project. Use your repository path, such as `github.com/you/hello-zinc`, once you have one.

## 2. Add Zinc

```bash
go get github.com/0mjs/zinc
```

This records Zinc in `go.mod` and `go.sum`. The first-party middleware packages ship in the same module, so there is nothing else to install.

## 3. Write the server

Create `main.go`:

```go
package main

import (
	"log"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/logger"
	"github.com/0mjs/zinc/middleware/recover"
)

func main() {
	app := zinc.New()

	app.Use(
		logger.New(),
		recover.New(),
	)

	app.Get("/", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"message": "Hello from Zinc!"})
	})

	app.Get("/hello/{name}", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"message": "Hello, " + c.Param("name") + "!"})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## 4. Run it

```bash
go run .
```

The server is now listening on `http://localhost:8080`. In a second terminal, call both routes:

```bash
curl http://localhost:8080/
curl http://localhost:8080/hello/gopher
```

```json
{"message":"Hello from Zinc!"}
{"message":"Hello, gopher!"}
```

The first terminal shows one structured log line per request. Press `Ctrl+C` there to stop the server.

## What you just built

| Line | What it does |
|---|---|
| `zinc.New()` | Creates an app with sensible defaults. The app is an `http.Handler`. |
| `app.Use(...)` | Adds middleware that runs on every request: logging and panic recovery. |
| `app.Get("/", ...)` | Registers a handler for `GET /`. |
| `{name}` | Captures one path segment, read with `c.Param("name")`. |
| `c.JSON(...)` | Encodes the value and sets `Content-Type: application/json`. |
| `app.Listen(":8080")` | Starts a standard-library `http.Server`. |

Every handler has the same shape, `func(c *zinc.Context) error`. You write the response through `c`, or return an error and let Zinc turn it into a response. That one rule carries through everything else in these docs.

## Next steps

- [Your First Route](/guide/first-route/) builds a small endpoint that reads input, validates it, and returns errors.
- [Routing](/guide/routing/) covers patterns, groups, and matching rules.
- [Middleware](/middleware/overview/) lists every first-party middleware.
