---
title: Body Limit
description: Reject oversized request bodies before handlers consume them.
---

`BodyLimit` rejects request bodies over a size you choose with `413 Request Entity Too Large`, before handlers read them. Use it to set a tighter limit on particular routes than the app-wide `Config.BodyLimit`, which binding already enforces.

## Quick start

```go
app.Use(middleware.BodyLimit(10 * middleware.MB))
```

## Config

```go
app.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
	Limit: 10 * middleware.MB,
}))
```

## Fields

| Field | Meaning |
|---|---|
| `Skipper` | Skip the limit for selected requests |
| `Limit` | Required maximum size in bytes |

## Size helpers

Zinc exposes convenient constants:

- `middleware.B`
- `middleware.KB`
- `middleware.MB`
- `middleware.GB`

## Failure behavior

When the body exceeds the configured limit, Zinc returns a `*middleware.BodyLimitError`.

That error includes:

- `Limit`
- `Observed`
- `Source`

`Source` tells you whether the limit was exceeded from:

- the request's `Content-Length`
- actual body reads

The error unwraps to `zinc.ErrRequestEntityTooLarge`.
