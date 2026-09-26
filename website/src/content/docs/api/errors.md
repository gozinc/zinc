---
title: Errors
description: Reference for zinc.HTTPError, the error constructors, StatusCoder, ValidationError, and the error handlers.
---

Return an error from a handler, and the application's error handler turns it into a response. See the [Errors guide](/guide/errors/) for patterns.

## Constructors

| Function | Status |
|---|---|
| `BadRequest(msg)` | 400 |
| `Unauthorized(msg)` | 401 |
| `Forbidden(msg)` | 403 |
| `NotFound(msg)` | 404 |
| `Conflict(msg)` | 409 |
| `Gone(msg)` | 410 |
| `UnprocessableEntity(msg)` | 422 |
| `TooManyRequests(msg)` | 429 |
| `InternalServerError(msg)` | 500 |
| `ServiceUnavailable(msg)` | 503 |
| `NewError(code, msg...)` | Any status; without a message, the status text is used |

Each returns a new `*HTTPError`. An empty message falls back to the status text.

## HTTPError

```go
type HTTPError struct {
	Code    int         // HTTP status code
	Message string      // client-facing message; defaults to the status text
	Cause   error       // underlying error, for logs; never sent
	Details zinc.Map    // written to the body under "details"
	Headers http.Header // headers added to the response
}
```

| Method | Returns |
|---|---|
| `Wrap(err)` | A copy that wraps `err`, so `errors.Is` and `errors.As` see it |
| `WithDetail(key, value)` | A copy with one detail |
| `WithHeader(key, value)` | A copy that adds a response header |
| `Error()` | The message, or the status text when there is none |
| `StatusCode()` | The code; `HTTPError` is a `StatusCoder` |
| `Is(target)` | Matches another `*HTTPError` with the same code, and the same message if the target has one, so `errors.Is(err, zinc.ErrNotFound)` holds for any 404 |

Builders always return copies, so the predefined errors below are safe to share.

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

The others follow the same naming: `ErrPaymentRequired` (402), `ErrNotAcceptable` (406), `ErrProxyAuthRequired` (407), `ErrRequestTimeout` (408), `ErrGone` (410), `ErrLengthRequired` (411), `ErrPreconditionFailed` (412), `ErrRequestURITooLong` (414), `ErrRequestedRangeNotSatisfiable` (416), `ErrExpectationFailed` (417), `ErrTeapot` (418), `ErrMisdirectedRequest` (421), `ErrLocked` (423), `ErrFailedDependency` (424), `ErrTooEarly` (425), `ErrUpgradeRequired` (426), `ErrPreconditionRequired` (428), `ErrRequestHeaderFieldsTooLarge` (431), `ErrUnavailableForLegalReasons` (451), `ErrNotImplemented` (501), `ErrBadGateway` (502), `ErrGatewayTimeout` (504), `ErrHTTPVersionNotSupported` (505), `ErrVariantAlsoNegotiates` (506), `ErrInsufficientStorage` (507), `ErrLoopDetected` (508), `ErrNotExtended` (510), and `ErrNetworkAuthenticationRequired` (511).

Three errors are not statuses: `ErrResponseAlreadySent` reports a second write after the response was committed, and `ErrTemplateEngineNotConfigured`, `ErrTemplateNameRequired`, and `ErrTemplateNotFound` come from `c.Render`.

## StatusCoder

```go
type StatusCoder interface {
	StatusCode() int
}
```

Implement it on a domain error to choose its HTTP status. For 4xx statuses the default handler sends the error's `Error()` text; for 5xx statuses, only the status text. `HTTPError`, `BindError`, and `ValidationError` implement it.

```go
func StatusCode(err error) int
```

`StatusCode` reports the status the default handler sends for `err`: the code of a `*HTTPError` anywhere in its chain, otherwise the status of the first `StatusCoder`, otherwise 500. It returns 0 for `nil`.

## ValidationError

```go
type ValidationError struct {
	Err error // the validator's error
}
```

Binding wraps a `Validator` failure in `*ValidationError`, which answers 422. `Fields()` returns the validator error's field messages when it has a `Fields() map[string]string` method. A validator that returns an `*HTTPError` or a `StatusCoder` keeps that status instead.

## Error handlers

```go
type ErrorHandler func(c *zinc.Context, err error)
```

Set `Config.ErrorHandler` to control how every returned error becomes a response.

| Handler | Body |
|---|---|
| `DefaultErrorHandler` (the default) | `{"error":{"status":404,"message":"...","fields":{...},"details":{...}}}`; `fields` and `details` appear only when present |
| `TextErrors` | The message as plain text, as in Zinc 0.3 |

Both resolve the status the same way, and neither sends the text of an unknown error or of a 5xx `StatusCoder`. A custom handler usually wraps the default:

```go
cfg.ErrorHandler = func(c *zinc.Context, err error) {
	if zinc.StatusCode(err) >= 500 {
		slog.Error("request failed", "err", err)
	}
	zinc.DefaultErrorHandler(c, err)
}
```

`c.HandleError(err)` sends an error to the handler immediately. Middleware that observes the final response, such as a logger, uses it; handlers normally just return the error.
