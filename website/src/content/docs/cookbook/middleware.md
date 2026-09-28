---
title: Custom Middleware
description: Write your own middleware that runs code before and after each handler and logs the status and size of every response.
---

This program adds a middleware that logs the method, path, status, response size and duration of every request. You'd write your own middleware when something should happen around many handlers, and none of the built-in middleware does it. A Zinc middleware is a handler that calls `c.Next()` to run the rest of the chain.

## The program

```go title="main.go"
package main

import (
	"log"
	"time"

	"github.com/0mjs/zinc"
)

// timing logs the method, path, status, response size and duration
// of every request.
func timing(c *zinc.Context) error {
	started := time.Now()
	base := c.Writer()
	writer := zinc.WrapResponseWriter(base)
	c.SetWriter(writer)
	defer c.SetWriter(base)

	err := c.Next()
	if err != nil {
		c.HandleError(err)
	}
	log.Printf(
		"%s %s status=%d bytes=%d duration=%s",
		c.Method(),
		c.Path(),
		writer.Status(),
		writer.BytesWritten(),
		time.Since(started),
	)
	return err
}

func main() {
	app := zinc.New()
	app.Use(timing)

	app.Get("/", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"ok": true})
	})

	app.Get("/users/{id}", func(c *zinc.Context) error {
		return zinc.NotFound("user not found")
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl http://localhost:8080/
# {"ok":true}

curl -i http://localhost:8080/users/7
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":404,"message":"user not found"}}

curl http://localhost:8080/nope
# {"error":{"status":404,"message":"Not Found"}}
```

The server logs each request with the status and size the client received:

```text
2026/09/28 01:21:30 GET / status=200 bytes=12 duration=181.417µs
2026/09/28 01:21:30 GET /users/7 status=404 bytes=52 duration=22.584µs
2026/09/28 01:21:30 GET /nope status=404 bytes=47 duration=33.417µs
```

Middleware added with `app.Use` runs for requests that match no route too, which is why `/nope` is logged.

## How it works

- `zinc.WrapResponseWriter(base)` wraps the response writer so it records the status and the number of bytes written. `c.SetWriter(writer)` makes the rest of the chain write through it.
- Code before `c.Next()` runs before the handler; code after it runs once the handler has returned.
- `c.HandleError(err)` runs Zinc's error handler now, so the error response is written through the wrapper before you read its status and size. Returning `err` afterwards is safe: the error handler runs only once per request.
- `defer c.SetWriter(base)` puts the original writer back after the chain, so nothing holds on to your wrapper once the request is done.

:::caution[Call HandleError before you measure]
Without `c.HandleError(err)`, the error response hasn't been written when you log. The `/users/7` line would read `status=200 bytes=0`, although the client gets a 52-byte `404`.
:::

## Register middleware where it's needed

Apply middleware at the narrowest scope that makes sense:

```go
app.Use(timing)                                     // every request
admin := app.Group("/admin", requireAdmin)          // routes in one group
app.Get("/expensive", rateLimit, expensiveHandler)  // a single route
```

`requireAdmin`, `rateLimit` and `expensiveHandler` stand for your own handlers. Standard `net/http` middleware, `func(http.Handler) http.Handler`, goes in `app.UseHTTP` and needs no adapter.

## Good to know

### The wrapper keeps optional writer features

The writer from `zinc.WrapResponseWriter` still supports the optional interfaces of the writer it wraps, such as `http.Flusher`, so streaming handlers keep working behind your middleware. If the writer is already wrapped, you get the same one back.

## See also

- [Groups and Middleware](/guide/groups-and-middleware/): ordering, groups and per-route middleware.
- [Response Writer](/api/response-writer/): `WrapResponseWriter` and its methods.
- [Structured Request Logs with slog](/cookbook/structured-logging/): the built-in logger, which does this for you.
