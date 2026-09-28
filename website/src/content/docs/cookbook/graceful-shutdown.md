---
title: Graceful Shutdown
description: Stop taking new connections on SIGTERM and let the requests already running finish before the process exits.
---

This program stops accepting connections when it gets `SIGTERM` or Ctrl-C, waits for requests already in progress, then exits. You want this whenever something else stops your process: a deploy, a scale-down, or `docker stop` all send `SIGTERM` first. Zinc's `app.ListenContext` does the waiting; a context from `signal.NotifyContext` decides when to start.

## The program

```go title="main.go"
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()

	app.Get("/slow", func(c *zinc.Context) error {
		time.Sleep(5 * time.Second) // stands in for slow work
		return c.String("done\n")
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutting down: waiting for in-flight requests")
	}()

	log.Println("listening on :8080")
	if err := app.ListenContext(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
	log.Println("stopped cleanly")
}
```

## Try it

Start a slow request, then stop the server while it's running:

```bash
curl -i http://localhost:8080/slow &
sleep 1
kill -TERM <pid>    # or press Ctrl-C in the server's terminal
```

The request that was already running still gets its answer:

```bash
# HTTP/1.1 200 OK
# Content-Type: text/plain; charset=utf-8
# Content-Length: 5
# Connection: close
#
# done
```

The server logs the drain and exits with status 0:

```text
2026/09/28 01:16:07 listening on :8080
2026/09/28 01:16:09 shutting down: waiting for in-flight requests
2026/09/28 01:16:13 stopped cleanly
```

A new request sent after the signal is refused, because the listener has closed:

```bash
curl -i http://localhost:8080/slow
# curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server
```

## How it works

- `signal.NotifyContext` cancels `ctx` on `SIGINT` (Ctrl-C) or `SIGTERM`. Zinc installs no signal handlers itself, so this context decides when to stop.
- `app.ListenContext(ctx, ":8080")` serves until `ctx` ends, then stops accepting connections and waits for in-flight requests. It returns `nil` once they finish.
- The wait is bounded by `Config.ShutdownTimeout`, ten seconds by default. Requests still running when it expires have their connections closed, and `ListenContext` returns an error.
- `log.Fatal(err)` turns that error into a non-zero exit status, so your platform can see the shutdown wasn't clean.

## Change the drain time

Give slow requests longer to finish:

```go
app := zinc.New(zinc.Config{ShutdownTimeout: 30 * time.Second})
```

With a 2-second timeout instead, the 5-second request above is cut off. `curl` reports `(52) Empty reply from server`, and the server exits with status 1 after logging:

```text
2026/09/28 01:16:25 zinc: graceful shutdown: context deadline exceeded
```

A negative `ShutdownTimeout` waits until every request finishes, however long that takes.

## Before production

- Make the readiness check fail before shutdown begins, so the load balancer stops sending new traffic first. See [Health and Readiness Checks](/cookbook/health-readiness/).
- Keep `ShutdownTimeout` shorter than your platform's grace period. Kubernetes waits 30 seconds by default before it sends `SIGKILL`.

## Good to know

### Running your own http.Server

If you serve Zinc from an `http.Server` you configure yourself, call its `Shutdown` as usual, then `app.Close()` to release static-file roots.

## See also

- [Configuration](/guide/configuration/): `ShutdownTimeout` and the server timeouts.
- [Health and Readiness Checks](/cookbook/health-readiness/): take an instance out of rotation.
- [Docker and Environment Config](/cookbook/docker-config/): the same shutdown inside a container.
