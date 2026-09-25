---
title: Groups
description: Reference for zinc.Group, a set of routes that share a path prefix and middleware.
---

A `*zinc.Group` registers routes below a prefix and runs its middleware before theirs. Create one from the app or from another group.

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
| `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect`, `Trace` | Routes below the prefix |
| `Add`, `Match`, `All`, `Any` | Routes for custom or multiple methods |
| `Handle(spec)`, `TryHandle(spec) error` | Routes described by a `RouteSpec` |
| `HandleHTTP(pattern, http.Handler)` | A standard handler below the prefix |
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

Group middleware runs only when one of the group's routes matches. Use `app.UsePrefix` for middleware that must also run for unmatched paths under a prefix.
