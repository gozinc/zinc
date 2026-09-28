---
title: Upgrading to 0.5
description: What changed in Zinc 0.5, 0.5.1 and 0.5.2. Moving from 0.4 needs no code changes.
slug: extra/migration-0.5
---

Moving from 0.4 to 0.5 needs no code changes. Update the module and run your tests:

```bash
go get github.com/0mjs/zinc@v0.5.2
go test ./...
```

0.5 rebuilt the router and removed the route cache. Everything below is either faster, a bug fix, or something that used to fail and now works.

## What you might notice

### `RouteCacheSize` does nothing

0.5 removed the route cache. Matching without it is faster on real traffic, where most URLs carry different IDs. `Config.RouteCacheSize` and `zinc.DefaultRouteCacheSize` still compile but are ignored, so you can delete them whenever it suits you.

### `Allow` headers are complete

A `405` or automatic `OPTIONS` answer lists every method that can serve the path. 0.4 left some out:

- A path registered with and without a trailing slash for different methods, such as `GET /v1/` and `DELETE /v1`, now lists both.
- A path registered in different letter cases for different methods, such as `POST /Users/list` and `DELETE /users/list/`, now lists both.
- Custom methods, such as `PURGE`, are listed in alphabetical order after the standard ones.

If a test compares an `Allow` header exactly, it may need the extra method.

### `:` and `*` work inside a path (0.5.1)

Routes such as `/v1/users:batch` or `/opening/09:00` used to fail at startup. They now register as ordinary routes. A segment that *starts* with `:` or `*`, such as `/users/:id`, still fails and tells you to write `/users/{id}`.

### The URL wins over the body (0.5.2)

`Bind().All` and typed handlers now read the body first, then query values, then path parameters. Before, the body came last, so a body key such as `"id"` replaced the `{id}` from the path, even without a `json` tag, because Go matches JSON keys ignoring case. A field tagged for both path and query now gets the path value. If you tagged path fields `json:"-"` to protect them, you can keep or remove the tag.

### Middleware sees 404 and 405 as errors (0.5.2)

When no route matches, `c.Next()` in `app.Use` or `app.UsePrefix` middleware returns `zinc.ErrNotFound` or `zinc.ErrMethodNotAllowed`. Before, it returned `nil` unless you set a custom `ErrorHandler`. The client gets the same response as before. Middleware that treats any error as a failure, such as a metric counting errors, now counts routing misses.

### The logger picks its level from the status (0.5.2)

The [logger](/middleware/logger/) logs a `5xx` at `ERROR` as `REQUEST_ERROR`, and everything else at `INFO` as `REQUEST`. Before, any returned error, including a `400` or `404`, was an `ERROR`. The `error` key is still there on `INFO` lines. If you alert on the logger's `ERROR` lines, you'll now only hear about server faults.

### Redirect and rewrite warn when they run too late (0.5.2)

[`redirect`](/middleware/redirect/) and [`rewrite`](/middleware/rewrite/) must run before routing, so register them with `app.Use` or `app.UsePrefix`. On a group or a route, they log a warning the first time a request reaches them. Their behavior hasn't changed.

### Repeated-URL benchmarks

Without a route cache, a benchmark that sends the same URL over and over is slower than on 0.4, because 0.4 answered it from memory. Real traffic, with varied URLs and many cores, is faster. See [Benchmarks](/extra/benchmarks/) for both.

## Next steps

- [Routing](/guide/routing/): the rules the new router follows.
- [Release notes](https://github.com/0mjs/zinc/releases): every change in 0.5.0, 0.5.1 and 0.5.2.
