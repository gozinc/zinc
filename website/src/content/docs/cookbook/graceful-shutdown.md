---
title: Graceful Shutdown
description: Stop accepting traffic and allow in-flight Zinc requests to finish.
---

When a deployment stops your process, it sends `SIGTERM`. This program stops accepting new connections, lets in-flight requests finish for up to ten seconds, then exits. Clients never see a dropped request.

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()
	app.Get("/health", func(c *zinc.Context) error {
		return c.String("ok")
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.ListenContext(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
```

`ListenContext` serves until `ctx` ends, then stops accepting connections and waits for in-flight requests. It returns `nil` once they finish. The wait is bounded by `Config.ShutdownTimeout`, ten seconds by default:

```go
app := zinc.New(zinc.Config{ShutdownTimeout: 30 * time.Second})
```

If requests are still running when the timeout expires, their connections are closed and `ListenContext` returns an error, so the process can exit with a non-zero status. Zinc installs no signal handlers itself; the context decides when to stop.

Readiness checks should fail before shutdown begins so a load balancer stops sending new traffic.

## Your own server

If you run Zinc on an `http.Server` you configure, call its `Shutdown` as usual, then `app.Close()` to release static-file roots.
