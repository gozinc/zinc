---
title: Errors
description: Return errors from handlers, let domain errors choose their status, and shape every error response in one place.
---

Handlers and middleware fail by returning an error. Zinc sends every returned error to one error handler, which turns it into a response. Handlers stay short, and error responses stay consistent across the whole app.

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	user, err := users.Find(c.Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(user)
})
```

## What the default handler sends

The default handler writes a JSON body with the status and a client-safe message:

```bash
curl -i http://localhost:8080/users/nope
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":404,"message":"user not found"}}
```

| Returned error | Status | Message |
|---|---|---|
| A Zinc HTTP error, such as `zinc.NotFound("user not found")` | Its code | Its message, or the status text |
| An error that implements `StatusCoder` | Its `StatusCode()` | Its `Error()` text for 4xx; the status text for 5xx |
| A binding failure (`*zinc.BindError`) | 400 | What was wrong, plus the failing field in `fields` |
| A validator failure (`*zinc.ValidationError`) | 422 | `validation failed`, plus `fields` when the validator provides them |
| Any other error | 500 | `Internal Server Error`; the error text is never sent |

Errors can be wrapped with `fmt.Errorf("...: %w", err)`; Zinc finds the status through the chain. An explicit Zinc HTTP error anywhere in the chain wins, which is why an oversized body still answers 413 even though it surfaces as a binding error.

## HTTP errors

Return one of the constructors for the statuses handlers use most:

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

For any other status, use `zinc.NewError(code, message)`. When the status text is message enough, return a predefined error: `zinc.ErrNotFound`, `zinc.ErrForbidden`, and one for every status.

Each builder method returns a copy, so the predefined errors are never modified:

```go
return zinc.Conflict("email already registered").
	Wrap(err).                           // the underlying error, for logs; never sent
	WithDetail("field", "email").        // added to the body under "details"
	WithHeader("Retry-After", "30")      // a response header
```

```json
{"error":{"status":409,"message":"email already registered","details":{"field":"email"}}}
```

HTTP errors match by status: `errors.Is(err, zinc.ErrNotFound)` is true for any 404, including copies and errors that wrap one.

## Errors that know their status

A domain error can choose its own status by implementing `zinc.StatusCoder`. Handlers then return it unchanged, with no mapping:

```go
type ErrUserNotFound struct{ ID string }

func (e ErrUserNotFound) Error() string { return "user " + e.ID + " not found" }
func (ErrUserNotFound) StatusCode() int { return http.StatusNotFound }
```

For 4xx statuses the error's own message is sent, so write it for clients. For 5xx statuses only the status text is sent.

## Binding and validation errors

Return binding errors as they are. Zinc answers 400 and names the field that failed, without decoder details:

```go
if err := c.Bind().Query(&in); err != nil {
	return err
}
```

```json
{"error":{"status":400,"message":"invalid query parameter","fields":{"page":"must be an integer"}}}
```

Failures from the configured `Validator` become `*zinc.ValidationError` and answer 422. If the validator's error has a `Fields() map[string]string` method, the body lists them:

```json
{"error":{"status":422,"message":"validation failed","fields":{"email":"required"}}}
```

A validator that returns its own HTTP error or `StatusCoder` keeps that status. See [Binding](/guide/binding/) for adapting a validation library.

## Stopping a middleware chain

Middleware stops a request by returning an error instead of calling `c.Next()`:

```go
func requireAPIKey(c *zinc.Context) error {
	if c.GetHeader("X-API-Key") == "" {
		return zinc.Unauthorized("missing API key")
	}
	return c.Next()
}
```

To stop with a body of your own instead, write the response and return its result: `return c.Status(zinc.StatusForbidden).JSON(body)`.

## A custom error handler

Most applications only need to log server errors. Wrap the default handler, and keep its responses:

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

`zinc.StatusCode(err)` reports the status the default handler would send. To change the body format entirely, write your own handler and use `errors.As` for the types above. `zinc.TextErrors` is a ready-made handler that sends plain-text bodies, as Zinc 0.3 did.

## Panics

Without help, Go's HTTP server catches a handler panic, logs it, and drops the connection, so the client gets no response at all. Add [Recover](/middleware/recover/) early in the middleware chain, and panics become `500` responses that flow through your error handler like any other error.

## Next steps

- [Binding](/guide/binding/) for the errors that binding and validation return.
- [Recover](/middleware/recover/) for turning panics into errors.
- [Errors API](/api/errors/) for the full reference.
