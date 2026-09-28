---
title: Context
description: Use the request's Context to read input, write the response, pass values from middleware to handlers, and run background work safely.
---

Every handler and middleware gets a `*zinc.Context` for the request it's serving. You use it to read the request, write the response, and pass values from middleware to handlers.

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"id":    c.Param("id"),          // from the path
		"route": c.FullPath(),           // the pattern that matched
		"agent": c.Header("User-Agent"), // from the headers
	})
})
```

```bash
curl localhost:8080/users/42
# {"agent":"curl/8.7.1","id":"42","route":"/users/{id}"}
```

Most of what the context does has its own page: [Request Data](/guide/request/) for reading input, [Binding](/guide/binding/) for filling structs, and [Responses](/guide/responses-and-rendering/) for writing output. This page covers what belongs to the context itself.

## Pass values from middleware to handlers

Middleware often works out something a later handler needs, such as the signed-in user. Store it with `c.Set`, then read it back with `zinc.MustValue` or `zinc.Value`, which also check its type:

```go
type ctxKey string

const userKey ctxKey = "user"

func loadUser(c *zinc.Context) error {
	user, err := sessions.User(c.Context(), c.Header("Authorization")) // your session store
	if err != nil {
		return zinc.ErrUnauthorized
	}
	c.Set(userKey, user)
	return c.Next()
}

app.Get("/me", loadUser, func(c *zinc.Context) error {
	user := zinc.MustValue[*User](c, userKey)
	return c.JSON(user)
})
```

```bash
curl -H 'Authorization: Bearer ada' localhost:8080/me
# {"id":1,"name":"Ada"}

curl localhost:8080/me
# {"error":{"status":401,"message":"Unauthorized"}}
```

Pick the getter by how sure you are the value is there:

| Getter | Returns |
|---|---|
| `zinc.MustValue[T](c, key)` | `T`. Panics if the value is missing or has another type. Use it for values an earlier middleware always sets. |
| `zinc.Value[T](c, key)` | `(T, bool)`. `false` if the value is missing or has another type. |
| `c.Get(key)` | `(any, bool)`, with no type check. |

Values last for the current request only. The next request starts with none.

:::tip[Use your own key type]
A key can be any comparable value. A plain string like `"user"` can clash with a key another package picked. A key of your own type, like `userKey` above, never matches anyone else's.
:::

## Read the matched route

`c.FullPath()` returns the pattern that matched, such as `/users/{id}`, not the path that was requested. Use it as the label in metrics and logs: every request to that route gets the same label, however many IDs there are.

`c.Route()` returns the route's full `RouteInfo`:

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	r := c.Route()
	return c.JSON(zinc.Map{"name": r.Name, "method": r.Method, "path": r.Path, "params": r.Params})
}).Name("users.show")
```

```bash
curl localhost:8080/users/5
# {"method":"GET","name":"users.show","params":["id"],"path":"/users/{id}"}
```

## Run work in the background safely

Zinc reuses context objects. Once your handler returns, the same `*zinc.Context` starts serving another request.

:::danger[Don't keep the context]
Don't store a `*zinc.Context`, pass it to a goroutine, or use it after the handler returns. The same goes for its response writer and request body. Code that does may read another request's data.
:::

Copy what the background work needs into plain values first, then pass those:

```go
app.Post("/reports/{id}", func(c *zinc.Context) error {
	job := ReportJob{ // your job type
		ID:        c.Param("id"),
		RequestID: c.Header(zinc.HeaderXRequestID),
	}
	go reports.Build(context.WithoutCancel(c.Context()), job) // your report builder
	return c.Status(zinc.StatusAccepted).JSON(zinc.Map{"queued": job.ID})
})
```

```bash
curl -X POST localhost:8080/reports/r1
# {"queued":"r1"}
```

Pick the `context.Context` by whether the work should outlive the request:

- `c.Context()` is cancelled when the request ends. Use it for work that should stop if the client goes away.
- `context.WithoutCancel(c.Context())` keeps request values such as trace IDs, but is never cancelled. Use it for work that must finish after the response is sent.

## Good to know

### MustValue panics

A missing value makes `zinc.MustValue` panic with a message such as `zinc: no request value for key user`. Add the [recover middleware](/middleware/recover/) if you want a panic to become a `500` response.

### Create a context outside a route

`app.AcquireContext(w, r)` wraps a `ResponseWriter` and `*http.Request` you already have in a `*zinc.Context`, and `app.ReleaseContext(c)` hands it back when you're done. They're for adapter code that calls Zinc from somewhere else. An app that only registers routes doesn't need them.

## Next steps

- [Context API](/api/context/): every method on the context, grouped by task.
- [Groups and Middleware](/guide/groups-and-middleware/): how `c.Next()` runs the rest of the chain.
- [Request Data](/guide/request/): read path, query, header and body values.
