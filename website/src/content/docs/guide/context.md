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
		"agent": c.Header("User-Agent"),
	})
})
```

The guides cover each area in depth: [Request Data](/guide/request/) for reading input, [Binding](/guide/binding/) for structs, and [Responses](/guide/responses-and-rendering/) for writing output. This page covers what is specific to the context itself.

## Share values between middleware and handlers

Middleware often discovers something that later handlers need, such as the current user. Store it with `Set` and read it back with `zinc.Value` or `zinc.MustValue`, which check its type.

```go
func loadUser(c *zinc.Context) error {
	user, err := sessions.User(c.Context(), c.Header("Authorization"))
	if err != nil {
		return zinc.ErrUnauthorized
	}
	c.Set("user", user)
	return c.Next()
}

app.Get("/me", loadUser, func(c *zinc.Context) error {
	user := zinc.MustValue[*User](c, "user")
	return c.JSON(user)
})
```

| Getter | Returns |
|---|---|
| `c.Get(key)` | `(any, bool)` |
| `zinc.Value[T](c, key)` | `(T, bool)`: the value if it is stored and has type `T` |
| `zinc.MustValue[T](c, key)` | `T`, and panics when it is missing or has another type. Use it for values an earlier middleware always sets. |

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
		RequestID: c.Header(zinc.HeaderXRequestID),
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
