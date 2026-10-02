---
title: Groups and Middleware
description: Write middleware, choose which requests it runs for, and see the order Zinc runs it in.
---

Middleware lets you run the same code around many handlers: logging, authentication, timeouts, headers. Use it when something should happen on every request, or on every request under a path, without repeating it in each handler.

```go
func timing(c *zinc.Context) error {
	start := time.Now()
	err := c.Next() // run everything after this middleware
	slog.Info("request", "path", c.FullPath(), "took", time.Since(start))
	return err
}

app.Use(timing)
app.Get("/users/{id}", showUser)
```

```text
level=INFO msg=request path=/users/{id} took=2.625µs
```

Middleware has the same signature as a handler. Code before `c.Next()` runs on the way in, and code after it runs on the way out. To stop the request, return without calling `c.Next()`.

## Choose where middleware runs

Attach middleware at the narrowest scope that fits:

| Scope | Register with | Runs for |
|---|---|---|
| Whole app | `app.Use(mw...)` | Every request, including 404s |
| Path prefix | `app.UsePrefix("/admin", mw...)` | Every request under the prefix, before routing, including 404s |
| Group | `app.Group("/api", mw...)` or `group.Use(mw...)` | Routes on the group and its subgroups |
| One route | `app.Get("/x", mw, handler)` | That route only |

```go
app.Use(requestid.New(), logger.New(), recover.New())

api := app.Group("/api", requireAPIKey)
api.Get("/users/{id}", showUser)
api.Post("/exports", bodylimit.New(bodylimit.Config{Limit: bodylimit.MB}), startExport)
```

Standard `func(http.Handler) http.Handler` middleware works too, through `app.UseHTTP`, `group.UseHTTP` and `zinc.FromHTTP`. [Zinc and net/http](/guide/http-interoperability/) covers those.

## Group routes

A group gives routes a shared path prefix and shared middleware. Groups nest, and a child runs its parent's middleware first:

```go
api := app.Group("/api", requireAPIKey)
v1 := api.Group("/v1", setVersionHeader("1")) // setVersionHeader: your middleware, sets Api-Version

v1.Get("/users", listUsers) // GET /api/v1/users: requireAPIKey → setVersionHeader → listUsers
```

```bash
curl -i -H 'X-API-Key: secret' http://localhost:8080/api/v1/users
# HTTP/1.1 200 OK
# Api-Version: 1
# Content-Type: application/json; charset=utf-8
#
# []
```

A group has everything the app has for registering routes: every method, `Handle`, `HandleHTTP`, `Mount`, `Static`, and its own `RouteNotFound`.

