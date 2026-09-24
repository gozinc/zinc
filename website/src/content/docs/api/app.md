---
title: Application
description: Reference for zinc.App, covering construction, routing, middleware, files, error routes, introspection, and server lifecycle.
---

`*zinc.App` is the application. It registers routes and middleware, and it is an `http.Handler`, so it runs on any `http.Server`.

```go
app := zinc.New()                     // zinc.DefaultConfig

cfg := zinc.DefaultConfig
cfg.StrictRouting = true
app = zinc.NewWithConfig(cfg)         // custom configuration
```

## Routes

| Method | Registers |
|---|---|
| `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect`, `Trace` `(path, handlers...)` | A route for that method |
| `Add(method, path, handlers...)` | A route for any method, including custom ones |
| `Match(methods, path, handlers...)` | The same chain for several methods |
| `All(path, handlers...)`, `Any` | The same chain for every standard method |
| `Handle(spec RouteSpec)` | A route described by a struct, optionally named. Panics on an invalid spec. |
| `TryHandle(spec RouteSpec) error` | The same, returning an error instead of panicking |
| `HandleHTTP(pattern, http.Handler)` | A standard handler, with a `"METHOD /path"` pattern |

Every method accepts a chain: middleware first, then the handler. Invalid or conflicting patterns panic at registration. See [Routing](/guide/routing/).

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
| `RoutesByMethod(method) []RouteInfo` | Routes for one method |
| `RoutesByPrefix(prefix) []RouteInfo` | Routes below a prefix |
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
| `ListenTLS(addr, certFile, keyFile) error` | Serve HTTPS |
| `Serve(net.Listener) error` | Serve on a listener you created |
| `Shutdown(ctx) error` | Stop accepting connections, wait for in-flight requests, and release disk-static roots |
| `Close() error` | Stop the active server and release disk-static roots immediately |
| `ServeHTTP(w, r)`, `Handler()` | Use the app as an `http.Handler` |

`Listen`, `ListenTLS`, and `Serve` apply the timeouts from [configuration](/guide/configuration/#server). The [Graceful Shutdown](/cookbook/graceful-shutdown/) recipe shows `Shutdown` in a complete program.

## Adapters

`AcquireContext(w, r) *Context` and `ReleaseContext(c)` create and recycle a context outside normal dispatch. They exist for adapters and low-level tests; applications do not need them.

## Related

- [Group](/api/group/) has the same routing methods, scoped to a prefix.
- [Context](/api/context/) is what every handler receives.
- [pkg.go.dev](https://pkg.go.dev/github.com/0mjs/zinc#App) has the generated reference.
