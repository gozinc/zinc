---
title: Typed Handlers
description: Write a handler that takes a struct and returns a value, and let Zinc bind, validate and send the JSON.
---

A typed handler is a function that takes a struct and returns a value. Zinc fills the struct from the request, validates it, calls your function and sends the result as JSON. Use it for JSON API routes, where most handlers would otherwise start with the same bind-and-check lines.

```go
package main

import (
	"log"

	"github.com/0mjs/zinc"
	"github.com/go-playground/validator/v10"
)

// structValidator lets go-playground/validator check the `validate` tags.
type structValidator struct{ v *validator.Validate }

func (s structValidator) Validate(target any) error { return s.v.Struct(target) }

type CreateUser struct {
	OrgID  string `path:"org"`
	DryRun bool   `query:"dry_run"`
	Email  string `json:"email" validate:"required,email"`
}

type User struct {
	ID    int    `json:"id"`
	OrgID string `json:"org_id"`
	Email string `json:"email"`
}

func createUser(c *zinc.Context, in CreateUser) (User, error) {
	user := User{ID: 1, OrgID: in.OrgID, Email: in.Email}
	if in.DryRun {
		return user, nil
	}
	// save user in your data layer
	return user, nil
}

func main() {
	app := zinc.New(zinc.Config{Validator: structValidator{v: validator.New()}})

	app.Post("/orgs/{org}/users", zinc.Typed(createUser)).Status(zinc.StatusCreated)

	log.Fatal(app.Listen(":8080"))
}
```

```bash
curl -i -X POST localhost:8080/orgs/acme/users \
  -H 'Content-Type: application/json' -d '{"email":"ada@example.com"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
#
# {"id":1,"org_id":"acme","email":"ada@example.com"}

curl -X POST 'localhost:8080/orgs/acme/users?dry_run=yes' \
  -H 'Content-Type: application/json' -d '{"email":"ada@example.com"}'
# {"error":{"status":400,"message":"invalid query parameter","fields":{"dry_run":"must be a boolean"}}}

curl -X POST localhost:8080/orgs/acme/users \
  -H 'Content-Type: application/json' -d '{"email":"not-an-email"}'
# {"error":{"status":422,"message":"validation failed"}}
```

`zinc.Typed(createUser)` turns the function into an ordinary handler, so you can use it anywhere a handler goes, including after route middleware. `.Status(zinc.StatusCreated)` sets the status for a successful call; a status the function sets itself with `c.Status`, `200` included, wins over it. `createUser` only runs when the input is valid.

## Compare with a plain handler

This is the same route written without `zinc.Typed`:

```go
app.Post("/orgs/{org}/users", func(c *zinc.Context) error {
	var in CreateUser
	if err := c.Bind().All(&in); err != nil {
		return err
	}
	user, err := createUser(c, in)
	if err != nil {
		return err
	}
	return c.Status(zinc.StatusCreated).JSON(user)
})
```

The typed version does the same work: bind, return binding and validation errors, call your code, send JSON. What's left in your function is the part only you can write. It's also easier to test, because it's a plain function with a struct in and a value out.

## Say where each field comes from

The input is a struct, and each field's tag says where its value comes from:

| Tag | Source |
|---|---|
| `path:"org"` | A route parameter |
| `query:"dry_run"` | The query string |
| `header:"X-Trace-ID"` | A request header |
| `cookie:"session"` | A cookie |
| `json:"email"` (or `xml`, `form`) | The body |

Only tagged fields are filled. [Binding](/guide/binding/) covers the supported types and tag options.

The body is decoded by its `Content-Type`, so a typed handler also accepts any format you [add a decoder for](/guide/customization/). A decoder reads its own library's tags: to accept JSON and YAML bodies, give the field both `json:"email"` and `yaml:"email"`.

For a route with no input, use `struct{}`:

```go
func health(c *zinc.Context, _ struct{}) (zinc.Map, error) {
	return zinc.Map{"ok": true}, nil
}

app.Get("/health", zinc.Typed(health))
```

```bash
curl localhost:8080/health
# {"ok":true}
```

## What bad input gets back

Your function isn't called when the input is wrong. The client gets one of these instead:

