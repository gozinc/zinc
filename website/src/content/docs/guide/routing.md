---
title: Routing
description: Send each request to the right handler, read values from the path, and control what happens when nothing matches.
---

Routing is how Zinc decides which handler runs for a request. You register a method and a path pattern, and Zinc calls your handler when a request matches.

```go
app.Get("/users", listUsers)             // GET /users
app.Post("/users", createUser)           // POST /users
app.Get("/users/{id}", showUser)         // GET /users/42
app.Get("/files/{path...}", serveFile)   // GET /files/css/app.css
```

Patterns use the same `{name}` syntax as Go's `net/http`, so you can move a route between a Zinc handler and a standard `http.Handler` without rewriting it.

## Register a route

There's a method for each HTTP verb: `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect` and `Trace`.

```go
app.Put("/users/{id}", updateUser)
app.Delete("/users/{id}", deleteUser)
```

For a custom method, or several methods at once:

```go
// One custom method
app.Add("PURGE", "/cache/{key}", purgeCache)

// A set of methods
app.Match([]string{zinc.MethodGet, zinc.MethodPost}, "/search", search)

// Every standard method
app.All("/debug", debugHandler)
```

## Read a path parameter

A `{name}` segment matches one part of the path. Read its value with `c.Param`:

```go
app.Get("/teams/{team}/users/{user}", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"team": c.Param("team"),
		"user": c.Param("user"),
	})
})
```

```bash
curl http://localhost:8080/teams/core/users/ada
# {"team":"core","user":"ada"}
```

`c.Param` always returns a string. When you need a number or another type, use `zinc.Param`. If the value doesn't fit, return the error and the client gets a `400` that names the parameter:

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	id, err := zinc.Param[int64](c, "id")
	if err != nil {
		return err
	}
	return c.JSON(zinc.Map{"id": id})
})
```

```bash
curl http://localhost:8080/users/42
# {"id":42}

curl http://localhost:8080/users/abc
# {"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}
```

To read several values into a struct, [bind](/guide/binding/) them with `path` tags.

## Match the rest of a path

End a pattern with `{name...}` to match everything after that point, slashes included. It also matches nothing at all:

```go
app.Get("/files/{path...}", func(c *zinc.Context) error {
	return c.String(c.Param("path"))
})
```

```bash
curl http://localhost:8080/files/css/app.css   # css/app.css
curl http://localhost:8080/files/              # (empty)
```

## Which route wins

When more than one route could match, the most specific one wins, one path segment at a time:

1. **Fixed text beats a parameter.** `/users/me` wins over `/users/{id}`.
2. **A parameter beats a rest-of-path match.** `/files/{name}` wins over `/files/{path...}`.

The order you register routes in doesn't matter.

Two defaults are worth knowing, because Gin and Echo behave differently:

- **Case doesn't matter.** `/Users/42` reaches the same handler as `/users/42`, so a link typed with capitals still works. Parameter values keep their original case. Set `CaseSensitive: true` in [`zinc.Config`](/guide/configuration/) to turn this off.
- **A trailing slash doesn't matter.** `/users/42/` reaches the same handler as `/users/42`. Set `StrictRouting: true` to treat them as different routes.

:::tip[Try it on the homepage]
The route matcher on the [Zinc homepage](/) runs these rules live. Type a path and see which route wins, and why.
:::

## Group related routes

A group gives routes a shared path prefix and shared middleware:

```go
api := app.Group("/api", requireAPIKey)
v1 := api.Group("/v1")

v1.Get("/users/{id}", showUser) // GET /api/v1/users/{id}, runs requireAPIKey first
v1.Post("/users", createUser)   // POST /api/v1/users
```

If you prefer nesting, `Route` builds the same thing as a block:

```go
app.Route("/api", func(api *zinc.Group) {
	api.Route("/v1", func(v1 *zinc.Group) {
		v1.Get("/users/{id}", showUser)
		v1.Post("/users", createUser)
	})
}, requireAPIKey)
```

[Groups and Middleware](/guide/groups-and-middleware/) covers middleware order and scope.

## When nothing matches

Zinc answers routing misses for you, with the status a client expects:

```bash
curl -i http://localhost:8080/nope
# HTTP/1.1 404 Not Found
# {"error":{"status":404,"message":"Not Found"}}