:::caution[Add a group's middleware before its routes]
Each route, mount, static directory and child group uses the middleware added to the group so far. Calling `group.Use` after any of them panics at startup:

```text
zinc: Use on group "/admin" after route GET /admin/users; register group middleware before its routes and child groups
```

Without the panic, an auth middleware added at the end of a group would quietly skip the routes above it. `app.Use` can be called at any time, because it runs for every request.
:::

## Protect everything under a path

Group middleware runs only when a route in the group matches. A request for a path the group doesn't have gets a plain 404, without your middleware running. When a whole path must be covered, such as authentication for everything under `/admin`, use `app.UsePrefix`:

```go
app.UsePrefix("/admin", requireAPIKey)
app.Get("/admin/stats", showStats)
```

```bash
curl http://localhost:8080/admin/missing
# {"error":{"status":401,"message":"Unauthorized"}}

curl -H 'X-API-Key: secret' http://localhost:8080/admin/missing
# {"error":{"status":404,"message":"Not Found"}}

curl http://localhost:8080/administrator
# {"error":{"status":404,"message":"Not Found"}}
```

The prefix matches whole path segments, so `/admin` covers `/admin` and `/admin/stats` but not `/administrator`. It follows the app's case setting: `/ADMIN/stats` is covered too, unless you set `CaseSensitive: true`.

## Execution order

Here's everything together, for a `POST /api/v1/exports`:

```go
app.UseHTTP(otelMiddleware)                           // standard net/http middleware, such as OpenTelemetry
app.Use(requestid.New(), logger.New(), recover.New())
app.UsePrefix("/api", rateLimit)                      // for example limiter.New(...)

api := app.Group("/api", requireAPIKey)
v1 := api.Group("/v1", setVersionHeader("1"))
v1.Post("/exports", bodylimit.New(), startExport)
```

```text
UseHTTP: otelMiddleware                 standard net/http, outermost
  app.Use: requestid → logger → recover
    UsePrefix: rateLimit                before routing
      routing
        group: requireAPIKey
          subgroup: setVersionHeader
            route: bodylimit
              handler: startExport
```

Within each level, middleware runs in the order you registered it. On the way out, the same list runs in reverse.

## Where a request goes

1. **App middleware** (`app.Use`) and **prefix middleware** (`app.UsePrefix`) run for every request, before routing, so they see misses too.
2. **Routing** picks the route for the method and path.
3. **The route's chain** runs: its groups' middleware, outer groups first, then the route's own middleware and handler. A route's `.Status` is set before the chain starts.
4. **The error handler** turns a returned error into the response, once.

When no route matches, step 3 is replaced by one of these, in order: a mount; the built-in [spec and docs page](/guide/openapi/#serve-the-spec); a `RouteNotFound` handler; the automatic `OPTIONS` answer, which runs the matching route's [CORS middleware](/middleware/cors/#on-a-group) for preflights; a `405` with `Allow`; or a `404`. Each still runs inside step 1, so app middleware sees every request.

## Stop a request early

Return an error instead of calling `c.Next()`:

```go
func requireAPIKey(c *zinc.Context) error {
	if c.Header("X-API-Key") != "secret" { // check against your key store
		return zinc.ErrUnauthorized // the chain stops; the error handler responds
	}
	return c.Next()
}
```

```bash
curl http://localhost:8080/api/users/42
# {"error":{"status":401,"message":"Unauthorized"}}
```

Everything after this middleware is skipped. Middleware that already ran still runs its "after" code, and gets the error back from `c.Next()`.

The built-in [logger](/middleware/logger/) sends that error through your error handler before it logs, so the log line shows the 401:

```text
level=INFO msg=REQUEST method=GET uri=/api/users/42 route=/api/users/{id} status=401 latency=121.792µs host=example.com bytes_in="" bytes_out=50 user_agent="" remote_ip=192.0.2.1 request_id="" error=Unauthorized
```

[Custom Middleware](/cookbook/middleware/) shows how your own middleware can do the same.

## Make middleware configurable

Return a closure to give middleware options:

```go
func requireRole(roles ...string) zinc.Middleware {
	return func(c *zinc.Context) error {
		user, ok := zinc.Value[*User](c, "user") // set earlier by loadUser, your auth middleware
		if !ok {
			return zinc.ErrUnauthorized
		}
		if !user.HasAnyRole(roles...) { // User is your type
			return zinc.ErrForbidden
		}
		return c.Next()
	}
}

admin := app.Group("/admin", loadUser, requireRole("admin"))
admin.Delete("/users/{id}", deleteUser)

app.Post("/posts", loadUser, requireRole("editor", "admin"), createPost)
```

```bash
curl -X DELETE http://localhost:8080/admin/users/1                        # no user
# {"error":{"status":401,"message":"Unauthorized"}}
curl -X DELETE -H 'X-Role: editor' http://localhost:8080/admin/users/1    # wrong role
# {"error":{"status":403,"message":"Forbidden"}}
```

`zinc.Middleware` is another name for `zinc.HandlerFunc`, so middleware and handlers mix freely in any chain.

## Skip middleware for some requests

`zinc.Skip` wraps a middleware and leaves it out whenever your function returns true:

```go
app.Use(zinc.Skip(func(c *zinc.Context) bool {
	return c.Path() == "/healthz"
}, logger.New()))
```

Requests to `/healthz` now leave no log line, and every other request is logged. `Skip` works with any middleware, yours or Zinc's, so none of them needs its own skip option.

## Use the built-in middleware

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

## Good to know

### Several matching prefixes

If more than one `UsePrefix` matches a request, each runs once, in the order you registered them. A shorter prefix registered later still runs after a longer one registered first.

### 404s and app middleware

`app.Use` and `app.UsePrefix` middleware wrap routing, so they also run for 404 and 405 responses. There, `c.Next()` returns `zinc.ErrNotFound` or `zinc.ErrMethodNotAllowed`, and `c.FullPath()` is empty because no route matched. Return the error to send the usual response, or write your own instead.

## Next steps

- [Custom Middleware](/cookbook/middleware/): record status and response size.
- [Zinc and net/http](/guide/http-interoperability/): use standard `func(http.Handler) http.Handler` middleware.
- [Errors](/guide/errors/): what happens to the errors middleware returns.