| Problem | Response |
|---|---|
| A value doesn't parse, such as `?dry_run=yes` | `400`, naming the field: `"fields":{"dry_run":"must be a boolean"}` |
| The body isn't valid JSON | `400`: `{"error":{"status":400,"message":"invalid request body"}}` |
| The configured `Validator` rejects the input | `422`: `{"error":{"status":422,"message":"validation failed"}}` |

To list the failing fields in the `422` response, have your validator return an error with a `Fields()` method. [Binding](/guide/binding/) shows the adapter for go-playground/validator, which gives:

```json
{"error":{"status":422,"message":"validation failed","fields":{"Email":"failed email"}}}
```

:::note[No validator, no 422]
Zinc doesn't ship a validator. Without `Validator` in `zinc.Config`, `validate` tags are ignored and only parse errors are rejected.
:::

## Choose the success status

A typed handler answers `200` by default. Declare another success status on the route:

```go
app.Post("/users", zinc.Typed(createUser)).Status(zinc.StatusCreated) // 201
```

`Status` accepts only `2xx` codes. Anything else panics when the route is registered.

## Send no body

Return `zinc.NoContent` for a response without a body. The route answers `204` unless you declare another status:

```go
type UserID struct {
	ID int `path:"id"`
}

app.Delete("/users/{id}", zinc.Typed(func(c *zinc.Context, in UserID) (zinc.NoContent, error) {
	return zinc.NoContent{}, nil // delete user in.ID in your data layer
}))

app.Post("/jobs", zinc.Typed(enqueue)).Status(zinc.StatusAccepted) // 202, no body
```

```bash
curl -i -X DELETE localhost:8080/users/7
# HTTP/1.1 204 No Content

curl -X DELETE localhost:8080/users/abc
# {"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}
```

Here `enqueue` is any function of the form `func(*zinc.Context, T) (zinc.NoContent, error)`.

## Return an error

Return an error and it goes to the [error handler](/guide/errors/), as it would from any handler. Zinc's error constructors and your own errors with a `StatusCode()` method keep their status:

```go
func getUser(c *zinc.Context, in UserID) (User, error) {
	user, err := users.Find(c.Context(), in.ID) // your data layer
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, zinc.NotFound("user not found")
	}
	return user, err
}

app.Get("/users/{id}", zinc.Typed(getUser))
```

```bash
curl localhost:8080/users/8
# {"error":{"status":404,"message":"user not found"}}
```

## Use the context

Your function still gets the `*zinc.Context`, so you have everything a plain handler has. Use `c.Context()` to stop work when the client goes away, `zinc.Value` to read what middleware stored, and `c.SetHeader` to add headers:

```go
func createUser(c *zinc.Context, in CreateUser) (User, error) {
	user, err := users.Create(c.Context(), in) // your data layer
	if err != nil {
		return User{}, err
	}
	c.SetHeader(zinc.HeaderLocation, fmt.Sprintf("/orgs/%s/users/%d", in.OrgID, user.ID))
	return user, nil
}
```

```bash
curl -i -X POST localhost:8080/orgs/acme/users \
  -H 'Content-Type: application/json' -d '{"email":"ada@example.com"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
# Location: /orgs/acme/users/7
#
# {"id":7,"org_id":"acme","email":"ada@example.com"}
```

## When to use a plain handler

A typed handler always answers with JSON, or nothing. Write a plain `func(c *zinc.Context) error` for routes that send HTML, files, streams or server-sent events, or that need to choose the format from the `Accept` header. Both kinds of handler work side by side in the same app.

## Good to know

### Which status wins

A status you set inside the function with `c.Status(...)` wins over the one declared with `.Status(...)` on the route. If the function writes the response itself, for example with `c.Redirect`, Zinc ignores the returned value and sends what you wrote.

### Typed and plain handlers bind the same way

A typed handler binds exactly as `c.Bind().All` does: body, headers, cookies, query and path. A working plain handler converts to a typed one without changing what it reads.

### Mistakes show up at startup

`zinc.Typed` panics if the input type isn't a struct, or if the function is `nil`. Zinc also works out how to fill the struct at that point, so the first request doesn't pay for it.

### An empty body is allowed

A request with no body still binds path, query, header and cookie fields, then runs the validator. A `required` body field comes back as a `422`, not a `400`.

## Next steps

- [Typed CRUD API](/cookbook/typed-crud/): a complete resource built with typed handlers.
- [Binding](/guide/binding/): tags, supported types and validators.
- [Errors](/guide/errors/): how errors become responses, and how to change them.
