---
title: Custom Middleware
description: Write reusable Zinc middleware with before-and-after behavior and accurate response measurements.
---

Middleware has the same signature as a route handler. Call `c.Next()` to run the
rest of the chain.

```go
package main

import (
	"log"
	"time"

	"github.com/0mjs/zinc"
)

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

	log.Fatal(app.Listen(":8080"))
}
```

Calling `c.HandleError(err)` before measuring lets Zinc's configured error handler
write through the wrapper, so failed requests get the correct status and byte
count. Returning the same error remains safe because Zinc tracks whether the
response has already been written.

The wrapper preserves optional standard writer capabilities. Restore the base
writer after the chain so request state does not retain your wrapper.

Register middleware at the narrowest useful scope:

```go
app.Use(timing)
admin := app.Group("/admin", requireAdmin)
app.Get("/expensive", rateLimit, expensiveHandler)
```

Standard `net/http` middleware belongs in `app.UseHTTP` and needs no adapter.
