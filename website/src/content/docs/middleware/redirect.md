---
title: Redirect
description: Send clients from old paths to new ones with a redirect status.
---

Redirect answers requests for old paths with a redirect to the new ones, so bookmarks and links keep working after you move a page or rename an API prefix. The client sees the new URL. To serve the new handler without the client knowing, use [Rewrite](/middleware/rewrite/) instead.

## Usage

```go
import "github.com/0mjs/zinc/middleware/redirect"

app.Use(redirect.New(redirect.Config{Rules: map[string]string{
	"/old":  "/new",
	"/v1/*": "/api/v1/*",
}}))
```

```bash
curl -i http://localhost:8080/old
# HTTP/1.1 301 Moved Permanently
# Location: /new

curl -i "http://localhost:8080/v1/users/42?page=2"
# HTTP/1.1 301 Moved Permanently
# Location: /api/v1/users/42?page=2
```

A rule ending in `*` matches a prefix, and a `*` in the target is replaced by the rest of the path. The query string is kept.

Register it with `app.Use`. It then runs before routing, so the old paths don't need routes of their own.

:::caution[Not in a group]
Group middleware only runs for paths that match one of the group's routes. An old path usually doesn't, so it gets a 404 instead of a redirect. Use `app.Use`, or `app.UsePrefix` for one subtree.

The first request that reaches a misplaced `redirect` logs a warning through `slog`:

```text
time=2026-09-28T02:13:48.899+01:00 level=WARN msg="redirect middleware runs after routing, so it only sees requests that already matched a route; register it with app.Use or app.UsePrefix" middleware=redirect route=/api/users
```
:::

## Defaults

| Setting | Default |
|---|---|
| Rules | none, so nothing is redirected |
| Status | `301 Moved Permanently` |

## Configuration

Send a `POST` to the new path as a `POST`, and let browsers forget the redirect:

```go
app.Use(redirect.New(redirect.Config{
	Rules:      map[string]string{"/login": "/signin"},
	StatusCode: zinc.StatusTemporaryRedirect,
}))
```

```bash
curl -i -X POST http://localhost:8080/login
# HTTP/1.1 307 Temporary Redirect
# Location: /signin
```

| Field | Default | Meaning |
|---|---|---|
| `Rules` | none | Old path → new path. A key ending in `*` matches a prefix. A `*` in the value is replaced by the rest of the path; with no `*`, the rest is appended. |
| `StatusCode` | `301` | Redirect status. A value outside `3xx` sends `302 Found`. |

Rules compare the path exactly, so `/old` doesn't match `/OLD` or `/old/`, though Zinc's routes do. Add a rule for each spelling you need, or register [Trailing Slash](/middleware/trailingslash/) first. An exact rule wins over a `*` rule; when two `*` rules match the same path, such as `/v1/*` and `/v1/users/*`, which one applies isn't defined.

Browsers and clients change a `POST` into a `GET` when they follow a `301` or `302`. Use `307 Temporary Redirect` or `308 Permanent Redirect` to keep the method and body.

## Redirect to another site

A target can be a full URL:

```go
app.Use(redirect.New(redirect.Config{Rules: map[string]string{
	"/docs/*": "https://docs.example.com/",
}}))
```

```bash
curl -i http://localhost:8080/docs/intro
# HTTP/1.1 301 Moved Permanently
# Location: https://docs.example.com/intro
```

## Errors

`redirect` never fails a request. Paths that match no rule continue to routing as normal.

`redirect.New` panics at startup when it's given more than one `Config`.

## Related

- [Rewrite](/middleware/rewrite/): serves a different path without telling the client.
- [Trailing Slash](/middleware/trailingslash/): redirects to one canonical form of each URL.
- [Groups and Middleware](/guide/groups-and-middleware/): where app, prefix and group middleware run.
