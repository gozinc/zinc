---
title: Groups
description: Reference for zinc.Group, a set of routes that share a path prefix and middleware.
---

A `*zinc.Group` registers routes below a prefix and runs its middleware before theirs. Create one from the app or from another group.

Full signatures and doc comments are on [pkg.go.dev](https://pkg.go.dev/github.com/0mjs/zinc#Group).

```go
api := app.Group("/api", requireAPIKey)
v1 := api.Group("/v1")

v1.Get("/users/{id}", showUser) // GET /api/v1/users/{id}
```

## Methods

| Method | Purpose |
|---|---|
| `Use(middleware...) *Group` | Adds middleware for this group's routes and subgroups. Call it before registering them: `Use` panics once the group has routes, mounts, files, or child groups |
| `Group(prefix, middleware...) *Group` | A nested group |
| `Route(prefix, fn func(*Group), middleware...) *Group` | A nested group declared in a block |
| `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect`, `Trace` | Routes below the prefix; each returns a `Route` to `Name` |
| `Add`, `Match`, `All` | Routes for custom or multiple methods |
| `TryHandle(spec) error` | A route from configuration, returning problems as errors |
| `Tags(tags...) *Group` | [OpenAPI](/guide/openapi/) tags for every route registered in the group from now on, and its child groups. Panics once the group has routes or child groups, like `Use` |
| `Security(schemes...) *Group` | The security schemes that protect every route registered in the group from now on, in the spec; `Route.Security` overrides it |
| `UseHTTP(middleware...) *Group` | Standard `func(http.Handler) http.Handler` middleware for this group, through `zinc.FromHTTP` |
| `HandleHTTP(pattern, http.Handler) Route` | A standard handler below the prefix |
| `Mount(prefix, http.Handler)` | A handler that owns a subtree below the prefix |
| `Static`, `StaticFS`, `File`, `FileFS` | Files below the prefix |
| `RouteNotFound(pattern, handlers...)` | A `404` handler below the prefix |

The methods behave like their [Application](/api/app/) counterparts, with the group's prefix and middleware applied.

## Middleware order

Group middleware runs after app middleware and before route middleware. A nested group runs its parent's middleware first:

```go
api := app.Group("/api", authenticate)
admin := api.Group("/admin", requireAdmin)

admin.Delete("/users/{id}", audit, deleteUser)
// authenticate → requireAdmin → audit → deleteUser
```

Group middleware runs only when one of the group's routes matches. Use `app.UsePrefix` for middleware that must also run for unmatched paths under a prefix. For the same reason, [redirect](/middleware/redirect/) and [rewrite](/middleware/rewrite/) can't work on a group, and adding them to one panics.
