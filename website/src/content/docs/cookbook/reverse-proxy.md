---
title: Reverse Proxy
description: Forward one Zinc path prefix to an upstream service with explicit path rewriting and transport timeouts.
---

Forward everything under `/api` to another service, with timeouts on the upstream connection. Zinc stays in front, so its middleware, such as logging, auth, and rate limits, still runs for proxied requests.

```go
package main

import (
	"log"
	"net"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/proxy"
)

func main() {
	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	app := zinc.New()
	app.UsePrefix(
		"/api",
		proxy.New(proxy.Config{
			Target: "http://127.0.0.1:9000",
			Rewrite: map[string]string{
				"/api/*": "/*",
			},
			Transport: transport,
		}),
	)

	log.Fatal(app.Listen(":8080"))
}
```

A request to `/api/users` is forwarded to
`http://127.0.0.1:9000/users`. The client keeps talking to Zinc; this is an
internal rewrite, not a redirect.

Use `Targets` for round-robin upstreams, `Balancer` for custom selection,
`ModifyResponse` for response changes, and opt-in retries only for requests
whose bodies can be replayed safely. See [Proxy middleware](/middleware/proxy/)
for the complete configuration.
