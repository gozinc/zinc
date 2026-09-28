---
title: Your First Route
description: Build one small endpoint end to end, reading params, query values, JSON input, and returning errors.
---

This page builds a small team-members endpoint, one step at a time. Each step covers something nearly every handler does: read the path, read the query, accept a JSON body, and fail with the right status. It picks up where the [Quickstart](/guide/quickstart/) left off.

## The handler shape

Every route handler and every middleware in Zinc has the same signature:

```go
func(c *zinc.Context) error
```

`c` holds the request and writes the response. Write a response and return `nil`, or return an error and Zinc turns it into a response.

## Read the path and query

```go
app.Get("/teams/{team}/members", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"team": c.Param("team"),
		"role": zinc.QueryOr(c, "role", "any"),
	})
})
```

```bash
curl 'http://localhost:8080/teams/platform/members?role=admin'
# {"role":"admin","team":"platform"}

curl http://localhost:8080/teams/platform/members
# {"role":"any","team":"platform"}
```

`{team}` in the pattern becomes a path parameter, read with `c.Param`. `zinc.QueryOr` reads a query value, and returns the fallback (`"any"`) when the value is missing.

## Accept a JSON body

Declare a struct for the input, then bind the request into it. Struct tags say where each field comes from:

```go
type CreateMember struct {
	Team  string `path:"team"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

app.Post("/teams/{team}/members", func(c *zinc.Context) error {
	var in CreateMember
	if err := c.Bind().All(&in); err != nil {
		return err // 400, naming the field when it can
	}
	if in.Name == "" {
		return zinc.UnprocessableEntity("name is required")
	}
	return c.Status(zinc.StatusCreated).JSON(zinc.Map{
		"team":  in.Team,
		"name":  in.Name,
		"email": in.Email,
	})
})
```

```bash
curl -i -X POST http://localhost:8080/teams/platform/members \
  -H 'Content-Type: application/json' \
  -d '{"name":"Ada","email":"ada@example.com"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
#
# {"email":"ada@example.com","name":"Ada","team":"platform"}
```

`Bind().All` fills `Team` from the path, and `Name` and `Email` from the JSON body.

A body can't change the team. Values from the path win over the body, so `{"team":"other"}` still binds `platform`.

When the body doesn't fit the struct, return the error unchanged. The client gets a `400` that names the field, without Go's decoder details:

```bash
curl -X POST http://localhost:8080/teams/platform/members \
  -H 'Content-Type: application/json' \
  -d '{"name":42}'
# {"error":{"status":400,"message":"invalid request body","fields":{"name":"must be a string"}}}

curl -X POST http://localhost:8080/teams/platform/members \
  -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com"}'
# {"error":{"status":422,"message":"name is required"}}
```

The first request fails binding. The second binds, then fails the `in.Name == ""` check. [Binding](/guide/binding/) shows how to plug in a validator and declare rules such as `required` on the struct instead.

## Return an error

A handler fails by returning an error. Zinc's error helpers, such as `zinc.NotFound`, carry a status code and a message the client sees:

```go
app.Get("/members/{id}", func(c *zinc.Context) error {
	member, err := store.Find(c.Param("id")) // store: your data layer
	if errors.Is(err, ErrNoMember) {         // ErrNoMember: your "not found" error
		return zinc.NotFound("member not found")
	}
	if err != nil {
		return err // 500, and the message stays on the server
	}
	return c.JSON(member)
})
```

```bash
curl -i http://localhost:8080/members/nope
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":404,"message":"member not found"}}
```

Any other error becomes a `500 Internal Server Error`. Its text is never sent, so internal details don't leak to clients:

```bash
# {"error":{"status":500,"message":"Internal Server Error"}}
```

[Errors](/guide/errors/) shows how your own error types can choose a status, and how to log server errors.

## Good to know

### Mistakes show up at startup

Zinc checks each route pattern when you register it. A typo such as `/users/:id`, or two routes that clash, panics as soon as the program starts. You find out before the first request arrives, and there's no error to check after `app.Get`.

## Next steps

- [Routing](/guide/routing/): every pattern form, groups, and which route wins.
- [Binding](/guide/binding/): every input source, and validation.
- [Errors](/guide/errors/): custom messages, details, and your own error format.
