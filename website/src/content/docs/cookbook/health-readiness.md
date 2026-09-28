---
title: Health and Readiness Checks
description: Give your orchestrator two endpoints, one that says the process is alive and one that says it can serve traffic.
---

This program answers two probes: `/live` says the process is running, and `/ready` says its dependencies are reachable. You need both when a platform such as Kubernetes or a load balancer decides whether to restart your process or send it traffic. Keeping them apart means a database outage takes the instance out of rotation without restarting a process that's working fine.

## The program

```go title="main.go"
package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/healthcheck"
)

// check reports whether one dependency can take traffic.
type check func(ctx context.Context) error

// tcpCheck passes when something accepts TCP connections on addr.
func tcpCheck(addr string) check {
	return func(ctx context.Context) error {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return err
		}
		return conn.Close()
	}
}

func main() {
	dbAddr := os.Getenv("DB_ADDR")
	if dbAddr == "" {
		dbAddr = "localhost:5432"
	}
	checks := map[string]check{
		"database": tcpCheck(dbAddr),
	}

	app := zinc.New()

	// Liveness: answers 204 as long as the process can serve requests.
	app.Use(healthcheck.New(healthcheck.Config{Path: "/live"}))

	// Readiness: 200 only when every dependency answers in time.
	app.Get("/ready", func(c *zinc.Context) error {
		ctx, cancel := context.WithTimeout(c.Context(), 500*time.Millisecond)
		defer cancel()

		results := zinc.Map{}
		ready := true
		for name, fn := range checks {
			if err := fn(ctx); err != nil {
				results[name] = "down"
				ready = false
				continue
			}
			results[name] = "up"
		}

		if !ready {
			return c.Status(zinc.StatusServiceUnavailable).JSON(zinc.Map{
				"status": "unavailable",
				"checks": results,
			})
		}
		return c.JSON(zinc.Map{"status": "ready", "checks": results})
	})

	log.Fatal(app.Listen(":8080"))
}
```

The database check only opens a TCP connection, so the program runs without a database driver. Set `DB_ADDR` to point it somewhere other than `localhost:5432`.

## Try it

With nothing listening on port 5432, the process is alive but not ready:

```bash
curl -i http://localhost:8080/live
# HTTP/1.1 204 No Content

curl -i http://localhost:8080/ready
# HTTP/1.1 503 Service Unavailable
# Content-Type: application/json; charset=utf-8
#
# {"checks":{"database":"down"},"status":"unavailable"}
```

Start something on that port in another terminal, such as `nc -lk 5432`, and ask again:

```bash
curl -i http://localhost:8080/ready
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
#
# {"checks":{"database":"up"},"status":"ready"}
```

## How it works

- `healthcheck.New(healthcheck.Config{Path: "/live"})` answers `GET` and `HEAD` on `/live` with `204` before routing runs. Other methods fall through to the router, so `POST /live` is a `404`.
- `/ready` runs every check in `checks` against one shared 500 ms deadline, so a hung dependency can't hold the probe open.
- Any failed check turns the answer into `503` with the name of the dependency that's down. Probes only read the status code, while the body helps a person debugging it.
- `check` has the same shape as `(*sql.DB).PingContext`, so an actual database fits in without changing the handler.

## Before production

- Use your own database or service client for each check. With `database/sql`, that's `checks["database"] = db.PingContext`.
- Point the liveness probe at `/live` and the readiness probe at `/ready`. A failing liveness probe restarts the process; a failing readiness probe only stops traffic.
- List only dependencies you can't serve without. An optional cache being down shouldn't take every instance out of rotation.
- If one health endpoint is enough, `healthcheck.Config` also takes a `Check` function: when it returns an error, the endpoint answers `503`.

## See also

- [Health Check](/middleware/healthcheck/): the `/live` endpoint's options.
- [Graceful Shutdown](/cookbook/graceful-shutdown/): let in-flight requests finish when the orchestrator stops you.
- [Docker and Environment Config](/cookbook/docker-config/): read settings such as `DB_ADDR` with defaults.
