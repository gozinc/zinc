---
title: Errors
description: Reference for zinc.HTTPError, the predefined status errors, and the ErrorHandler type.
---

Return an `*HTTPError` from a handler to send a specific status. See the [Errors guide](/guide/errors/) for patterns.

## HTTPError

```go
type HTTPError struct {
	Code    int         // HTTP status code
	Message string      // client-facing message; defaults to the status text
	Cause   error       // underlying error, for logs; never sent by default
	Meta    zinc.Map    // extra data for custom error handlers
	Headers http.Header // headers added to the response
}
```

| Method | Returns |
|---|---|
| `WithMessage(msg)` | A copy with a client-facing message |
| `WithCause(err)` | A copy that wraps `err`, so `errors.Is` and `errors.As` see it |
| `WithMeta(key, value)` | A copy with one metadata value |
| `WithHeader(key, value)` | A copy that adds a response header |
| `Error()` | The message, or the status text when there is none |
| `Is(target)` | Matches another `*HTTPError` with the same code, and the same message if the target has one, so `errors.Is(err, zinc.ErrNotFound)` holds for any 404 |

Builders always return copies, so the predefined errors below are safe to share.

## Constructors

```go
err := zinc.NewError(zinc.StatusTeapot)
```

## Predefined errors

There is one `Err` value for each standard 4xx and 5xx status, named after it:

| Common | |
|---|---|
| `ErrBadRequest` | 400 |
| `ErrUnauthorized` | 401 |
| `ErrForbidden` | 403 |
| `ErrNotFound` | 404 |
| `ErrMethodNotAllowed` | 405 |
| `ErrConflict` | 409 |
| `ErrRequestEntityTooLarge` | 413 |
| `ErrUnsupportedMediaType` | 415 |
| `ErrUnprocessableEntity` | 422 |
| `ErrTooManyRequests` | 429 |
| `ErrInternalServerError` | 500 |
| `ErrServiceUnavailable` | 503 |

## ErrorHandler

```go
type ErrorHandler func(c *zinc.Context, err error)
```

Set `Config.ErrorHandler` to control how every returned error becomes a response. The default writes `HTTPError`s as plain text with their status, binding errors as generic `400 Bad Request` (preserving HTTP causes such as 413), and other errors as `500 Internal Server Error`.
