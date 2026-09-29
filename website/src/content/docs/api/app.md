---
title: Application
description: Reference for zinc.App, covering construction, routing, middleware, files, error routes, introspection, and server lifecycle.
---

`*zinc.App` is the application. It registers routes and middleware, and it is an `http.Handler`, so it runs on any `http.Server`.

Full signatures and doc comments are on [pkg.go.dev](https://pkg.go.dev/github.com/0mjs/zinc#App).

```go
app := zinc.New()                                  // defaults
app := zinc.New(zinc.Config{StrictRouting: true})   // change only what you need
```

## Routes

| Method | Registers |
|---|---|
| `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect`, `Trace` `(path, handlers...) Route` | A route for that method |
| `Add(method, path, handlers...) Route` | A route for any method, including custom ones |
| `Match(methods []string, path, handlers...)` | The same chain for several methods; returns nothing |
| `All(path, handlers...)` | The same chain for every standard method; returns nothing |
| `TryHandle(spec RouteSpec) error` | A route from configuration or plugins, returning every problem as an error |
| `HandleHTTP(pattern, http.Handler) Route` | A standard handler, with a `"METHOD /path"` pattern |

Every method accepts a chain: middleware first, then the handler. Invalid or conflicting patterns panic at registration. See [Routing](/guide/routing/).

The returned `Route` has these methods. Each returns the route, so they chain:

| Method | Purpose |
|---|---|
| `Name(name) Route` | A unique name for `URL` and `RouteByName`, and the OpenAPI operation ID; panics on a duplicate |
| `Status(code) Route` | The success status of a [typed handler](/guide/typed-handlers/), such as `201`; panics unless `code` is 2xx |
| `Summary(text) Route` | A one-line summary in the [OpenAPI](/guide/openapi/) spec |
| `Description(text) Route` | A longer description in the spec; Markdown works |
| `Tags(tags...) Route` | Tags that group the route in the spec, after its group's |
| `Deprecated() Route` | Marks the route deprecated in the spec |
| `Hidden() Route` | Leaves the route out of the spec |
| `Input(v) Route` | The request type of a handler that isn't typed, such as `CreateUser{}`; panics on a typed route |
| `Output(v) Route` | The success-response type of a handler that isn't typed; panics on a typed route |
| `Response(status, v) Route` | Another response the handler writes itself; `nil` means no body |
| `Errors(statuses...) Route` | Error statuses the route answers by returning an error, described with the error handler's body |
| `Security(schemes...) Route` | The security schemes that protect the route, replacing its group's; any one is enough, and none marks it public. `"oauth:pets:read"` adds a scope |
| `SecurityAll(schemes...) Route` | Like `Security`, but every scheme is needed |

The spec methods change nothing about how the route serves requests.

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

## OpenAPI

| Method | Purpose |
|---|---|
| `OpenAPI(path, OpenAPIConfig, middleware...) Route` | Serves the spec as JSON at `path`, with `GET`, in place of the one at `Config.OpenAPIPath`. Built on the first request and kept; rebuilt when routes are added. Panics on a config the spec can't be valid with |
| `OpenAPISpec(OpenAPIConfig) ([]byte, error)` | The spec as JSON, without serving it |

Every app also serves its spec at `Config.OpenAPIPath`, `/openapi.json` unless set, described by `Config.OpenAPI`; `"-"` turns it off.

```go
type OpenAPIConfig struct {
	Title           string // default: the main module's name
	Version         string // default: the main module's version, or "0.0.0"
	Description     string
	TermsOfService  string
	Contact         *OpenAPIContact      // {Name, URL, Email}
	License         *OpenAPILicense      // {Name, Identifier, URL}
	ExternalDocs    *OpenAPIExternalDocs // {Description, URL}
	Servers         []OpenAPIServer      // {URL, Description}
	Tags            []OpenAPITag         // {Name, Description, ExternalDocs}
	SecuritySchemes map[string]OpenAPISecurityScheme // by name
	Security        []string                         // for routes that set none
	NoAuthResponses bool                             // no automatic 401 and 403
	Schemas         map[reflect.Type]map[string]any  // for types from other packages
}

type OpenAPISecurityScheme struct {
	Type             string // "http", "apiKey", "oauth2", "openIdConnect" or "mutualTLS"
	Scheme           string // for "http": "bearer" or "basic"
	BearerFormat     string // such as "JWT"
	In, Name         string // for "apiKey": "header", "query" or "cookie", and its name
	Flows            *OpenAPIOAuthFlows // for "oauth2": AuthorizationCode, ClientCredentials, Password, Implicit
	OpenIDConnectURL string
	Description      string
}

type OpenAPIOAuthFlow struct {
	AuthorizationURL, TokenURL, RefreshURL string
	Scopes                                 map[string]string // scope: description
}
```

A type that implements `zinc.SchemaProvider`, with an `OpenAPISchema() map[string]any` method, supplies its own JSON Schema. A named type that implements `zinc.EnumProvider`, with an `Enum() []any` method, becomes an enum. See [OpenAPI](/guide/openapi/).

## Groups and middleware

| Method | Purpose |
|---|---|
| `Group(prefix, middleware...) *Group` | Routes that share a prefix and middleware |
| `Route(prefix, fn func(*Group), middleware...) *Group` | The same, declared in a nested block |
| `Use(middleware...)` | Middleware for every request |
| `UsePrefix(prefix, middleware...)` | Middleware for requests under a prefix, before routing |
| `UseHTTP(middleware ...zinc.HTTPMiddleware)` | Standard `func(http.Handler) http.Handler` middleware around the whole app |
| `Mount(prefix, http.Handler)` | A handler that owns a subtree; receives paths without the prefix |

Two functions build middleware from other middleware:

| Function | Returns |
|---|---|
| `zinc.Skip(skip func(*Context) bool, mw Middleware) Middleware` | `mw`, except for requests where `skip` reports true |
| `zinc.FromHTTP(mw HTTPMiddleware) Middleware` | Standard middleware as Zinc middleware, for one route or group |

See [Groups and Middleware](/guide/groups-and-middleware/) for execution order.

## Files

| Method | Serves |
|---|---|
| `Static(prefix, dir, opts ...StaticOption)` | A directory from disk |
| `StaticFS(prefix, fs.FS, opts ...StaticOption)` | A directory from any filesystem, such as `embed.FS` |
| `File(path, file) Route` | One file from disk |
| `FileFS(path, name, fs.FS) Route` | One file from a filesystem |

A nil filesystem panics at registration, like other registration mistakes. Options are `StaticOption` values that adjust a `StaticConfig`: `zinc.WithStaticIndex(name)` and `zinc.WithStaticBrowse(bool)`. See [Static Files](/guide/static-files/).

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
| `URL(name, params...) (string, error)` | The path for a named route, with parameters filled in order. Values are escaped; a catch-all value keeps its slashes, with `?`, `#` and `%` escaped. |

```go
type RouteInfo struct {
	Name    string
	Method  string
	Path    string   // the registered pattern
	Params  []string // parameter names, in order
	Mounted bool     // true for Mount, Static, and StaticFS
	Handler string   // the handler's function name
	Status  int      // the success status set with Route.Status, or 0
}
```

## Server lifecycle

| Method | Purpose |
|---|---|
| `Listen(addr...) error` | Serve HTTP; the address defaults to `:8080` |
| `ListenContext(ctx, addr) error` | Serve HTTP until `ctx` ends, then shut down gracefully within `Config.ShutdownTimeout` |
| `ListenTLS(addr, certFile, keyFile) error` | Serve HTTPS |
| `Serve(net.Listener) error` | Serve on a listener you created |
| `Shutdown(ctx) error` | Stop accepting connections, wait for in-flight requests, and close the folders `Static` keeps open |
| `Close() error` | Stop the active server and close the folders `Static` keeps open, immediately |
| `ServeHTTP(w, r)`, `Handler()` | Use the app as an `http.Handler` |

`Listen`, `ListenContext`, `ListenTLS`, and `Serve` apply the timeouts from [configuration](/guide/configuration/#server). The [Graceful Shutdown](/cookbook/graceful-shutdown/) recipe shows `ListenContext` in a complete program. Use `Shutdown` for servers started with `Listen` or `Serve`.

## Adapters

`zinc.Wrap(http.Handler)` and `zinc.WrapFunc(http.HandlerFunc)` turn a standard handler into a Zinc `HandlerFunc`, for routes that mix both.

`AcquireContext(w, r) *Context` and `ReleaseContext(c)` create and recycle a context outside normal dispatch. They exist for adapters and low-level tests; applications do not need them.

## Related

- [Group](/api/group/) has the same routing methods, scoped to a prefix.
- [Context](/api/context/) is what every handler receives.
- [pkg.go.dev](https://pkg.go.dev/github.com/0mjs/zinc#App) has the generated reference.
