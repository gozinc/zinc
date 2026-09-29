---
title: Rewrite
description: Route a request as if it had asked for a different path, without telling the client.
---

Rewrite changes the path Zinc routes a request by, so an old URL can be served by a new handler without a redirect. Use it to keep old API paths working or to give one route a second address. The client keeps the URL it asked for; to send it to the new one, use [Redirect](/middleware/redirect/).

## Usage

```go
import "github.com/0mjs/zinc/middleware/rewrite"

app.Use(rewrite.New(rewrite.Config{Rules: map[string]string{
	"/old":  "/new",
	"/v1/*": "/api/v1/*",
}}))

app.Get("/api/v1/users/{id}", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"id":       c.Param("id"),
		"path":     c.Path(),
		"original": c.OriginalURL(),
		"page":     c.Query("page"),
	})
})
```

```bash
curl -i "http://localhost:8080/v1/users/42?page=2"
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
#
# {"id":"42","original":"/v1/users/42?page=2","page":"2","path":"/api/v1/users/42"}
```

The handler sees the new path in `c.Path()`, and `c.OriginalURL()` still returns what the client sent. The query string is left alone.

Register it with `app.Use`, so it runs before routing picks a handler.

:::caution[Not in a group]
In a group, routing has already chosen a handler by the time `rewrite` runs: the path would change, but the original route's handler would still run. Registering it on a group panics at startup:

```go
app.Group("/api").Use(rewrite.New(cfg))
// panic: zinc: rewrite middleware on group "/api" would run after routing, where it can't work; register it with app.Use, or app.UsePrefix("/api", ...) for the group's paths
```

On a single route it can't change the route either, and logs a warning through `slog` the first time it runs.
:::

## Defaults

| Setting | Default |
|---|---|
| Rules | none, so nothing is rewritten |

## Configuration

| Field | Default | Meaning |
|---|---|---|
| `Rules` | none | Incoming path → path to route instead. A key ending in `*` matches a prefix. A `*` in the value is replaced by the rest of the path; with no `*`, the rest is appended. |

Rules compare the path exactly, so `/old` doesn't match `/OLD` or `/old/`. An exact rule wins over a `*` rule; when two `*` rules match the same path, which one applies isn't defined, so keep prefixes from overlapping.

## Errors

`rewrite` never fails a request. If the new path has no route, the client gets the usual 404.

`rewrite.New` panics at startup when it's given more than one `Config`.

## Related

- [Redirect](/middleware/redirect/): sends the client to the new URL instead.
- [Proxy](/middleware/proxy/): rewrites paths on their way to another server.
- [Routing](/guide/routing/): how Zinc matches the rewritten path.
