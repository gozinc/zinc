---
title: Trailing Slash
description: Remove or add the trailing slash on request paths, or redirect clients to one form.
---

Trailing Slash makes `/users/` and `/users` reach the same route, or sends clients to one canonical form of each URL. Zinc already matches both forms unless you turn on `StrictRouting`, so you'd add this to a strict app, or when you want one URL per page for caches and search engines.

## Usage

```go
import "github.com/0mjs/zinc/middleware/trailingslash"

app := zinc.New(zinc.Config{StrictRouting: true})
app.Use(trailingslash.New())

app.Get("/users", func(c *zinc.Context) error {
	return c.String("users at " + c.Path())
})
```

```bash
curl http://localhost:8080/users/    # users at /users
curl http://localhost:8080/users//   # users at /users
```

Without the middleware, the strict app answers `/users/` with a 404. The slash is removed before routing, and the client isn't told.

Register it with `app.Use`, so it runs before routing.

## Defaults

| Setting | Default |
|---|---|
| Direction | remove trailing slashes |
| Client sees | nothing: the path is changed before routing |
| Redirect status | `301 Moved Permanently` (when `Redirect` is on) |

## Configuration

Redirect clients to the form without the slash, keeping the method and body:

```go
app.Use(trailingslash.New(trailingslash.Config{
	Redirect:   true,
	StatusCode: zinc.StatusPermanentRedirect,
}))
```

```bash
curl -i "http://localhost:8080/users/?page=2"
# HTTP/1.1 308 Permanent Redirect
# Location: /users?page=2
```

| Field | Default | Meaning |
|---|---|---|
| `Add` | `false` | Adds a trailing slash instead of removing it |
| `Redirect` | `false` | Redirects the client to the changed path instead of routing it directly. The query string is kept. |
| `StatusCode` | `301` | Redirect status when `Redirect` is on. A value outside `3xx` sends `302 Found`. |

## Always end with a slash

For sites whose URLs end in `/`, add the slash and redirect to it:

```go
app := zinc.New(zinc.Config{StrictRouting: true})
app.Use(trailingslash.New(trailingslash.Config{Add: true, Redirect: true}))

app.Get("/docs/", func(c *zinc.Context) error {
	return c.String("docs")
})
```

```bash
curl -i http://localhost:8080/docs
# HTTP/1.1 301 Moved Permanently
# Location: /docs/
```

The root path `/` is always left alone.

## Errors

`trailingslash` never fails a request. If the changed path has no route, the client gets the usual 404.

`trailingslash.New` panics at startup when it's given more than one `Config`.

## Related

- [Routing](/guide/routing/): how Zinc treats trailing slashes, and `StrictRouting`.
- [Redirect](/middleware/redirect/): redirects old paths to new ones.
- [Rewrite](/middleware/rewrite/): routes one path as another.
