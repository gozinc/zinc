---
title: Add Zinc to an Existing net/http Service
description: Put Zinc in front of a working net/http service and add new routes without rewriting the old handlers or middleware.
---

This program keeps an existing `http.ServeMux` and its middleware running, and adds new routes with Zinc beside them. You'd use it to adopt Zinc gradually in a service that already works, moving handlers over when there's a reason to. Zinc is an `http.Handler` and accepts standard handlers and middleware, so nothing has to change on day one.

## The program

```go title="main.go"
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
)

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started))
	})
}

func main() {
	legacy := http.NewServeMux()
	legacy.HandleFunc("GET /reports", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("legacy report\n"))
	})

	app := zinc.New()

	// Standard middleware can keep wrapping the whole service.
	app.UseHTTP(accessLog)

	// Mount strips /legacy before the request reaches legacy.
	app.Mount("/legacy", legacy)

	// New endpoints use Zinc handlers.
	app.Get("/api/health", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"status": "ok"})
	})

	// A standard handler can own a single route, path values included.
	app.HandleHTTP("GET /files/{name}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("file " + r.PathValue("name") + "\n"))
	}))

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl http://localhost:8080/legacy/reports
# legacy report

curl http://localhost:8080/api/health
# {"status":"ok"}

curl http://localhost:8080/files/report.pdf
# file report.pdf
```

`accessLog` sees every request, whichever kind of handler serves it:

```text
2026/09/28 01:20:58 GET /legacy/reports 31.917µs
2026/09/28 01:20:58 GET /api/health 184.708µs
2026/09/28 01:20:58 GET /files/report.pdf 12.25µs
```

A path under `/legacy` that the old mux doesn't know gets the mux's own answer, not Zinc's JSON error:

```bash
curl -i http://localhost:8080/legacy/missing
# HTTP/1.1 404 Not Found
# Content-Type: text/plain; charset=utf-8
#
# 404 page not found
```

## How it works

- `app.UseHTTP(accessLog)` wraps the whole app in standard `func(http.Handler) http.Handler` middleware, with no adapter. It runs before the mount prefix is removed, so it logs the full path.
- `app.Mount("/legacy", legacy)` hands everything under `/legacy` to the old mux, with `/legacy` removed from the path. The mux keeps its own routing, 404s and 405s.
- `app.Get("/api/health", ...)` is an ordinary Zinc route, with binding, groups and Zinc middleware available.
- `app.HandleHTTP("GET /files/{name}", ...)` routes one pattern to a standard handler. Path values are read with `r.PathValue`, as with Go's `ServeMux`.

## Choose the smallest integration point

- `app.Mount("/prefix", handler)` when an existing handler owns a whole subtree.
- `app.HandleHTTP("METHOD /path", handler)` for one standard handler.
- `app.UseHTTP(middleware)` for standard middleware around the whole app.
- Zinc handlers for new routes that benefit from context helpers, binding, groups and Zinc middleware.

## See also

- [Zinc and net/http](/guide/http-interoperability/): every way Zinc and standard handlers fit together.
- [Groups and Middleware](/guide/groups-and-middleware/): where Zinc middleware and `UseHTTP` middleware run.
- [Reverse Proxy](/cookbook/reverse-proxy/): when the old service runs as a separate process.
