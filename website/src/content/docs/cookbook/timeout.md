---
title: Request Timeouts
description: Apply request deadlines and make downstream work stop when the deadline expires.
---

The timeout middleware changes the request context; downstream database, HTTP,
and application calls must use `c.Context()` for cancellation to propagate.

```go
package main

import (
	"log"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/timeout"
)

func main() {
	app := zinc.New()
	app.Use(zinc.Skip(func(c *zinc.Context) bool {
		return c.Path() == "/events"
	}, timeout.New(timeout.Config{Timeout: 3 * time.Second})))

	app.Get("/report", func(c *zinc.Context) error {
		select {
		case <-time.After(2 * time.Second):
			return c.JSON(zinc.Map{"status": "ready"})
		case <-c.Context().Done():
			return c.Context().Err()
		}
	})

	log.Fatal(app.Listen(":8080"))
}
```

Reduce the middleware timeout below two seconds to see Zinc return the default
`503 Service Unavailable` timeout error.

:::caution[Long-lived connections]
Do not apply short deadlines to WebSockets, server-sent events, or intentionally long-lived streams. Skip those routes or use a separate group.
:::
