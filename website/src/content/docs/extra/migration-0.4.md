---
title: Migrating to Zinc 0.4
description: Upgrade a Zinc 0.3 application to 0.4, which reshapes the public API around shorter handlers and safer defaults.
slug: extra/migration-0.4
---

:::note[In progress]
Zinc 0.4 is under development. This guide grows with each change, and it is complete when 0.4.0 is released.
:::

Zinc 0.4 simplifies the public API and fixes several defaults that could silently leave an application unprotected. Work through the checklist, then read the sections that apply to you.

## Checklist

| If your app uses | What changes | Section |
|---|---|---|
| `group.Use` after registering routes on that group | It panics at startup | [Group middleware order](#group-middleware-order) |
| Binding into structs with untagged fields | Path, query, header, and form values bind only to tagged fields | [Binding requires tags](#binding-requires-tags) |
| `errors.Is` against `zinc.Err*` values | Copies and wrapped errors now match by status | [Matching HTTP errors](#matching-http-errors) |
| `c.OriginalURL()` after a rewrite | It returns the URL as received | [Original URL](#original-url) |
| `c.SSE` with manual flushing | Each event is flushed for you, and streams outlive `WriteTimeout` | [Server-sent events](#server-sent-events) |
| Clients that read error bodies | Errors are JSON by default | [JSON error bodies](#json-error-bodies) |
| `WithMessage`, `WithCause`, `WithMeta`, `Meta` | Replaced by constructors, `Wrap`, and `WithDetail` | [Building errors](#building-errors) |
| A `Validator` | Failures answer 422 instead of 500 | [Validation errors](#validation-errors) |
| `c.Fail`, `c.AbortWithStatus`, `c.AbortWithJSON`, `c.Error` | Removed or renamed | [Context error helpers](#context-error-helpers) |
| `middleware.RateLimiter` with the default handler | It returns a 429 error instead of writing text | [JSON error bodies](#json-error-bodies) |

## Group middleware order

A route, mount, static directory, or child group captures its group's middleware when it is registered. In 0.3, calling `group.Use` afterwards silently left those registrations without the new middleware, so an authentication middleware added at the end of a group protected nothing above it. In 0.4, `group.Use` **panics** once the group has any of them, and the message names the first one:

```text
zinc: Use on group "/admin" after route GET /admin/secret; register group middleware before its routes and child groups
```

Move `Use` above the group's routes, or pass the middleware to `Group`:

```go
// 0.3: compiles, but /admin/secret is public
admin := app.Group("/admin")
admin.Get("/secret", secret)
admin.Use(requireAdmin)

// 0.4
admin := app.Group("/admin", requireAdmin)
admin.Get("/secret", secret)
```

`app.Use` is unaffected. Global middleware runs on every request whenever it is registered.

## Binding requires tags

**This closes a mass-assignment hole. Check every struct you bind.** In 0.3, an exported field without a `path`, `query`, `header`, or `form` tag still bound under its lower-cased name. A struct written for a JSON body could therefore be filled from the query string, including fields hidden from JSON:

```go
type UpdateUser struct {
	Name    string `json:"name"`
	IsAdmin bool   `json:"-"` // 0.3: set by ?isadmin=true through c.Bind().All
}
```

In 0.4, each of those sources binds only fields that carry its tag. Add tags to the fields you intend to accept:

```go
type ListUsers struct {
	Page  int    `query:"page"`
	Limit int    `query:"limit"`
	Sort  string `query:"sort"`
}
```

A tag with options but no name, such as `query:",omitempty"`, still opts in under the lower-cased field name. Body binding through `json`, `xml`, `yaml`, and `toml` is unchanged.

## Matching HTTP errors

`errors.Is` now matches HTTP errors by status. Errors made by the constructors and copies made by `Wrap`, `WithDetail`, or `WithHeader` match the sentinel for their status, as do errors that wrap them:

```go
err := fmt.Errorf("load: %w", zinc.NotFound("user not found"))
errors.Is(err, zinc.ErrNotFound) // 0.3: false; 0.4: true
```

A target that carries a message also requires that message. If you compared `HTTPError.Code` with `errors.As` only because `errors.Is` did not work, you can simplify that code.

## Original URL

`c.OriginalURL()` now returns the request URI as received, before `SetPath`, the Rewrite middleware, or Trailing Slash changed it, as the documentation always described. In 0.3 it returned the rewritten path. Use `c.Request().URL` for the current target.

## Server-sent events

`c.SSE` now flushes each event as it writes it, so events reach the client immediately. `Config.WriteTimeout` now applies to each event instead of to the whole response. In 0.3, a stream served through `Listen` was cut off when the write timeout expired, which is 10 seconds by default.

Manual flushing after `c.SSE` still works, but you can remove it:

```go
if err := c.SSE(zinc.SSEvent{Data: msg}); err != nil {
	return err
}
// no longer needed:
// if f, ok := c.Writer().(http.Flusher); ok { f.Flush() }
```

## JSON error bodies

The default error handler now writes JSON instead of plain text. That includes the router's 404 and 405 responses, static-file misses, and errors from built-in middleware:

```text
0.3: HTTP/1.1 404 Not Found
     Content-Type: text/plain; charset=utf-8

     user not found

0.4: HTTP/1.1 404 Not Found
     Content-Type: application/json; charset=utf-8

     {"error":{"status":404,"message":"user not found"}}
```

Binding failures add a `fields` object naming what was wrong, and `WithDetail` values appear under `details`. The rules for what reaches the client are unchanged: an unknown error sends only its status text.

If clients depend on text bodies, keep them:

```go
cfg.ErrorHandler = zinc.TextErrors
```

The rate limiter's default handler used to write `Rate limit exceeded` itself. It now returns a 429 error, so the response follows your error handler like every other error.

Two error responses changed status. **A missing file served by `c.FileFS`** was a 500 and is now a 404. **Static directories** now send their 404 and 405 responses through the error handler instead of net/http's `404 page not found` text.

## Building errors

`WithMessage`, `WithCause`, `WithMeta`, and the `Meta` field are removed:

| 0.3 | 0.4 |
|---|---|
| `zinc.ErrNotFound.WithMessage("user not found")` | `zinc.NotFound("user not found")` |
| `zinc.ErrBadRequest.WithMessage("bad cursor").WithCause(err)` | `zinc.BadRequest("bad cursor").Wrap(err)` |
| `zinc.ErrTeapot.WithMessage("no coffee")` | `zinc.NewError(zinc.StatusTeapot, "no coffee")` |
| `err.WithMeta("field", "email")` | `err.WithDetail("field", "email")`, now written to the body |
| `httpErr.Meta` | `httpErr.Details` |

Constructors exist for 400, 401, 403, 404, 409, 410, 422, 429, 500, and 503. `NewError(code, message)` covers every other status.

**Return binding errors unchanged.** `return zinc.ErrBadRequest.WithMessage("invalid body").WithCause(err)` after a failed bind still compiles as `zinc.BadRequest("invalid body").Wrap(err)`, but it now hides the field details that the default handler would send. Use `return err`.

Domain errors can choose their own status by implementing `StatusCode() int`, which removes most error-mapping code from handlers. See [Errors](/guide/errors/).

## Validation errors

In 0.3, a `Validator` failure reached the default handler as an unknown error and answered **500**. In 0.4 it is wrapped in `*zinc.ValidationError` and answers **422 Unprocessable Entity**. If the validator's error has a `Fields() map[string]string` method, the fields appear in the body. A validator that already returned an HTTP error keeps its status.

## Context error helpers

| 0.3 | 0.4 |
|---|---|
| `return c.Fail(err)` | `return err` |
| `return c.AbortWithStatus(code)` | `return zinc.NewError(code)` |
| `return c.AbortWithJSON(code, v)` | `return c.Status(code).JSON(v)` |
| `c.Error(err)` | `c.HandleError(err)` |

`c.HandleError` is for middleware that needs the final status, such as a logger. Handlers should return errors.

A custom error handler that inspects `*zinc.HTTPError` keeps working. To log failures without replacing the response format, wrap the default:

```go
cfg.ErrorHandler = func(c *zinc.Context, err error) {
	if zinc.StatusCode(err) >= 500 {
		slog.Error("request failed", "err", err)
	}
	zinc.DefaultErrorHandler(c, err)
}
```
