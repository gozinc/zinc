---
title: Groups and Middleware
description: Write middleware, choose where it applies, and understand the exact order in which Zinc runs it.
---

Middleware is code that runs around your handlers: logging, authentication, timeouts, headers. In Zinc, middleware has the same signature as a handler and calls `c.Next()` to continue.

```go
func timing(c *zinc.Context) error {
	start := time.Now()
	err := c.Next() // run everything after this middleware
	slog.Info("request", "path", c.FullPath(), "took", time.Since(start))
	return err
}
```

Code before `c.Next()` runs on the way in, and code after it runs on the way out. To stop the request, return without calling `c.Next()`.

## Where middleware applies

Attach middleware at the narrowest scope that fits:

| Scope | Register with | Runs for |
|---|---|---|
| Whole app | `app.Use(mw...)` | Every request, including 404s |
| Path prefix | `app.UsePrefix("/admin", mw...)` | Every request under the prefix, before routing |
| Group | `app.Group("/api", mw...)` or `group.Use(mw...)` | Routes registered on the group and its subgroups |
| One route | `app.Get("/x", mw, handler)` | That route only |
| Standard middleware, whole app | `app.UseHTTP(func(http.Handler) http.Handler)` | Every request, outside all Zinc middleware |
| Standard middleware, group or route | `group.UseHTTP(mw)` or `zinc.FromHTTP(mw)` | That group or route, in Zinc middleware order |

```go
app.Use(requestid.New(), logger.New(), recover.New())

api := app.Group("/api", requireAPIKey)
api.Get("/users/{id}", showUser)
api.Post("/exports", bodylimit.New(bodylimit.Config{Limit: bodylimit.MB}), startExport)
```

## Execution order

For a request to `POST /api/exports` in the example above, Zinc runs:

```text
UseHTTP middleware            (standard net/http, outermost)
  app.Use: RequestID → RequestLogger → Recover
    app.UsePrefix middleware   (for matching prefixes)
      routing
        group: requireAPIKey
          route: BodyLimit
            handler: startExport
```

Within each level, middleware runs in the order you registered it. Nested groups add their middleware after their parent's.

Register a group's middleware before its routes. Each route, mount, static directory, and child group captures the group's middleware when it is registered, so `group.Use` panics once any of them exist. Otherwise, an authentication middleware added at the end of a group would silently skip the routes above it. `app.Use` has no such restriction, because global middleware runs on every request.

## Stopping early

Return an error, or write a response, instead of calling `c.Next()`:

```go
func requireAPIKey(c *zinc.Context) error {
	if !keys.Valid(c.Header("X-API-Key")) {
		return zinc.ErrUnauthorized // the chain stops; the error handler responds
	}
	return c.Next()
}
```

Everything after this middleware is skipped. Middleware that already ran still finishes its "after" code and sees the returned error. The built-in [Request Logger](/middleware/request-logger/) sends that error through your error handler before logging, so it records the `401`. [Custom Middleware](/cookbook/middleware/) shows how your own middleware can do the same.

## Configurable middleware

Return a closure to make middleware configurable:

```go
func requireRole(roles ...string) zinc.Middleware {
	return func(c *zinc.Context) error {
		user, ok := c.Get("user")
		if !ok {
			return zinc.ErrUnauthorized
		}
		if !user.(*User).HasAnyRole(roles...) {
			return zinc.ErrForbidden
		}
		return c.Next()
	}
}

admin := app.Group("/admin", loadUser, requireRole("admin"))
admin.Delete("/users/{id}", deleteUser)

app.Post("/posts", loadUser, requireRole("editor", "admin"), createPost)
```

`zinc.Middleware` is an alias for `zinc.HandlerFunc`, so middleware and handlers mix freely in any chain.

## Groups

Groups share a prefix and a middleware chain, and nest:

```go
api := app.Group("/api", requireAPIKey)
v1 := api.Group("/v1", setVersionHeader("1"))

v1.Get("/users", listUsers) // GET /api/v1/users: requireAPIKey → setVersionHeader → listUsers
```

Groups support everything the app does: every route method, `Handle`, `HandleHTTP`, `Mount`, `Static`, and a group-scoped `RouteNotFound`.

:::note[Group middleware needs a route]
Group middleware runs only when a route in the group matches. For behavior that must also cover unmatched paths under a prefix, such as authentication for everything below `/admin`, use `app.UsePrefix`.
:::

## Skip middleware for some requests

`zinc.Skip` runs a middleware unless its predicate reports true, in which case the chain continues without it:

```go
app.Use(zinc.Skip(func(c *zinc.Context) bool {
	return c.Path() == "/healthz"
}, logger.New()))
```

It works with any middleware, yours or Zinc's, so none of them needs its own skip option.

## First-party middleware

Zinc ships 27 middleware packages under `github.com/0mjs/zinc/middleware`, each with a `New` function. A typical API starts with:

```go
import (
	"github.com/0mjs/zinc/middleware/logger"
	"github.com/0mjs/zinc/middleware/recover"
	"github.com/0mjs/zinc/middleware/requestid"
	"github.com/0mjs/zinc/middleware/secure"
)

app.Use(
	requestid.New(),
	logger.New(),
	recover.New(),
	secure.New(),
)
```

Browse them all in the [Middleware overview](/middleware/overview/).

## Next steps

- [Custom Middleware](/cookbook/middleware/) measures status and response size.
- [Zinc and net/http](/guide/http-interoperability/) covers standard `func(http.Handler) http.Handler` middleware.
- [Errors](/guide/errors/) explains what happens to returned errors.
