---
title: Application
description: Reference for zinc.App, covering construction, routing, middleware, files, error routes, introspection, and server lifecycle.
---

`*zinc.App` is the application. It registers routes and middleware, and it is an `http.Handler`, so it runs on any `http.Server`.

```go
app := zinc.New()                                  // defaults
app := zinc.New(zinc.Config{StrictRouting: true})   // change only what you need
```

## Routes

| Method | Registers |
|---|---|
| `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect`, `Trace` `(path, handlers...) Route` | A route for that method |
| `Add(method, path, handlers...) Route` | A route for any method, including custom ones |
| `Match(methods, path, handlers...)` | The same chain for several methods |
| `All(path, handlers...)` | The same chain for every standard method |
| `TryHandle(spec RouteSpec) error` | A route from configuration or plugins, returning every problem as an error |
| `HandleHTTP(pattern, http.Handler) Route` | A standard handler, with a `"METHOD /path"` pattern |

Every method accepts a chain: middleware first, then the handler. Invalid or conflicting patterns panic at registration. See [Routing](/guide/routing/).

The returned `Route` has two methods:

| Method | Purpose |
|---|---|
| `Name(name) Route` | A unique name for `URL` and `RouteByName`; panics on a duplicate |
| `Status(code) Route` | The success status of a [typed handler](/guide/typed-handlers/), such as `201`; panics unless `code` is 2xx |

```go
app.Get("/users/{id}", showUser).Name("users.show")
app.Post("/users", zinc.Typed(createUser)).Status(zinc.StatusCreated)
```

## Typed handlers

| Function | Returns |
|---|---|
| `zinc.Typed[In, Out](fn func(*Context, In) (Out, error)) HandlerFunc` | A handler that binds and validates `In`, calls `fn`, and writes `Out` as JSON |
| `zinc.NoContent` | The `Out` type for a response without a body: `204` unless another status is declared |

See [Typed Handlers](/guide/typed-handlers/).

`TryHandle` takes a `RouteSpec`:

```go
type RouteSpec struct {
	Name    string      // optional; enables URL generation
	Method  string
	Path    string
	Handler HandlerFunc
}
```

## Groups and middleware

| Method | Purpose |
|---|---|
| `Group(prefix, middleware...) *Group` | Routes that share a prefix and middleware |
| `Route(prefix, fn func(*Group), middleware...) *Group` | The same, declared in a nested block |
| `Use(middleware...)` | Middleware for every request |
| `UsePrefix(prefix, middleware...)` | Middleware for requests under a prefix, before routing |
| `UseHTTP(func(http.Handler) http.Handler...)` | Standard middleware around the whole app |
| `Mount(prefix, http.Handler)` | A handler that owns a subtree; receives paths without the prefix |

See [Groups and Middleware](/guide/groups-and-middleware/) for execution order.

## Files

| Method | Serves |
|---|---|
| `Static(prefix, dir, opts...) error` | A directory from disk |
| `StaticFS(prefix, fs.FS, opts...) error` | A directory from any filesystem, such as `embed.FS` |
| `File(path, file) error` | One file from disk |
| `FileFS(path, name, fs.FS) error` | One file from a filesystem |

Options: `zinc.WithStaticIndex(name)` and `zinc.WithStaticBrowse(bool)`. See [Static Files](/guide/static-files/).

## Error routes

| Method | Replaces |
|---|---|
| `NotFound(handler)` | The app-wide `404` response |
| `MethodNotAllowed(handler)` | The app-wide `405` response |
| `RouteNotFound(pattern, handlers...)` | The `404` response below a prefix, such as `/api/{tail...}` |

## Introspection

| Method | Returns |
|---|---|
| `Routes() []RouteInfo` | Every route and mount, in registration order |
| `FindRoute(method, path) (RouteInfo, bool)` | The route that would serve a request |
| `RouteByName(name) (RouteInfo, bool)` | A named route |
| `URL(name, params...) (string, error)` | The path for a named route, with parameters filled in order. Segment values are escaped; a catch-all value is inserted as given. |

```go
type RouteInfo struct {
	Name    string
	Method  string
	Path    string   // the registered pattern
	Params  []string // parameter names, in order
	Mounted bool     // true for Mount, Static, and StaticFS
	Handler string   // the handler's function name
}
```

## Server lifecycle

| Method | Purpose |
|---|---|
| `Listen(addr...) error` | Serve HTTP; the address defaults to `:8080` |
| `ListenContext(ctx, addr) error` | Serve HTTP until `ctx` ends, then shut down gracefully within `Config.ShutdownTimeout` |
| `ListenTLS(addr, certFile, keyFile) error` | Serve HTTPS |
| `Serve(net.Listener) error` | Serve on a listener you created |
| `Shutdown(ctx) error` | Stop accepting connections, wait for in-flight requests, and release disk-static roots |
| `Close() error` | Stop the active server and release disk-static roots immediately |
| `ServeHTTP(w, r)`, `Handler()` | Use the app as an `http.Handler` |

`Listen`, `ListenContext`, `ListenTLS`, and `Serve` apply the timeouts from [configuration](/guide/configuration/#server). The [Graceful Shutdown](/cookbook/graceful-shutdown/) recipe shows `ListenContext` in a complete program. `Shutdown` remains for servers started with `Listen` or `Serve`.

## Adapters

`AcquireContext(w, r) *Context` and `ReleaseContext(c)` create and recycle a context outside normal dispatch. They exist for adapters and low-level tests; applications do not need them.

## Related

- [Group](/api/group/) has the same routing methods, scoped to a prefix.
- [Context](/api/context/) is what every handler receives.
- [pkg.go.dev](https://pkg.go.dev/github.com/0mjs/zinc#App) has the generated reference.
