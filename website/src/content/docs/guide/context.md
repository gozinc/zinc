---
title: Context
description: Understand the request-scoped Context, share values between middleware and handlers, and keep background work safe.
---

Every handler and middleware receives a `*zinc.Context`. It is the one object you need for a request: it reads input, writes the response, carries values between middleware and handlers, and exposes the underlying `http.Request` and `http.ResponseWriter`.

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"id":    c.Param("id"),        // request data
		"route": c.FullPath(),         // "/users/{id}", the matched pattern
		"agent": c.GetHeader("User-Agent"),
	})
})
```

The guides cover each area in depth: [Request Data](/guide/request/) for reading input, [Binding](/guide/binding/) for structs, and [Responses](/guide/responses-and-rendering/) for writing output. This page covers what is specific to the context itself.

## Share values between middleware and handlers

Middleware often discovers something that later handlers need, such as the current user. Store it with `Set` and read it with a typed getter.

```go
func loadUser(c *zinc.Context) error {
	user, err := sessions.User(c.Context(), c.GetHeader("Authorization"))
	if err != nil {
		return zinc.ErrUnauthorized
	}
	c.Set("user", user)
	return c.Next()
}

app.Get("/me", loadUser, func(c *zinc.Context) error {
	user := c.MustGet("user").(*User)
	return c.JSON(user)
})
```

| Getter | Returns |
|---|---|
| `Get(key)` | `(any, bool)` |
| `MustGet(key)` | `any`, and panics when missing. Use it only when a missing value is a bug. |
| `GetString`, `GetBool`, `GetInt`, `GetInt64`, `GetFloat64` | The typed value, or its zero value |
| `GetStringSlice`, `GetStringMap`, `GetStringMapString` | The typed collection, or `nil` |

Values live only for the current request.

## Route information

`c.FullPath()` returns the matched pattern, such as `/users/{id}`. It is better than the raw path for metrics and logs because it does not explode into one label per ID. `c.Route()` returns the full `RouteInfo`, including the route's name.

## Lifetime and goroutines

Zinc pools contexts, so a `*zinc.Context` is valid only until its handler returns. After that, the same object serves another request.

:::danger[Never keep the context]
Do not store `*zinc.Context`, pass it to a goroutine, or use it after the handler returns. The same applies to its response writer and request body.
:::

Copy what background work needs, then pass the copies:

```go
app.Post("/reports/{id}", func(c *zinc.Context) error {
	job := ReportJob{
		ID:        c.Param("id"),
		RequestID: c.RequestID(),
	}
	go reports.Build(context.WithoutCancel(c.Context()), job)
	return c.Status(zinc.StatusAccepted).JSON(zinc.Map{"queued": job.ID})
})
```

Choose the context deliberately:

- `c.Context()` is cancelled when the request ends. Use it for work that should stop if the client goes away.
- `context.WithoutCancel(c.Context())` keeps request values such as trace IDs but ignores cancellation. Use it for work that must finish after the response is sent.

## Using Zinc from existing handlers

`app.AcquireContext(w, r)` and `app.ReleaseContext(c)` let adapter code create a context around a `ResponseWriter` and `*http.Request` it already owns. Applications almost never need them.

## Next steps

- [Context API](/api/context/) for every method, grouped by purpose.
- [Groups and Middleware](/guide/groups-and-middleware/) for how `c.Next()` runs the chain.
