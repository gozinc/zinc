---
title: Your First Route
description: Build one small endpoint end to end, reading params, query values, JSON input, and returning errors.
---

The [Quickstart](/guide/quickstart/) got a server running. This page builds a slightly more realistic endpoint and introduces the four things nearly every handler does: read the path, read the query, decode a body, and fail cleanly.

## The handler shape

Every route handler and every middleware in Zinc has the same signature:

```go
func(c *zinc.Context) error
```

`c` holds the request and writes the response. Return `nil` after writing a response, or return an error and let Zinc turn it into one.

## Read the path and query

Brace segments in a route pattern become parameters. Query values come from the URL.

```go
app.Get("/teams/{team}/members", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"team": c.Param("team"),
		"role": c.QueryOr("role", "any"),
	})
})
```

```bash
curl 'http://localhost:8080/teams/platform/members?role=admin'
# {"role":"admin","team":"platform"}
```

## Decode a request body

Declare a struct for the input, then bind into it. Struct tags say where each field comes from.

```go
type CreateMember struct {
	Team  string `path:"team"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

app.Post("/teams/{team}/members", func(c *zinc.Context) error {
	var in CreateMember
	if err := c.Bind().All(&in); err != nil {
		return zinc.ErrBadRequest.WithMessage("invalid request").WithCause(err)
	}
	if in.Name == "" {
		return zinc.ErrUnprocessableEntity.WithMessage("name is required")
	}
	return c.Status(zinc.StatusCreated).JSON(in)
})
```

`Bind().All` fills `Team` from the path and `Name` and `Email` from the JSON body. When the body is malformed it returns an error, which the handler turns into a `400 Bad Request`.

:::caution[Wrap binding errors]
Return binding errors as `zinc.ErrBadRequest.WithCause(err)`, as above. A raw binding error is not an HTTP error, so Zinc's default handler answers `500 Internal Server Error` for what is really a client mistake. You can also map binding errors once in a [custom error handler](/guide/errors/#map-binding-errors-to-400).
:::

## Return errors

Handlers fail by returning an error. Zinc's predefined errors carry a status code and an optional client-facing message.

```go
app.Get("/members/{id}", func(c *zinc.Context) error {
	member, err := store.Find(c.Param("id"))
	if errors.Is(err, ErrNoMember) {
		return zinc.ErrNotFound.WithMessage("member not found")
	}
	if err != nil {
		return err // becomes 500 Internal Server Error, details stay private
	}
	return c.JSON(member)
})
```

```bash
curl -i http://localhost:8080/members/nope
# HTTP/1.1 404 Not Found
# member not found
```

Any error that is not a Zinc HTTP error becomes a plain `500 Internal Server Error`, so internal details never leak to clients. [Errors](/guide/errors/) shows how to replace the plain-text body with a JSON envelope.

## Registration is checked at startup

Route patterns are validated when you register them. A typo such as `/users/:id` or a conflicting route panics when the program starts, not on the first request, so you never check an error after `app.Get`.

## Next steps

- [Routing](/guide/routing/) covers every pattern form, groups, and precedence.
- [Binding](/guide/binding/) covers every input source and validation.
- [Errors](/guide/errors/) covers custom messages, metadata, and JSON error responses.
