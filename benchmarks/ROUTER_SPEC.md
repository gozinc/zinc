# Zinc router behaviour

This is the behaviour 0.5's router work must keep. `FuzzRouterReference` checks it: it serves random route tables and requests with the router under development and with Zinc v0.4.0 (`internal/zinc040`), and fails on any difference in status, headers, body, or the route and parameters a handler sees.

```sh
cd benchmarks
go test -run '^$' -fuzz '^FuzzRouterReference$' -fuzztime 60s .
```

Anything here that should change must change on purpose: update this file, give the change a migration note, and teach the fuzz test the new rule. Don't make the test lenient.

## Deliberate changes from v0.4.0

The fuzz test's reference is v0.4.0 with these changes applied, each marked in `internal/zinc040` and listed in its `doc.go`.

1. **`Allow` lists every method of a static path (P4).** v0.4.0 indexed only static paths that have more than one accepted spelling. So with `GET /v1/` (spellings `/v1/` and `/v1`) and `DELETE /v1` (one spelling), `OPTIONS /v1` answered `Allow: GET, HEAD, OPTIONS`, leaving out DELETE even though `DELETE /v1` was served. Found by `FuzzRouterReference` when the index was made complete.

2. **`Allow` merges every spelling of a static path (P5).** v0.4.0 looked up the path's spellings in order (as sent, without its trailing slash, then lowercased) and returned the first one that had any methods. With `POST /Users/users` and `DELETE /users/users/`, `GET /Users/users` answered `Allow: POST, OPTIONS`, leaving out DELETE, though `DELETE /Users/users` was served. Found by `FuzzRouterReference` when static routes moved into the route tree.
3. **Custom methods are listed in `Allow` in sorted order (P5).** v0.4.0 had no single rule: sorted for some static paths, registration order for others, and tree-creation order for parameter routes. Standard methods keep their fixed order. The fuzz test doesn't generate custom methods; `TestAllowSortsCustomMethods` covers this.

## Patterns

- A pattern is a path of `/`-separated segments. A pattern that doesn't start with `/` gets one.
- `{name}` matches one non-empty segment. `{name...}` matches the rest of the path, including slashes, and must be the final segment.
- A parameter must fill its whole segment: `/files/{id}.json` is rejected.
- Names are letters, digits and `_`, and don't start with a digit. A name can appear only once per pattern.
- The 0.3 grammar (`:name`, `*name`) is rejected, and the error names the `{…}` form to use.
- Registering the same method and pattern twice, or patterns that can't coexist, panics at registration, naming both.

## Matching a request

Matching uses `URL.Path`, the decoded path, so `%2F` is a slash.

The router tries, in order, and takes the first match:

1. **An exact static route** for the method.
2. **A case-folded static route**, when routing is case-insensitive (the default).
3. **The method's dynamic routes**, walked on the case-folded path:
   - a literal segment beats a `{param}`, and a `{param}` beats a `{name...}`, at every level;
   - a parameter's value keeps the request's original case.

A trailing slash is ignored unless `StrictRouting` is on: `/users/` matches `/users`. With `StrictRouting`, `/users/` and `/users` are different paths.

With `CaseSensitive`, nothing is folded.

## When no route matches the method

1. **HEAD.** A HEAD request with no HEAD route is served by the GET route, unless `DisableAutoHead`. The response has no body.
2. **Mounts.** Next, a mount whose prefix matches serves the request.
3. **Scoped 404 handlers.** Next, a `RouteNotFound` handler whose pattern matches the path serves it with status 404.
4. **OPTIONS.** An OPTIONS request for a path that other methods match answers `204 No Content` with `Allow`, unless `DisableAutoOptions`.
5. **405.** A request for a path that other methods match answers `405 Method Not Allowed` with `Allow`, unless `DisableMethodNotAllowed`. The default body is the JSON error envelope; `App.MethodNotAllowed` replaces it.
6. **404.** Otherwise the answer is `404 Not Found`. The default body is the JSON error envelope; `App.NotFound` replaces it.

`Allow` lists the path's methods in a fixed order. It adds HEAD when GET is allowed and automatic HEAD is on, and OPTIONS when automatic OPTIONS is on.

## The route cache

The route cache is invisible: every answer is the same with the cache on (`RouteCacheSize` > 0, the default 1,000), off (`RouteCacheSize: -1`), or tiny. `FuzzRoutingCacheEquivalence`, in the root package, checks this, and `FuzzRouterReference` runs each request twice with each cache setting.

## What the fuzz test covers

| Covered | Values |
|---|---|
| Config | `StrictRouting`, `CaseSensitive`, `DisableAutoHead`, `DisableAutoOptions`, `DisableMethodNotAllowed`, and cache sizes 1,000, off and 2 |
| Route tables | Up to 24 routes, from 0 to 4 segments deep |
| Route segments | Static (including mixed case and non-ASCII), `{param}` and `{name...}` |
| Route patterns | Sometimes a trailing slash |
| Methods | GET, POST, PUT, DELETE and PATCH |
| Requests | Near a registered route or random; with changed case, an extra or missing segment, a trailing or doubled slash, and encoded values |
| Request methods | Every method above, plus HEAD and OPTIONS |
| Scoped 404s and mounts | A `RouteNotFound` handler and a mount, in some tables |

Groups aren't generated. A group joins its prefix to the pattern before the router sees it, so the route tables already cover what the router does with grouped routes.
