---
title: Routing
description: Register routes, capture path parameters and wildcards, understand matching precedence, and handle 404 and 405 responses.
---

A route pairs an HTTP method and a path pattern with a handler chain. Zinc uses the brace syntax from Go 1.22's `net/http`, so patterns look the same whether a Zinc handler or a standard handler serves them.

```go
app.Get("/users", listUsers)
app.Post("/users", createUser)
app.Get("/users/{id}", showUser)
app.Get("/assets/{path...}", serveAsset)
```

## Methods

Each common method has a helper: `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect`, and `Trace`.

```go
app.Put("/users/{id}", updateUser)
app.Delete("/users/{id}", deleteUser)
```

For anything else, name the methods yourself:

```go
app.Add("PURGE", "/cache/{key}", purgeCache)                   // one custom method
app.Match([]string{zinc.MethodGet, zinc.MethodHead}, "/ping", ping) // a chosen set
app.All("/echo", echo)                                         // every standard method
```

## Path parameters

A `{name}` segment captures exactly one non-empty path segment. Read it with `c.Param`.

```go
app.Get("/teams/{team}/users/{user}", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"team": c.Param("team"),
		"user": c.Param("user"),
	})
})
```

Parameters are strings. Convert and validate them in the handler, or [bind](/guide/binding/) them into a typed struct with `path:"team"` tags. Zinc deliberately has no regex constraints in patterns.

## Wildcards

A final `{name...}` segment captures the rest of the path, including slashes. It can also be empty.

```go
app.Get("/files/{path...}", func(c *zinc.Context) error {
	return c.String(c.Param("path")) // "/files/css/app.css" gives "css/app.css"
})
```

## Matching rules

When more than one route could match a request, Zinc picks the most specific one, segment by segment:

1. **Static segments** win over parameters: `/users/me` beats `/users/{id}`.
2. **Parameters** win over wildcards: `/files/{name}` beats `/files/{path...}`.

A few more rules round out the behavior:

- **Case.** Literal segments ignore case by default, so `/Users` matches `/users`. Captured values keep their original case. Set `CaseSensitive` to change this.
- **Trailing slashes.** `/users` and `/users/` are the same route by default. Set `StrictRouting` to treat them as different.
- **Encoding.** Matching uses `Request.URL.Path`, the decoded path.
- **Conflicts.** Parameter names do not make routes distinct: `/users/{id}` and `/users/{name}` conflict for the same method.

:::tip[Try it on the homepage]
The route matcher on the [Zinc homepage](/) runs these rules live. Type a path and see which route wins and why.
:::

### Pattern grammar

```text
pattern   = "/" [ segment { "/" segment } ]
segment   = literal | parameter | catch-all
parameter = "{" identifier "}"
catch-all = "{" identifier "...}"   ; final segment only
```

An identifier contains letters, digits, and underscores, and cannot start with a digit. A parameter must fill a whole segment, and each name can appear only once per pattern.

Some `net/http` pattern features are intentionally not supported: method or host prefixes inside ordinary route paths, the `{$}` end marker, and `ServeMux`'s overlap resolution. `HandleHTTP("GET /users/{id}", h)` accepts a method prefix because the method is split off before matching.

### Invalid patterns fail at startup

Bad patterns panic when they are registered, so a mistake stops the program at boot instead of hiding until the first request.

```text
/users/:id             use /users/{id}
/files/*path           use /files/{path...}
/users/prefix-{id}     a parameter must fill a whole segment
/files/{path...}/meta  a catch-all must be last
/users/{id}/{id}       parameter names must be unique
```

