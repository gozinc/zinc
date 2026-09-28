---
title: Coming from Gin or Echo
description: The Zinc version of what you already write in Gin or Echo, side by side, and the few differences worth knowing.
---

If you've used Gin or Echo, you already know most of Zinc. Handlers take a context, routes group under prefixes, and middleware wraps handlers. This page maps what you write today to the Zinc version, then lists the differences that catch people out.

## Side by side

| | Gin | Echo | Zinc |
|---|---|---|---|
| Create an app | `r := gin.Default()` | `e := echo.New()` | `app := zinc.New()` |
| Handler | `func(c *gin.Context)` | `func(c *echo.Context) error` | `func(c *zinc.Context) error` |
| Route | `r.GET("/users/:id", h)` | `e.GET("/users/:id", h)` | `app.Get("/users/{id}", h)` |
| Rest of path | `/files/*path` | `/files/*` | `/files/{path...}` |
| Path value | `c.Param("id")` | `c.Param("id")` | `c.Param("id")` |
| Query value | `c.Query("q")` | `c.QueryParam("q")` | `c.Query("q")` |
| JSON body | `c.ShouldBindJSON(&in)` | `c.Bind(&in)` | `c.Bind().JSON(&in)` |
| JSON response | `c.JSON(200, v)` | `c.JSON(200, v)` | `c.JSON(v)` |
| Other status | `c.JSON(201, v)` | `c.JSON(201, v)` | `c.Status(201).JSON(v)` |
| Return an error | `c.AbortWithStatusJSON(404, …)` | `return echo.NewHTTPError(404, "…")` | `return zinc.NotFound("…")` |
| Group | `r.Group("/api", mw)` | `e.Group("/api", mw)` | `app.Group("/api", mw)` |
| Middleware | `r.Use(mw)` | `e.Use(mw)` | `app.Use(mw)` |
| Request values | `c.Set("user", u)` | `c.Set("user", u)` | `c.Set("user", u)` |
| Static files | `r.Static("/assets", "./public")` | `e.Static("/assets", "public")` | `app.Static("/assets", "./public")` |
| Start | `r.Run(":8080")` | `e.Start(":8080")` | `app.Listen(":8080")` |

## One handler, three ways

The same endpoint: read an ID from the path, look up a user, and return it or a 404.

```go
// Gin
r.GET("/users/:id", func(c *gin.Context) {
	user, ok := users[c.Param("id")]
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
})

// Echo
e.GET("/users/:id", func(c *echo.Context) error {
	user, ok := users[c.Param("id")]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	return c.JSON(http.StatusOK, user)
})

// Zinc
app.Get("/users/{id}", func(c *zinc.Context) error {
	user, ok := users[c.Param("id")]
	if !ok {
		return zinc.NotFound("user not found")
	}
	return c.JSON(user)
})
```

## Things that work differently

### Patterns use braces

Zinc uses the same `{id}` syntax as Go's own `net/http`. A route written as `/users/:id` fails when the app starts and tells you to write `/users/{id}`, so you can't end up with a route that silently never matches.

### The status is set on its own

`c.JSON(v)` sends `200`. For another status, set it first: `c.Status(zinc.StatusCreated).JSON(v)`. Status constants live in the `zinc` package, so you don't need to import `net/http` for them.

### Errors are values you return

As in Echo, a handler returns an error instead of writing one and stopping. Zinc sends every returned error through one [error handler](/guide/errors/), which answers with JSON by default:

```json
{"error":{"status":404,"message":"user not found"}}
```

There's no `Abort` step. Middleware stops a request the same way: by returning an error instead of calling `c.Next()`.

### Middleware has the handler's shape

In Gin, middleware is a `gin.HandlerFunc` that calls `c.Next()`. In Echo, it wraps the next handler. In Zinc it's an ordinary handler that calls `c.Next()` and returns its error:

```go
func timing(c *zinc.Context) error {
	start := time.Now()
	err := c.Next()
	log.Printf("%s %s took %s", c.Method(), c.Path(), time.Since(start))
	return err
}
```

Standard `func(http.Handler) http.Handler` middleware works too, through [`app.UseHTTP`](/guide/http-interoperability/).

### Binding needs tags

`c.Bind().JSON` reads the body using `json` tags, like `encoding/json`. Path, query and header values only fill fields that have a matching `path`, `query` or `header` tag, so a client can't set a field you didn't mean to expose. See [Binding](/guide/binding/).

### Case and trailing slashes don't matter by default

`/Users/42` and `/users/42/` reach the same handler as `/users/42`. Gin and Echo treat them as different paths. Set `CaseSensitive` or `StrictRouting` in [`zinc.Config`](/guide/configuration/) if you want that behaviour back.

### It's all `net/http` underneath

The app is an `http.Handler`, and handlers can use standard ones. You can mount an existing `http.ServeMux`, serve a route with `promhttp.Handler()`, or run Zinc inside a server you already have. See [Zinc and net/http](/guide/http-interoperability/).

## A complete example

A small users API in Zinc, with the pieces above in one place:

```go
package main

import (
	"log"
	"sync"
	"time"

	"github.com/0mjs/zinc"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type store struct {
	mu    sync.Mutex
	users map[string]User
}

func timing(c *zinc.Context) error {
	start := time.Now()
	err := c.Next()
	log.Printf("%s %s took %s", c.Method(), c.Path(), time.Since(start))
	return err
}

func main() {
	s := &store{users: map[string]User{}}
	app := zinc.New()
	app.Use(timing)

	api := app.Group("/api")

	api.Get("/users/{id}", func(c *zinc.Context) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		user, ok := s.users[c.Param("id")]
		if !ok {
			return zinc.NotFound("user not found")
		}
		return c.JSON(user)
	})

	api.Post("/users", func(c *zinc.Context) error {
		var in User
		if err := c.Bind().JSON(&in); err != nil {
			return err // 400 that names the bad field
		}
		if in.ID == "" || in.Name == "" {
			return zinc.UnprocessableEntity("id and name are required")
		}
		s.mu.Lock()
		s.users[in.ID] = in
		s.mu.Unlock()
		return c.Status(zinc.StatusCreated).JSON(in)
	})

	log.Fatal(app.Listen(":8080"))
}
```

```bash
curl -X POST localhost:8080/api/users -H "Content-Type: application/json" -d '{"id":"1","name":"Ada"}'
# {"id":"1","name":"Ada"}

curl localhost:8080/api/users/1
# {"id":"1","name":"Ada"}

curl localhost:8080/api/users/2
# {"error":{"status":404,"message":"user not found"}}
```

## Next steps

- [Routing](/guide/routing/): patterns, groups and what happens when nothing matches.
- [Errors](/guide/errors/): return errors and shape the responses.
- [Middleware](/middleware/overview/): the built-in middleware, from logging to CORS.
