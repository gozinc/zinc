---
title: Errors
description: Return errors from handlers, let domain errors choose their status, and shape every error response in one place.
---

When a handler or middleware fails, it returns an error and Zinc turns it into a response. You get the same JSON error format across the whole app without writing a response in every handler.

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	user, err := users.Find(c.Context(), c.Param("id")) // users is your data layer
	if err != nil {
		return err
	}
	return c.JSON(user)
})
```

Every returned error goes to one error handler. The default handler picks a status, writes a client-safe message, and never sends internal error text.

## Return a 404 (or any status)

Return one of the constructors for the statuses handlers use most:

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	return zinc.NotFound("user not found")
})
```

```bash
curl -i http://localhost:8080/users/nope
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":404,"message":"user not found"}}
```

The full set:

```go
return zinc.BadRequest("cursor is malformed")        // 400
return zinc.Unauthorized("token expired")             // 401
return zinc.Forbidden("admins only")                  // 403
return zinc.NotFound("widget not found")              // 404
return zinc.Conflict("email already registered")      // 409
return zinc.Gone("this export has expired")           // 410
return zinc.UnprocessableEntity("start after end")    // 422
return zinc.TooManyRequests("slow down")              // 429
return zinc.InternalServerError("try again later")    // 500
return zinc.ServiceUnavailable("maintenance")         // 503
```

For any other status, use `zinc.NewError`. When the status text is message enough, return a predefined error such as `zinc.ErrNotFound` or `zinc.ErrForbidden`. There's one for each standard 4xx and 5xx status.

```go
return zinc.NewError(zinc.StatusTeapot, "no coffee") // {"error":{"status":418,"message":"no coffee"}}
return zinc.ErrForbidden                             // {"error":{"status":403,"message":"Forbidden"}}
```

## Add details, headers and a cause

You'll often want more than a status and a message: the underlying error for your logs, extra fields for the client, or a header such as `Retry-After`. Chain them onto the error:

```go
return zinc.Conflict("email already registered").
	Wrap(err).                      // the underlying error, for logs; never sent
	WithDetail("field", "email").   // added to the body under "details"
	WithHeader("Retry-After", "30") // a response header
```

```bash
curl -i http://localhost:8080/signup
# HTTP/1.1 409 Conflict
# Content-Type: application/json; charset=utf-8
# Retry-After: 30
#
# {"error":{"status":409,"message":"email already registered","details":{"field":"email"}}}
```

Each call returns a new error and leaves the original alone. That's what makes it safe to build on shared errors like `zinc.ErrNotFound`: another request never sees your details.

To check for an HTTP error, compare by status. `errors.Is(err, zinc.ErrNotFound)` is true for any 404, including one with details added and one wrapped inside another error.

## Let your domain errors pick a status

Your own error types can choose their status by adding a `StatusCode() int` method (the `zinc.StatusCoder` interface). Handlers then return them unchanged, with no mapping code:

```go
type ErrUserNotFound struct{ ID string }

func (e ErrUserNotFound) Error() string { return "user " + e.ID + " not found" }
func (ErrUserNotFound) StatusCode() int { return http.StatusNotFound }
```

```bash
curl http://localhost:8080/users/42
# {"error":{"status":404,"message":"user 42 not found"}}
```

:::caution[4xx messages reach the client]
For a 4xx status, the error's own `Error()` text is sent, so write it for clients. For a 5xx status, only the status text is sent, such as `Service Unavailable`.
:::

Wrapping keeps the status. `fmt.Errorf("load user: %w", ErrUserNotFound{ID: "7"})` still answers 404 with `user 7 not found`.

## What the default handler sends

| Returned error | Status | Message sent |
|---|---|---|
| A Zinc HTTP error, such as `zinc.NotFound(...)` | Its code | Its message, or the status text if it has none. This includes 5xx: `zinc.InternalServerError("try again later")` sends `try again later`. |
| An error with a `StatusCode()` method | That status | Its `Error()` text for 4xx; the status text for 5xx |
| A binding failure (`*zinc.BindError`) | 400 | What was wrong, plus the failing field under `fields` |
| A validation failure (`*zinc.ValidationError`) | 422 | `validation failed`, plus `fields` when the validator provides them |
| Any other error | 500 | `Internal Server Error`. The error text is never sent. |

Zinc looks through wrapped errors (`fmt.Errorf("...: %w", err)`, `errors.Join`) to find the status.

## Binding and validation errors

