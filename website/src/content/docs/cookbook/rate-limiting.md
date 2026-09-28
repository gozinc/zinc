---
title: Rate Limiting
description: Limit each client to a few requests per second and answer the rest with 429 Too Many Requests and a Retry-After header.
---

This program lets each client IP make a burst of three requests, then one per second after that. Anything faster gets `429 Too Many Requests` with a `Retry-After` header. You'd want this on a public API, a login form or any endpoint that's expensive to run, so one noisy client can't use up capacity meant for everyone. It uses the built-in [Rate Limiter](/middleware/limiter/) with a limit low enough to hit by hand.

## The program

```go title="main.go"
package main

import (
	"log"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/limiter"
)

func main() {
	app := zinc.New()

	app.Use(limiter.New(limiter.Config{
		Rate:     1, // one request per second, refilled steadily
		Capacity: 3, // bursts of up to three
		Key:      (*zinc.Context).IP,
		LimitReached: func(c *zinc.Context) error {
			c.SetHeader("Retry-After", "1")
			return zinc.TooManyRequests("rate limit exceeded")
		},
	}))

	app.Get("/", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"ok": true})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Send five requests in a row and print each status. The first three spend the burst; the next two are refused:

```bash
for i in 1 2 3 4 5; do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/
done
# 200
# 200
# 200
# 429
# 429
```

A refused request carries the header and a JSON error:

```bash
curl -i http://localhost:8080/
# HTTP/1.1 429 Too Many Requests
# Content-Type: application/json; charset=utf-8
# Retry-After: 1
#
# {"error":{"status":429,"message":"rate limit exceeded"}}
```

Wait a second and one more request gets through:

```bash
sleep 1; curl http://localhost:8080/
# {"ok":true}
```

## How it works

- `limiter.New` keeps a token bucket for each key. Each request spends a token; `Rate` adds tokens back per second, up to `Capacity`.
- `Key: (*zinc.Context).IP` gives each client IP its own bucket. Without `Key`, one bucket is shared by every request to the app.
- `LimitReached` answers a request that finds its bucket empty. This one adds `Retry-After` so well-behaved clients know when to try again, then returns the same `429` error the limiter uses by default.

## Before production

- Raise the numbers to fit your traffic. The [Rate Limiter](/middleware/limiter/) page's per-IP example uses `Rate: 20, Capacity: 40`.
- Behind a load balancer, configure [trusted proxies](/guide/ip-address/) first. Otherwise `c.IP()` is the load balancer's address, and every client shares one bucket.
- Buckets live in the process's memory. With several instances, each one limits separately, so a client's effective limit is multiplied by the instance count.
- For an API used with keys, limit by key instead of IP: `Key: func(c *zinc.Context) string { return c.Header("X-API-Key") }`.

## Good to know

### Limit concurrent requests instead

To cap how many requests run at once, rather than how fast they arrive, use `limiter.Concurrency`. Requests over the cap get `429` instead of waiting:

```go
app.Post("/reports", limiter.Concurrency(4), buildReport) // buildReport: your handler
```

## See also

- [Rate Limiter](/middleware/limiter/): every option, including `MaxKeys` and memory use.
- [Client IP and Proxies](/guide/ip-address/): make per-IP keys accurate behind a proxy.
- [Custom Middleware](/cookbook/middleware/): write your own policy if a token bucket doesn't fit.
