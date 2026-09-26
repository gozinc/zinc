---
title: Typed Handlers
description: Declare a handler's input and output as Go types, and let Zinc bind, validate, and respond.
---

A typed handler's signature is its request contract. Zinc binds the input from the request, validates it, calls your function, and writes the output as JSON:

```go
type CreateUser struct {
	OrgID  string `path:"org"`
	DryRun bool   `query:"dry_run"`
	Email  string `json:"email" validate:"required,email"`
}

api.Post("/orgs/{org}/users", zinc.Typed(func(c *zinc.Context, in CreateUser) (User, error) {
	return users.Create(c.Context(), in)
})).Status(zinc.StatusCreated)
```

`zinc.Typed` turns the function into an ordinary handler, so it works anywhere a handler does, including after route middleware. Handlers that don't use it are unaffected.

## Input

The input is a struct. Each field comes from where its tag says:

| Tag | Source |
|---|---|
| `path:"org"` | A route parameter |
| `query:"dry_run"` | The query string |
| `header:"X-Trace-ID"` | A request header |
| `json:"email"` (or `xml`, `form`) | The body |

Only tagged fields bind; see [Binding](/guide/binding/). The body is decoded by its `Content-Type`, so a typed handler also accepts any format you [configure a decoder for](/guide/customization/#body-formats). That decoder reads its own library's tags: a struct that accepts JSON and YAML bodies needs both `json:"email"` and `yaml:"email"`.

The binding plan for the type is prepared when the route is registered. Use `struct{}` for a handler that takes no input.

What happens when the input is wrong:

| Problem | Response |
|---|---|
| A value doesn't parse, such as `?page=two` | `400` naming the field: `{"fields":{"page":"must be an integer"}}` |
| The configured `Validator` rejects the input | `422` with the validator's fields |

Your function only runs with valid input.

## Output

The output is written as JSON with status `200`. Declare another success status on the route:

```go
api.Post("/users", zinc.Typed(createUser)).Status(zinc.StatusCreated) // 201
```

For a response without a body, return `zinc.NoContent`. The route answers `204`, unless you declare another status:

```go
api.Delete("/users/{id}", zinc.Typed(func(c *zinc.Context, in UserID) (zinc.NoContent, error) {
	return zinc.NoContent{}, users.Delete(c.Context(), in.ID)
}))

app.Post("/jobs", zinc.Typed(enqueue)).Status(zinc.StatusAccepted) // 202, no body
```

A status the function sets with `c.Status(...)` takes precedence, and a function that writes the response itself, for example with `c.Redirect`, has its output ignored.

## Errors

Return an error, and it goes to the error handler like any other. Constructors and domain errors that implement `StatusCoder` work as usual:

```go
func getUser(c *zinc.Context, in UserID) (User, error) {
	user, err := users.Find(c.Context(), in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, zinc.NotFound("user not found")
	}
	return user, err
}
```

## The context is still there

The function receives the `*zinc.Context`, so everything else remains available: `c.Context()` for cancellation, `zinc.Value` for values from middleware, `c.SetHeader` for a `Location` header after a create.

## Performance

A typed handler costs the same as the equivalent hand-written handler: the same allocations and the same time, within measurement noise. The type checks happen once, when the route is registered.

## Next steps

- [Typed CRUD API](/cookbook/typed-crud/) builds a complete resource with typed handlers.
- [Binding](/guide/binding/) covers tags, supported types, and validators.
- [Errors](/guide/errors/) covers the error responses.