Return binding errors as they are. Zinc answers 400 and names the field that failed, without decoder details:

```go
var in struct {
	Page int `query:"page"`
}
if err := c.Bind().Query(&in); err != nil {
	return err
}
```

```bash
curl "http://localhost:8080/users?page=abc"
# {"error":{"status":400,"message":"invalid query parameter","fields":{"page":"must be an integer"}}}
```

When your configured validator rejects a value, the response is a 422. The `fields` come from your validator; with the go-playground adapter in [Binding](/guide/binding/#validation) they look like this:

```bash
curl -X POST http://localhost:8080/signup -H 'Content-Type: application/json' -d '{"email":""}'
# {"error":{"status":422,"message":"validation failed","fields":{"Email":"failed required"}}}
```

[Binding](/guide/binding/#validation) shows how to plug in a validation library.

## Stop a request from middleware

Middleware stops a request by returning an error instead of calling `c.Next()`:

```go
func requireAPIKey(c *zinc.Context) error {
	if c.Header("X-API-Key") == "" {
		return zinc.Unauthorized("missing API key")
	}
	return c.Next()
}
```

```bash
curl http://localhost:8080/
# {"error":{"status":401,"message":"missing API key"}}
```

To send a body of your own instead, write it and return the result: `return c.Status(zinc.StatusForbidden).JSON(body)`.

## Log server errors

Most apps only need to add logging. Wrap the default handler and keep its responses:

```go
app := zinc.New(zinc.Config{
	ErrorHandler: func(c *zinc.Context, err error) {
		if zinc.StatusCode(err) >= 500 {
			slog.ErrorContext(c.Context(), "request failed", "route", c.FullPath(), "err", err)
		}
		zinc.DefaultErrorHandler(c, err)
	},
})
```

A handler that returns `fmt.Errorf("build report: %w", err)` now logs the full error while the client sees only the status text:

```text
level=ERROR msg="request failed" route=/reports/{id} err="build report: disk full"
```

```bash
curl http://localhost:8080/reports/9
# {"error":{"status":500,"message":"Internal Server Error"}}
```

`zinc.StatusCode(err)` returns the status the default handler would send: 500 for an unknown error, and 0 for `nil`.

:::tip[Send errors to a tracker]
The same `if` is the place to report to Sentry or another error tracker. You have the request's context, route and full error in one spot.
:::

## Change the error format

To send a different body shape, write your own `ErrorHandler` and use `errors.As` to check for `*zinc.HTTPError`, `*zinc.BindError` and `*zinc.ValidationError`.

For plain-text bodies, use the built-in `zinc.TextErrors` handler:

```go
app := zinc.New(zinc.Config{ErrorHandler: zinc.TextErrors})
```

```bash
curl -i http://localhost:8080/users/nope
# HTTP/1.1 404 Not Found
# Content-Type: text/plain; charset=utf-8
#
# user not found
```

It picks statuses and hides internal messages the same way as the default handler.

## Recover from panics

Without help, Go's HTTP server catches a handler panic, logs it, and drops the connection, so the client gets no response at all. Add [Recover](/middleware/recover/) early in the middleware chain:

```go
app.Use(recover.New())
```

```bash
curl http://localhost:8080/panic
# {"error":{"status":500,"message":"Internal Server Error"}}
```

The panic becomes a 500 error that goes through your error handler like any other, so the logging above records it too.

## Good to know

### An explicit HTTP error wins

If a Zinc HTTP error appears anywhere in the wrapped chain, its status is used, even when an outer error has its own `StatusCode()`. That's why a body over the size limit answers 413, though it comes back from binding:

```bash
# {"error":{"status":413,"message":"Request Entity Too Large"}}
```

### A validator can choose its own status

If your validator returns a Zinc HTTP error or an error with a `StatusCode()` method, that status is kept and the error isn't wrapped as a `*zinc.ValidationError`.

### Statuses outside 400–599

A `StatusCode()` outside 400–599 is ignored, and the error answers 500.

### Nothing is sent after the response starts

If a handler has already written a response and then returns an error, the error handler writes nothing. The client keeps the response it already has, with its original status.

### Plain-text errors leave out fields

`zinc.TextErrors` sends only the status text for binding and validation errors, such as `Bad Request`. Use the JSON default if clients need to know which field failed.

## Next steps

- [Binding](/guide/binding/): the errors that binding and validation return, and how to plug in a validator.
- [Recover](/middleware/recover/): turn panics into errors.
- [Errors API](/api/errors/): the full reference.
