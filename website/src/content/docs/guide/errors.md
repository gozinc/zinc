---
title: Errors
description: Return HTTP errors from handlers, attach messages and metadata, and shape every error response in one place.
---

Handlers and middleware fail by returning an error. Zinc sends every returned error to one error handler, which turns it into a response. Handlers stay short, and error responses stay consistent across the whole app.

```go
app.Get("/users/{id}", func(c *zinc.Context) error {
	user, err := users.Find(c.Context(), c.Param("id"))
	if errors.Is(err, ErrUserNotFound) {
		return zinc.ErrNotFound.WithMessage("user not found")
	}
	if err != nil {
		return err
	}
	return c.JSON(user)
})
```

## What the default handler sends

| Returned error | Response |
|---|---|
| A Zinc HTTP error, such as `zinc.ErrNotFound` | Its status code, with its message as a plain-text body |
| An error that wraps a Zinc HTTP error | The same, found with `errors.As` |
| `*zinc.BindError` without an HTTP cause | `400 Bad Request`, without decoder details |
| Any other error | `500 Internal Server Error`, without the error text |

Unknown errors never leak their message to clients, so returning `err` straight from a database call is safe. It is just not very informative. Log it, or map it in a [custom handler](#a-custom-error-handler).

## HTTP errors

Zinc predefines an error for each HTTP status: `zinc.ErrBadRequest`, `zinc.ErrUnauthorized`, `zinc.ErrForbidden`, `zinc.ErrNotFound`, `zinc.ErrConflict`, `zinc.ErrUnprocessableEntity`, `zinc.ErrTooManyRequests`, `zinc.ErrInternalServerError`, and the rest. Create one for any code with `zinc.NewError(code)`.

Each builder method returns a copy, so the shared values are never modified:

```go
return zinc.ErrBadRequest.
	WithMessage("email is invalid").     // client-facing text
	WithCause(err).                      // the underlying error, for logs; never sent
	WithMeta("field", "email").          // extra data for a custom handler
	WithHeader("X-Error-Code", "E1042")  // a response header
```

The default handler sends the message and headers. `Meta` and `Cause` are for your own handler and logs.

## Stopping a middleware chain

Middleware can stop a request by returning an error instead of calling `c.Next()`:

```go
func requireAPIKey(c *zinc.Context) error {
	if c.GetHeader("X-API-Key") == "" {
		return zinc.ErrUnauthorized
	}
	return c.Next()
}
```

`c.AbortWithStatus(code)` is shorthand for returning `zinc.NewError(code)`. To stop with a body rather than an error, write the response and return its result: `return c.AbortWithJSON(zinc.StatusForbidden, body)`.

## A custom error handler

Replace the default handler to send JSON, log failures, or report them to an error tracker. This one gives every error the same envelope:

```go
cfg := zinc.DefaultConfig
cfg.ErrorHandler = func(c *zinc.Context, err error) {
	status, message := zinc.StatusInternalServerError, "internal server error"

	var httpErr *zinc.HTTPError
	if errors.As(err, &httpErr) {
		status, message = httpErr.Code, httpErr.Error()
	} else {
		slog.Error("request failed", "path", c.Path(), "err", err)
	}

	_ = c.Status(status).JSON(zinc.Map{"error": message})
}
app := zinc.NewWithConfig(cfg)
```

```bash
curl -i http://localhost:8080/users/nope
# HTTP/1.1 404 Not Found
# Content-Type: application/json; charset=utf-8
#
# {"error":"user not found"}
```

:::caution[Start from DefaultConfig]
Copy `zinc.DefaultConfig` and change fields, as above. A bare `zinc.Config{...}` literal leaves `AutoHead`, `AutoOptions`, `HandleMethodNotAllowed`, and the route cache switched off, because their zero values are `false` and `0`.
:::

### Map binding errors to 400

The default handler maps `*zinc.BindError` to a generic `400 Bad Request`, while preserving HTTP causes such as a body limit (413). Known JSON syntax/type errors are binding errors; opaque codec failures and invalid JSON destinations remain internal errors (500). Customize the response format once in your error handler:

```go
cfg.ErrorHandler = func(c *zinc.Context, err error) {
	status, message := zinc.StatusInternalServerError, "internal server error"

	var httpErr *zinc.HTTPError
	var bindErr *zinc.BindError
	switch {
	case errors.As(err, &httpErr): // check first: an oversized body is a BindError wrapping a 413
		status, message = httpErr.Code, httpErr.Error()
	case errors.As(err, &bindErr):
		status, message = zinc.StatusBadRequest, "invalid "+bindErr.Source
	default:
		slog.Error("request failed", "path", c.Path(), "err", err)
	}

	_ = c.Status(status).JSON(zinc.Map{"error": message})
}
```

## Panics

Without help, Go's HTTP server catches a handler panic, logs it, and drops the connection, so the client gets no response at all. Add [Recover](/middleware/recover/) early in the middleware chain, and panics become `500` responses that flow through your error handler like any other error.

## Next steps

- [Binding](/guide/binding/) for the errors that binding and validation return.
- [Recover](/middleware/recover/) for turning panics into errors.
- [Errors API](/api/errors/) for the full `HTTPError` type.

HTTP errors match by status: `errors.Is(err, zinc.ErrUnauthorized)` is true for any 401, including copies made with `WithMessage` and errors that wrap one. A target that carries a message also requires that message. The cause attached with `WithCause` stays reachable through `errors.Is` and `errors.As`. Validation errors are application-defined: return an HTTP error from the validator or map your validation type in a custom error handler.
