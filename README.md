# Zinc

[![Release](https://img.shields.io/github/v/release/0mjs/zinc?style=flat-square)](https://github.com/0mjs/zinc/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/0mjs/zinc.svg)](https://pkg.go.dev/github.com/0mjs/zinc)
[![Go Report Card](https://goreportcard.com/badge/github.com/0mjs/zinc?style=flat-square)](https://goreportcard.com/report/github.com/0mjs/zinc)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](./LICENSE)
[![CodSpeed](https://img.shields.io/endpoint?url=https://codspeed.io/badge.json)](https://app.codspeed.io/0mjs/zinc?utm_source=badge)

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
	"github.com/0mjs/zinc/middleware/recover"
	"github.com/0mjs/zinc/middleware/requestid"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var users = map[string]User{"42": {ID: "42", Name: "Ada"}}

func main() {
	app := zinc.New()
	app.Use(recover.New(), requestid.New())

	api := app.Group("/api")

	api.Get("/users/{id}", func(c *zinc.Context) error {
		user, ok := users[c.Param("id")]
		if !ok {
			return zinc.NotFound("user not found")
		}
		return c.JSON(user)
	})

	api.Post("/users", func(c *zinc.Context) error {
		var user User
		if err := c.Bind().JSON(&user); err != nil {
			return err // 400 with the failing field
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

curl localhost:8080/api/users/7
# {"error":{"status":404,"message":"user not found"}}
```

Handlers return errors, and one error handler turns them into responses. Invalid or conflicting routes fail at startup rather than at request time.

### Typed handlers

A handler's signature can be its request contract. Zinc binds and validates the input, calls the function, and writes the output:

```go
type CreateUser struct {
	OrgID string `path:"org"`
	Email string `json:"email"`
}

api.Post("/orgs/{org}/users", zinc.Typed(func(c *zinc.Context, in CreateUser) (User, error) {
	return users.Create(c.Context(), in)
})).Status(http.StatusCreated)
```

A value that doesn't parse is a `400` naming the field, a validation failure is a `422`, and a typed handler allocates no more than the same code written by hand.

## What you get

- **Routing:** a radix router with groups, parameters, catch-alls, and clear precedence: static, then parameter, then catch-all.
- **Binding:** path, query, header, form, multipart, JSON, and XML input, with an optional validator. YAML, TOML, or a faster JSON library plug in per app with one map entry, and Zinc itself requires no other module.
- **Responses:** JSON, text, files, streams, templates, and redirects.
- **Errors:** JSON error responses by default, short constructors like `zinc.NotFound("…")`, and domain errors that choose their own status. Internal error text never reaches clients.
- **Middleware:** security, observability, limits, and transport, listed below.
- **Replaceable parts:** swap the validator, renderer, error handler, or JSON library, and add body formats, through `zinc.Config`.

## Still `net/http`

Zinc wraps the standard library rather than replacing it:

```go
// Run it on a server you configure.
server := &http.Server{Addr: ":8080", Handler: app, ReadHeaderTimeout: 5 * time.Second}

// Serve a route with any standard handler.
app.HandleHTTP("GET /metrics", promhttp.Handler())

// Wrap the app in standard middleware, or just one group.
app.UseHTTP(otelhttp.NewMiddleware("api"))
admin := app.Group("/admin").UseHTTP(basicAuth)

// Hand a whole subtree to an existing handler. The prefix is stripped.
app.Mount("/legacy", legacyMux)
```

Inside a handler, `c.Request()` and `c.Writer()` give you the underlying request and response writer.

## Middleware

Each middleware is its own package under `github.com/0mjs/zinc/middleware`, with a `New` function that takes an optional `Config`, and none adds a dependency. Middleware that needs a third-party library lives in [`github.com/0mjs/contrib`](https://github.com/0mjs/contrib).

| Family | Packages, in chain order |
| --- | --- |
| Observe | OpenTelemetry, `requestid`, `logger`, `prometheus`, `healthcheck`, `bodydump` |
| Contain | `recover`, `timeout`, `bodylimit`, `limiter` |
| Shape | `redirect`, `trailingslash`, `rewrite`, `methodoverride` |
| Guard | `secure`, `cors`, `contenttype`, `session`, `csrf`, `basicauth`, `keyauth`, `casbin`, contrib `jwtauth` |
| Carry | `decompress`, `compress`, `nocache`, `headers`, `proxy`, `pprof` |

OpenTelemetry uses the standard `otelhttp` package through `UseHTTP`. The [middleware docs](https://zinc.carbonsoft.sh/middleware/overview/) cover configuration for each.

## Performance

In the 0.4.0 release run against Gin, Echo, and Chi, Zinc had the lowest median latency in 61 of 77 workloads, with all four frameworks measured together on an Apple M1 Pro. The [benchmark report](./BENCHMARKS.md) has the full results, environment, and commands. Results vary by workload and machine, so run the suite against the revision you deploy.

## Good to know

- `*zinc.Context` is pooled and valid only during its request. Copy values out before starting background work. Use `c.Request().Context()` when that work should be cancelled with the request.
- Use `app.TryHandle(spec)` for routes that come from configuration or plugins. It returns an error instead of panicking.

## Project status

Zinc is pre-1.0. Pin a release, and check the [release notes](https://github.com/0mjs/zinc/releases) and the [0.4 migration guide](https://zinc.carbonsoft.sh/extra/migration-0.4/) when you upgrade.

Bug reports and focused proposals are welcome in [GitHub Issues](https://github.com/0mjs/zinc/issues). Read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request.

## License

[MIT](./LICENSE)
