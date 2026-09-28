---
title: Headers
description: Add fixed headers to every response, or run different middleware when a request carries a header.
---

Headers adds the same response headers to every request it runs on, such as an API version or an app name. It can also pick a middleware to run when a request carries a particular header. For browser security headers, use [Secure Headers](/middleware/secure/) instead.

## Usage

```go
import "github.com/0mjs/zinc/middleware/headers"

app.Use(headers.New(headers.Config{
	Set: map[string]string{"X-App": "zinc", "X-API-Version": "2"},
}))

app.Get("/", func(c *zinc.Context) error {
	return c.String("hi")
})
```

```bash
curl -i http://localhost:8080/
# HTTP/1.1 200 OK
# Content-Type: text/plain; charset=utf-8
# X-Api-Version: 2
# X-App: zinc
#
# hi
```

The headers are set before your handler runs, so they're on every response, 404s and errors included. A handler can still overwrite one with `c.SetHeader`.

## Defaults

`headers.New()` with no config sets nothing and runs no routes.

| Setting | Default |
|---|---|
| Response headers | none |
| Header routes | none |

## Configuration

| Field | Default | Meaning |
|---|---|---|
| `Set` | none | Response headers to set, as name → value. Names are canonicalized, so `x-app` is sent as `X-App`. |
| `Routes` | none | Header routes, checked in order. See below. |

Each `headers.Route` has three fields:

| Field | Meaning |
|---|---|
| `Header` | Request header to look for. **Required.** |
| `Value` | Value it must equal. Empty matches any non-empty value. |
| `Middleware` | Middleware to run when the route matches. **Required.** |

## Run middleware by request header

Mark requests that carry `X-Admin: 1`, and let every other request through untouched:

```go
app.Use(headers.New(headers.Config{
	Routes: []headers.Route{{
		Header: "X-Admin",
		Value:  "1",
		Middleware: func(c *zinc.Context) error {
			c.Set("admin", true)
			return c.Next()
		},
	}},
}))

app.Get("/whoami", func(c *zinc.Context) error {
	admin, _ := c.Get("admin")
	return c.JSON(zinc.Map{"admin": admin == true})
})
```

```bash
curl http://localhost:8080/whoami -H "X-Admin: 1"     # {"admin":true}
curl http://localhost:8080/whoami -H "X-Admin: yes"   # {"admin":false}
curl http://localhost:8080/whoami                     # {"admin":false}
```

The first route that matches runs in place of the rest of the chain, and decides whether to call `c.Next()`. If no route matches, the request continues as normal. A route middleware that returns an error ends the request there:

```go
app.Use(headers.New(headers.Config{
	Routes: []headers.Route{{
		Header:     "X-Debug",
		Middleware: func(c *zinc.Context) error { return zinc.ErrForbidden },
	}},
}))

app.Get("/", func(c *zinc.Context) error {
	return c.String("hi")
})
```

```bash
curl -i http://localhost:8080/ -H "X-Debug: anything"
# HTTP/1.1 403 Forbidden
#
# {"error":{"status":403,"message":"Forbidden"}}
```

## Errors

`headers` returns no errors of its own. A route middleware's error goes to the app's error handler as usual.

`headers.New` panics at startup when a `Route` has no `Header` or no `Middleware`, or when it's given more than one `Config`.

## Related

- [Secure Headers](/middleware/secure/): sets browser security headers with safe defaults.
- [No Cache](/middleware/nocache/): sets the headers that stop caching.
- [Groups and Middleware](/guide/groups-and-middleware/): writing your own middleware.
