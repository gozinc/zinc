---
title: Reverse Proxy
description: Forward everything under /api to another service, with the prefix removed and timeouts on the upstream connection.
---

This program forwards every request under `/api` to another service and serves the rest itself. You'd use it to put one public address in front of several backends, or to move endpoints to a new service one prefix at a time. Zinc stays in front, so middleware such as logging, auth and rate limits still runs for proxied requests.

## The program

The program starts a small upstream on port 9000 so you can try it without another service. Point `Target` at your own service instead.

```go title="main.go"
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/proxy"
)

// upstream stands in for the service you're proxying to.
// It echoes the path and forwarding headers it receives.
func upstream() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "upstream got %s %s\n", r.Method, r.URL.RequestURI())
		fmt.Fprintf(w, "X-Forwarded-Host: %s\n", r.Header.Get("X-Forwarded-Host"))
		fmt.Fprintf(w, "X-Forwarded-For: %s\n", r.Header.Get("X-Forwarded-For"))
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:9000", mux))
}

func main() {
	go upstream()

	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	app := zinc.New()
	app.UsePrefix("/api", proxy.New(proxy.Config{
		Target: "http://127.0.0.1:9000",
		Rewrite: map[string]string{
			"/api":   "/",
			"/api/*": "/*",
		},
		Transport: transport,
	}))

	app.Get("/", func(c *zinc.Context) error {
		return c.String("served by Zinc\n")
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

A request under `/api` reaches the upstream with the prefix removed and the query string kept:

```bash
curl 'http://localhost:8080/api/users?page=2'
# upstream got GET /users?page=2
# X-Forwarded-Host: localhost:8080
# X-Forwarded-For: ::1
```

Anything else is served by Zinc:

```bash
curl http://localhost:8080/
# served by Zinc
```

If the upstream is down (comment out `go upstream()` to see it), the client gets a `502` and the server logs the cause:

```bash
curl -i http://localhost:8080/api/users
# HTTP/1.1 502 Bad Gateway
# Content-Length: 0
```

```text
2026/09/28 01:20:05 http: proxy error: dial tcp 127.0.0.1:9000: connect: connection refused
```

## How it works

- `app.UsePrefix("/api", ...)` runs the proxy for `/api` and every path below it, but not for `/apix`. The proxy ends the chain, so no Zinc route is needed under `/api`.
- `Rewrite` changes the path before it's sent. `"/api/*": "/*"` puts whatever followed `/api/` in place of the `*`, and `"/api": "/"` covers the bare prefix. The client keeps talking to Zinc; this is not a redirect.
- The proxy sets `X-Forwarded-For`, `X-Forwarded-Host` and `X-Forwarded-Proto`, so the upstream knows who the original client was.
- `Transport` bounds the upstream connection: three seconds to connect, five to start answering. Without it, a hung upstream holds the request open until the client disconnects.

## Before production

- Use `Targets` for round-robin across several upstreams, or `Balancer` to choose one yourself.
- Use `ModifyResponse` to change the upstream's response before it reaches the client.
- Turn on `Retries` only for requests whose bodies can be replayed safely. Retrying a `POST` can run it twice upstream.
- If the upstream trusts `X-Forwarded-For`, make sure clients can't reach it directly.

## See also

- [Proxy](/middleware/proxy/): every option, including balancing and retries.
- [Zinc and net/http](/guide/http-interoperability/): mount an existing `http.Handler` instead of proxying.
- [Client IP and Proxies](/guide/ip-address/): read the client's IP when Zinc is the one behind a proxy.
