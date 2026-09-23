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
| Standard middleware | `app.UseHTTP(func(http.Handler) http.Handler)` | Every request, outside all Zinc middleware |

```go
app.Use(middleware.RequestID(), middleware.RequestLogger(), middleware.Recover())

api := app.Group("/api", requireAPIKey)
api.Get("/users/{id}", showUser)
api.Post("/exports", middleware.BodyLimit(1<<20), startExport)
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

## Stopping early

Return an error, or write a response, instead of calling `c.Next()`:

```go
func requireAPIKey(c *zinc.Context) error {
	if !keys.Valid(c.GetHeader("X-API-Key")) {
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

## First-party middleware

Zinc ships 29 middleware in `github.com/0mjs/zinc/middleware`. A typical API starts with:

```go
app.Use(
	middleware.RequestID(),
	middleware.RequestLogger(),
	middleware.Recover(),
	middleware.Secure(),
)
```

Browse them all in the [Middleware overview](/middleware/overview/).

## Next steps

- [Custom Middleware](/cookbook/middleware/) measures status and response size.
- [Zinc and net/http](/guide/http-interoperability/) covers standard `func(http.Handler) http.Handler` middleware.
- [Errors](/guide/errors/) explains what happens to returned errors.
