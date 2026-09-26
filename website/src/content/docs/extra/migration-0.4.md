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

`errors.Is` now matches HTTP errors by status. Copies made with `WithMessage`, `WithCause`, `WithMeta`, or `WithHeader` match the sentinel they came from, as do errors that wrap them:

```go
err := fmt.Errorf("load: %w", zinc.ErrNotFound.WithMessage("user not found"))
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
