---
title: Upgrading to 0.5
description: What changed in Zinc 0.5 and 0.5.1. Moving from 0.4 needs no code changes.
slug: extra/migration-0.5
---

Moving from 0.4 to 0.5 needs no code changes. Update the module and run your tests:

```bash
go get github.com/0mjs/zinc@v0.5.1
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

### Repeated-URL benchmarks

Without a route cache, a benchmark that sends the same URL over and over is slower than on 0.4, because 0.4 answered it from memory. Real traffic, with varied URLs and many cores, is faster. See [Benchmarks](/extra/benchmarks/) for both.

## Next steps

- [Routing](/guide/routing/): the rules the new router follows.
- [Release notes](https://github.com/0mjs/zinc/releases): every change in 0.5.0 and 0.5.1.
