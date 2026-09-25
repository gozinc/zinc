# Zinc

[![Release](https://img.shields.io/github/v/release/0mjs/zinc?style=flat-square)](https://github.com/0mjs/zinc/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/0mjs/zinc.svg)](https://pkg.go.dev/github.com/0mjs/zinc)
[![Go Report Card](https://goreportcard.com/badge/github.com/0mjs/zinc?style=flat-square)](https://goreportcard.com/report/github.com/0mjs/zinc)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](./LICENSE)

A high-performance application layer for `net/http`.

Zinc adds fast routing, request binding, structured errors, response helpers, and production middleware to Go's standard HTTP stack. It does not replace that stack: a Zinc app is an `http.Handler`, and handlers always have access to the original request and response writer.

[Documentation](https://zinc.carbonsoft.sh) · [Quick start](https://zinc.carbonsoft.sh/getting-started/quick-start) · [Middleware](https://zinc.carbonsoft.sh/middleware) · [API reference](https://pkg.go.dev/github.com/0mjs/zinc)

## Install

```sh
go get github.com/0mjs/zinc
```

Zinc requires Go 1.25 or newer.

## Start a server

```go
package main

import (
	"log"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()

	app.Get("/", func(c *zinc.Context) error {
		return c.String("Hello from Zinc!")
	})

	log.Fatal(app.Listen(":8080"))
}
```

Run it, then make a request:

```sh
curl http://localhost:8080
```

```text
Hello from Zinc!
```

## Why Zinc?

The standard library has excellent HTTP contracts. Building an application directly on top of them still leaves a fair amount of repetitive work.

Zinc fills that gap without introducing another HTTP engine:

- concise handlers with central error handling
- fast routing, route groups, and middleware chains
- binding and validation for request data
- helpers for JSON, files, streams, templates, and redirects
- direct access to `http.Request`, `http.ResponseWriter`, `http.Handler`, and `http.Server`

The aim is simple application code that still behaves like ordinary Go.

## Still `net/http`

`App` implements `http.Handler`, so you can use it with a server you own:

```go
server := &http.Server{
	Addr:              ":8080",
	Handler:           app,
	ReadHeaderTimeout: 5 * time.Second,
}

log.Fatal(server.ListenAndServe())
```

Standard handlers can own individual routes:

```go
app.HandleHTTP("GET /metrics", promhttp.Handler())

app.HandleHTTP("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, r.PathValue("id"))
}))
```

Standard middleware can wrap the entire application:

```go
app.UseHTTP(requestTracing, authenticateRequest)
```

Handlers that own a whole subtree can be mounted:

```go
app.Mount("/legacy", legacyMux) // receives paths with "/legacy" removed
```

Inside a Zinc handler, the standard request and writer are available when you need them:

```go
app.Get("/request", func(c *zinc.Context) error {
	r := c.Request()
	w := c.Writer()

	w.Header().Set("X-Method", r.Method)
	return c.String("ok")
})
```

This keeps standard middleware, observability tools, test helpers, server configuration, cancellation, and request-scoped values usable.

## Routing and middleware

Routes can be grouped and middleware can be applied to the whole app or one part of it:

Every route method accepts typed `zinc.HandlerFunc` handlers. Zinc does not use untyped response shorthands.

```go
app.Use(middleware.RequestID())
app.Use(middleware.Recover())

api := app.Group("/api", requireAuth)

api.Get("/health", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"status": "ok"})
})
```

Routes declared in source fail fast if a pattern is invalid or conflicts with another route. Use `app.TryHandle(spec)` when route definitions come from configuration or plugins and need ordinary error handling.

Zinc includes middleware for common concerns such as recovery, request IDs, logging, CORS, compression, authentication, rate limiting, security headers, metrics, and tracing. See the [middleware documentation](https://zinc.carbonsoft.sh/middleware) for configuration and examples.

## Binding and errors

Handlers return errors. Zinc sends successful responses through the context and passes failures to one central error handler.

```go
type CreateUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

app.Post("/users", func(c *zinc.Context) error {
	var input CreateUser
	if err := c.Bind().JSON(&input); err != nil {
		return zinc.ErrBadRequest.WithMessage("invalid request").WithCause(err)
	}

	return c.Status(http.StatusCreated).JSON(input)
})
```

`*zinc.Context` is pooled and lives only for the active request. Extract values before starting background work; use `c.Request().Context()` when that work should share request cancellation, or deliberately use `context.WithoutCancel` when it must not.

Binding supports path, query, header, JSON, XML, YAML, TOML, form, and multipart input. The binder, validator, JSON codec, renderer, and error handler can all be replaced through `zinc.Config`.

## Performance

Zinc keeps benchmarks in the repository so performance claims can be checked against the code that produced them.

On the Apple M1 Pro comparison, Zinc has the lowest median latency in 60 of 77 comparable scenarios against Gin, Echo, and Chi. Zinc was measured on 25 September 2026 at commit `42e11d2`; rival samples are reused from 24 September, so close results merit a fresh paired run. Correct response-header ownership adds one 16-byte allocation to common string-response paths. This score is not a release gate.

Results vary by workload and machine. See the [full benchmark report](./BENCHMARKS.md) for the environment, command, scorecard, and complete results.

## Packages

| Package | Purpose |
| --- | --- |
| `github.com/0mjs/zinc` | Application, router, context, binding, responses, rendering, and static files |
| `github.com/0mjs/zinc/middleware` | First-party HTTP middleware |

## Project status

Zinc is pre-1.0. Pin a release and read the [Zinc 0.2 migration guide](https://zinc.carbonsoft.sh/extra/migration-0.2) and [release notes](https://github.com/0mjs/zinc/releases) when upgrading.

Bug reports and focused proposals are welcome in [GitHub Issues](https://github.com/0mjs/zinc/issues). See [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request.

## License

[MIT](./LICENSE)
