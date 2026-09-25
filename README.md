# Zinc

[![Release](https://img.shields.io/github/v/release/0mjs/zinc?style=flat-square)](https://github.com/0mjs/zinc/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/0mjs/zinc.svg)](https://pkg.go.dev/github.com/0mjs/zinc)
[![Go Report Card](https://goreportcard.com/badge/github.com/0mjs/zinc?style=flat-square)](https://goreportcard.com/report/github.com/0mjs/zinc)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](./LICENSE)

**Galvanize `net/http`.**

Zinc is a fast, thin application layer for Go's standard HTTP stack. It adds routing, binding, central error handling, response helpers, and production middleware. A Zinc app is still an `http.Handler`, so everything you already use with `net/http` keeps working.

[Documentation](https://zinc.carbonsoft.sh) · [Quick start](https://zinc.carbonsoft.sh/guide/quickstart/) · [Middleware](https://zinc.carbonsoft.sh/middleware/overview/) · [API reference](https://pkg.go.dev/github.com/0mjs/zinc)

## Install

```sh
go get github.com/0mjs/zinc
```

Zinc requires Go 1.25 or newer.

## A small API

```go
package main

import (
	"log"
	"net/http"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var users = map[string]User{"42": {ID: "42", Name: "Ada"}}

func main() {
	app := zinc.New()
	app.Use(middleware.Recover(), middleware.RequestID())

	api := app.Group("/api")

	api.Get("/users/{id}", func(c *zinc.Context) error {
		user, ok := users[c.Param("id")]
		if !ok {
			return zinc.ErrNotFound
		}
		return c.JSON(user)
	})

	api.Post("/users", func(c *zinc.Context) error {
		var user User
		if err := c.Bind().JSON(&user); err != nil {
			return zinc.ErrBadRequest.WithMessage("invalid user").WithCause(err)
		}
		users[user.ID] = user
		return c.Status(http.StatusCreated).JSON(user)
	})

	log.Fatal(app.Listen(":8080"))
}
```

```sh
curl localhost:8080/api/users/42
# {"id":"42","name":"Ada"}
```

Handlers return errors, and one error handler turns them into responses. Invalid or conflicting routes fail at startup rather than at request time.

## What you get

- **Routing:** a radix router with groups, parameters, catch-alls, and clear precedence: static, then parameter, then catch-all.
- **Binding:** path, query, header, form, multipart, JSON, XML, YAML, and TOML input, with an optional validator.
- **Responses:** JSON, text, files, streams, templates, and redirects.
- **Errors:** typed HTTP errors with safe client messages and wrapped causes.
- **Middleware:** security, observability, limits, and transport, listed below.
- **Replaceable parts:** swap the binder, validator, JSON codec, renderer, or error handler through `zinc.Config`.

## Still `net/http`

Zinc wraps the standard library rather than replacing it:

```go
// Run it on a server you configure.
server := &http.Server{Addr: ":8080", Handler: app, ReadHeaderTimeout: 5 * time.Second}

// Serve a route with any standard handler.
app.HandleHTTP("GET /metrics", promhttp.Handler())

// Wrap the app in standard middleware.
app.UseHTTP(otelhttp.NewMiddleware("api"))

// Hand a whole subtree to an existing handler. The prefix is stripped.
app.Mount("/legacy", legacyMux)
```

Inside a handler, `c.Request()` and `c.Writer()` give you the underlying request and response writer.

## Middleware

Import from `github.com/0mjs/zinc/middleware`.

| Job | Middleware |
| --- | --- |
| Protect | Basic Auth, Casbin, CORS, CSRF, Header Guards, JWT, Key Auth, Secure Headers, Session |
| Observe | Body Dump, Jaeger, OpenTelemetry, pprof, Prometheus, Request ID, Request Logger |
| Control | Body Limit, Context Timeout, Rate Limiter, Recover, Utility |
| Transport | Decompress, Gzip, Method Override, Proxy, Redirect, Rewrite, Static, Trailing Slash |

OpenTelemetry uses the standard `otelhttp` package through `UseHTTP`. The [middleware docs](https://zinc.carbonsoft.sh/middleware/overview/) cover configuration for each.

## Performance

In the recorded comparison against Gin, Echo, and Chi, Zinc had the lowest latency in 62 of 77 workloads. That run used commit `5f77c75` on an Apple M1 Pro and predates the 0.3 hardening work. The [benchmark report](./BENCHMARKS.md) has the full results, environment, and commands. Results vary by workload and machine, so run the suite against the revision you deploy.

On the Apple M1 Pro comparison, Zinc has the lowest median latency in 60 of 77 comparable scenarios against Gin, Echo, and Chi. Zinc was measured on 25 September 2026 at commit `42e11d2`; rival samples are reused from 24 September, so close results merit a fresh paired run. Correct response-header ownership adds one 16-byte allocation to common string-response paths. This score is not a release gate.

## Good to know

- `*zinc.Context` is pooled and valid only during its request. Copy values out before starting background work. Use `c.Request().Context()` when that work should be cancelled with the request.
- Use `app.TryHandle(spec)` for routes that come from configuration or plugins. It returns an error instead of panicking.

## Project status

Zinc is pre-1.0. Pin a release, and check the [release notes](https://github.com/0mjs/zinc/releases) and the [0.2 migration guide](https://zinc.carbonsoft.sh/extra/migration-0.2/) when you upgrade.

Bug reports and focused proposals are welcome in [GitHub Issues](https://github.com/0mjs/zinc/issues). Read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request.

## License

[MIT](./LICENSE)
