---
title: Request Timeouts
description: Give every request a 3-second budget and answer 503 when a handler runs over it.
---

This program gives every request three seconds. A handler that finishes in time answers as usual; one that runs over gets `503 Service Unavailable`. You'd want this so a slow database or upstream service can't hold connections open indefinitely. The [Context Timeout](/middleware/timeout/) middleware sets the deadline, and your handler passes `c.Context()` to the slow calls so they stop when it passes.

## The program

```go title="main.go"
package main

import (
	"context"
	"log"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/timeout"
)

// work stands in for a database query or an outgoing HTTP call.
// Like those, it gives up as soon as ctx is cancelled.
func work(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	app := zinc.New()
	app.Use(timeout.New(timeout.Config{Timeout: 3 * time.Second}))

	app.Get("/report", func(c *zinc.Context) error {
		if err := work(c.Context(), 2*time.Second); err != nil {
			return err
		}
		return c.JSON(zinc.Map{"status": "ready"})
	})

	app.Get("/slow", func(c *zinc.Context) error {
		if err := work(c.Context(), 5*time.Second); err != nil {
			return err
		}
		return c.JSON(zinc.Map{"status": "ready"})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

`/report` takes two seconds, inside the budget:

```bash
curl -i http://localhost:8080/report
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
#
# {"status":"ready"}
```

`/slow` would take five, so it's stopped at three:

```bash
curl -i http://localhost:8080/slow
# HTTP/1.1 503 Service Unavailable
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":503,"message":"Service Unavailable"}}
```

## How it works

- `timeout.New(timeout.Config{Timeout: 3 * time.Second})` replaces the request's context with one that's cancelled after three seconds.
- `work(c.Context(), ...)` passes that context down. Database drivers, `http.NewRequestWithContext` and most client libraries accept a context the same way, and return an error when it's cancelled.
- The handler returns that error. The middleware sees a deadline error and turns it into `503`.
- The handler runs on the request's own goroutine, so it's never left running in the background after the response is sent.

:::caution[The deadline only stops code that checks it]
The middleware can't interrupt a handler. A handler that calls `time.Sleep(5 * time.Second)`, or a query run without `c.Context()`, runs to the end and answers `200` after five seconds.
:::

## Before production

- Keep the budget below the server's `WriteTimeout`, ten seconds by default. Past that, the connection is cut and the client gets no `503` at all. See [Configuration](/guide/configuration/).
- Don't put short deadlines on WebSockets, server-sent events or other long-lived streams. Register those routes outside the group that uses the timeout, or skip them with `zinc.Skip`.
- To send a different status or body, set `timeout.Config.ErrorHandler`.

## See also

- [Context Timeout](/middleware/timeout/): `ErrorHandler`, `timeout.Remaining` and the other options.
- [Context](/guide/context/): when `c.Context()` is cancelled, and running work past the response.
- [Graceful Shutdown](/cookbook/graceful-shutdown/): let requests finish when the process stops.