curl -i -X DELETE http://localhost:8080/users
# HTTP/1.1 405 Method Not Allowed
# Allow: GET, HEAD, POST, OPTIONS
# {"error":{"status":405,"message":"Method Not Allowed"}}
```

| Request | Response |
|---|---|
| No route has this path | `404 Not Found` |
| The path exists, but not for this method | `405 Method Not Allowed`, with an `Allow` header listing the methods that work |
| `OPTIONS` for a path that exists | `204 No Content`, with an `Allow` header |
| `HEAD` for a path with a `GET` route | Runs the `GET` handler and sends the headers without the body |

You can turn off the last three with `DisableMethodNotAllowed`, `DisableAutoOptions` and `DisableAutoHead` in [`zinc.Config`](/guide/configuration/).

### Customize 404 and 405

Both responses go through your [error handler](/guide/errors/), so by default they're JSON like every other error. To send something else across the whole app, such as an HTML page:

```go
app.NotFound(func(c *zinc.Context) error {
	return c.Status(zinc.StatusNotFound).HTML(notFoundPage)
})

app.MethodNotAllowed(func(c *zinc.Context) error {
	return zinc.NewError(zinc.StatusMethodNotAllowed, "this endpoint is read-only")
})
```

To change it for part of the site only, for example to keep API misses as JSON while everything else gets the HTML page:

```go
app.RouteNotFound("/api/{rest...}", func(c *zinc.Context) error {
	return zinc.NotFound("unknown API route")
})
```

## Build a URL from a route name

Give a route a name, then build its URL anywhere without writing the path out again:

```go
app.Get("/users/{id}", showUser).Name("users.show")

url, err := app.URL("users.show", "42") // "/users/42"
```

Names must be unique: a duplicate panics at startup. `app.URL` returns an error if a value can't form a valid URL, such as an empty `id`.

## Use a standard handler

Any `http.Handler` can serve a route. It reads parameters with `r.PathValue`:

```go
app.HandleHTTP("GET /metrics", promhttp.Handler())

app.HandleHTTP("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, r.PathValue("id"))
}))

app.Mount("/legacy", legacyMux) // legacyMux handles everything under /legacy
```

[Zinc and net/http](/guide/http-interoperability/) covers mounting and standard middleware.

## List your routes

Every route you register is recorded, which helps in tests and debug pages:

```go
all := app.Routes()                                     // every route, in the order you added them
route, ok := app.FindRoute(zinc.MethodGet, "/users/42") // the route that would serve this request
named, ok := app.RouteByName("users.show")              // a route by its name
```

## Good to know

### Mistakes stop the app at startup

An invalid pattern panics when you register it, so you find out when the app starts, not when the first user hits the route:

```text
/users/:id             use /users/{id}
/files/*path           use /files/{path...}
/users/prefix-{id}     a parameter must be a whole segment
/files/{path...}/meta  a rest-of-path match must come last
/users/{id}/{id}       each parameter name can appear once
```

The same method and path registered twice also panics. So do two routes for the same method that differ only in a parameter name, such as `GET /users/{id}` and `GET /users/{name}`.

### Routes from configuration

If your patterns come from a config file or a plugin, use `TryHandle` instead. It returns an error rather than panicking:

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

### What a pattern can contain

```text
pattern   = "/" [ segment { "/" segment } ]
segment   = text | parameter | rest
parameter = "{" name "}"
rest      = "{" name "...}"     ; last segment only
```

- **Names** use letters, digits and underscores, and don't start with a digit.
- **Text** can contain any character except `{` and `}`. Routes like `/v1/users:batch` or `/opening/09:00` work as written.
- **A segment can't start with `:` or `*`.** That's Gin's and Echo's parameter syntax, so `/users/:id` fails at startup and tells you to write `{id}`, rather than quietly creating a route that never matches.
- **Some `net/http` pattern features aren't supported**: host names, the `{$}` end marker, and a method inside the path. `HandleHTTP("GET /users/{id}", h)` is the one place a method prefix works, because Zinc reads it off before matching.

### Paths are matched after decoding

Zinc matches on the decoded path, so `%2F` in a request counts as a `/`. That's why a `{name}` value can never contain a slash, and why `app.URL` rejects one. If a value needs slashes, use a `{name...}` parameter: `app.URL` keeps its slashes and escapes `?`, `#` and `%`.

### Mounted handlers see a trimmed path

A handler mounted at `/legacy` sees `/legacy/users` as `/users`. Zinc gives it a copy of the request with the path trimmed, so your outer middleware still sees the original.

## Next steps

- [Request Data](/guide/request/): query strings, headers, forms and files.
- [Binding](/guide/binding/): read path, query and body values into a struct in one call.
- [Groups and Middleware](/guide/groups-and-middleware/): run code before and after handlers.