When patterns come from configuration or plugins, use [`TryHandle`](#routes-from-configuration) to get an error instead of a panic.

## Groups

A group shares a path prefix and middleware across related routes.

```go
api := app.Group("/api", requireAPIKey)
v1 := api.Group("/v1")

v1.Get("/users/{id}", showUser) // GET /api/v1/users/{id}, runs requireAPIKey first
v1.Post("/users", createUser)
```

`Route` does the same with a nested block, which some teams find easier to scan:

```go
app.Route("/api", func(api *zinc.Group) {
	api.Route("/v1", func(v1 *zinc.Group) {
		v1.Get("/users/{id}", showUser)
		v1.Post("/users", createUser)
	})
}, requireAPIKey)
```

[Groups and Middleware](/guide/groups-and-middleware/) covers ordering and scoping in detail.

## Not found and method not allowed

Zinc answers routing misses with the right status:

| Request | Default response |
|---|---|
| No route matches the path | `404 Not Found` |
| The path exists, but not for this method | `405 Method Not Allowed` with an `Allow` header |
| `OPTIONS` for a known path | `204 No Content` with an `Allow` header |
| `HEAD` for a path with a `GET` route | The `GET` handler runs, without a body |

The last three are on by default. Turn them off with `DisableMethodNotAllowed`, `DisableAutoOptions`, and `DisableAutoHead` in [`zinc.Config`](/guide/configuration/).

The default `404` and `405` go through your [error handler](/guide/errors/), so they are JSON error bodies like every other error. Replace them app-wide, for example to serve an HTML page:

```go
app.NotFound(func(c *zinc.Context) error {
	return c.Status(zinc.StatusNotFound).HTML(notFoundPage)
})

app.MethodNotAllowed(func(c *zinc.Context) error {
	return zinc.NewError(zinc.StatusMethodNotAllowed, "this endpoint is read-only")
})
```

Or only below a prefix, for example to keep API misses in JSON while the rest of the site serves that HTML page:

```go
app.RouteNotFound("/api/{tail...}", func(c *zinc.Context) error {
	return zinc.NotFound("unknown API route")
})
```

A handler that returns an error goes through the error handler; one that writes a response sends it as written.

## Named routes and URLs

Every registration method returns the `Route` it created. Name it to build its URL elsewhere without hard-coding paths:

```go
app.Get("/users/{id}", showUser).Name("users.show")

url, err := app.URL("users.show", "42") // "/users/42"
```

Names are unique. A duplicate panics at startup, like an invalid pattern.

### Routes from configuration

Route declarations in your source panic when they are invalid, so mistakes surface at startup. For routes you do not control, such as patterns from configuration or plugins, `TryHandle` returns every problem, including a duplicate name, as an error:

```go
if err := app.TryHandle(zinc.RouteSpec{
	Name:    nameFromConfig,
	Method:  zinc.MethodGet,
	Path:    patternFromConfig,
	Handler: showUser,
}); err != nil {
	return fmt.Errorf("register route: %w", err)
}
```

## Standard library handlers

Any `http.Handler` can serve a route, and it reads parameters with `r.PathValue`:

```go
app.HandleHTTP("GET /metrics", promhttp.Handler())

app.HandleHTTP("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, r.PathValue("id"))
}))

app.Mount("/legacy", legacyMux) // owns everything below /legacy
```

See [Zinc and net/http](/guide/http-interoperability/) for the details.

## Inspecting routes

Route metadata stays available for tests, debug pages, and tooling.

```go
all := app.Routes()                                     // every route, in registration order
route, ok := app.FindRoute(zinc.MethodGet, "/users/42") // what would serve this request
named, ok := app.RouteByName("users.show")              // a named route
```

## Next steps

- [Groups and Middleware](/guide/groups-and-middleware/) for scoping behavior to route families.
- [Request Data](/guide/request/) for everything you can read from a request.
- [Application API](/api/app/) for the complete method list.

Named URLs reject empty or slash-containing values for single-segment parameters. Routing uses decoded `URL.Path`, so an encoded slash cannot represent a single segment. Use a catch-all parameter for multiple segments; generated catch-all values preserve `/` separators while escaping query, fragment, and percent characters.

Mounted handlers receive a cloned request with consistent `URL.Path`, `URL.RawPath`, and `RequestURI`. The original request remains available to outer middleware. Case-insensitive static routes retain precedence over parameter routes, including Unicode case folds; captured values preserve their original spelling.
