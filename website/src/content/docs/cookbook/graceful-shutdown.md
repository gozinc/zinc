---
title: Graceful Shutdown
description: Stop accepting traffic and allow in-flight Zinc requests to finish.
---

When a deployment stops your process, it sends `SIGTERM`. This program stops accepting new connections, lets in-flight requests finish for up to ten seconds, then exits. Clients never see a dropped request.

```go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()
	app.Get("/health", func(c *zinc.Context) error {
		return c.String("ok")
	})

	go func() {
		if err := app.Listen(":8080"); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}()

	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-signals.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}
```

Readiness checks should fail before shutdown begins so a load balancer stops sending new traffic.
