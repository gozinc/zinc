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
4. **`:` and `*` are literal except at a segment's start (0.5.1).** v0.4.0 rejected them anywhere in a pattern, so routes such as `/v1/users:batch` or `/times/12:00` couldn't be registered. A segment that starts with either is still rejected with the `{…}` form to use.
5. **Another method never hides a route (0.7.2).** v0.4.0 tried a request with a trailing slash without the slash, then as sent, but skipped the second try when another method's parameter route matched the first. With `GET /x/{id}` and `POST /x/{id}/`, `POST /x/123/` answered `405` with `Allow: GET, HEAD, OPTIONS`, though `FindRoute` found the POST route, and with `DisableMethodNotAllowed` the POST route ran. Now both spellings are tried for the request's method before any 405, and `Allow` merges the methods of both. Upgrade note: such requests reach their route instead of a 405.
6. **A catch-all keeps the path as sent (0.7.2).** v0.4.0 dropped a catch-all's trailing slash under default routing (`/files/a/` gave `a`, `/files//` gave an empty value) and, with strict routing too, a leading slash (`/files//a` gave `a`). So `App.URL("files", "a/")` built `/files/a/`, which routed back to `a`. Now the value is the rest of the path, slashes included. Upgrade note: a handler that relied on the slash being dropped sees it.
7. **A parameter route with a trailing slash is also reached without it (0.7.2).** v0.4.0 recorded only static routes under both spellings, so under default routing `GET /x/{id}/` answered `/x/1/` but not `/x/1`, and `GET /x/{id}` could be registered beside it although it then took every request the other matched. Now every route is recorded under both spellings unless routing is strict, so the pair conflicts at registration. Upgrade note: a table with both now panics at registration; keep one.
8. **A registration conflict names both routes (0.7.2).** v0.4.0 said `route already registered for /x/{id}/`; now `route already registered: GET /x/{id}/ matches the same requests as GET /x/{id}`, or `route already registered: GET /x` for the same pattern twice.

## Patterns

- A pattern is a path of `/`-separated segments. A pattern that doesn't start with `/` gets one.
- `{name}` matches one non-empty segment. `{name...}` matches the rest of the path, including slashes, and must be the final segment.
- A parameter must fill its whole segment: `/files/{id}.json` is rejected.
- Names are letters, digits and `_`, and don't start with a digit. A name can appear only once per pattern.
- A segment that starts with `:` or `*` is rejected, and the error names the `{…}` form to use: that is Gin's, Echo's and httprouter's syntax, and was Zinc 0.1's. Anywhere else in a segment, `:` and `*` are literal characters (`/v1/users:batch`, `/times/12:00`).
- Registering the same method and pattern twice, or patterns that match the same requests (such as `/x/{id}` and `/x/{id}/` without `StrictRouting`, or `/Users/{id}` and `/users/{name}` without `CaseSensitive`), panics at registration, naming both.
- A route has at most 256 parameters; a wider one panics at registration.
- A nil handler, or nil middleware on the route or its group, panics when the route is registered.

## Matching a request

Matching uses `URL.Path`, the decoded path, so `%2F` is a slash.

The router tries, in order, and takes the first match:

1. **An exact static route** for the method.
2. **A case-folded static route**, when routing is case-insensitive (the default).
3. **The method's dynamic routes**, walked on the case-folded path:
   - a literal segment beats a `{param}`, and a `{param}` beats a `{name...}`, at every level;
   - a parameter's value keeps the request's original case;
   - a `{name...}` value is the rest of the path as sent, slashes included.

Only routes for the request's method take part: another method's routes never change which route matches.

A trailing slash is ignored unless `StrictRouting` is on: a route is recorded under its pattern and the pattern without its trailing slash, and a request is matched without its trailing slash first, then as sent. So `/users/` matches `/users`, and `/x/{id}/` matches `/x/1`. A catch-all keeps the slash: `/files/{path...}` gives `a/` for `/files/a/`, and an empty value for `/files/`. With `StrictRouting`, `/users/` and `/users` are different paths.

Dispatch, `App.FindRoute` and `Allow` use one lookup, so they agree: `FindRoute` finds the route a request runs, including the GET route for a HEAD request under automatic HEAD, and `Allow` lists exactly the methods that would be served. `App.URL` builds a URL only if it routes back to the same route and values; otherwise it returns an error.

With `CaseSensitive`, nothing is folded.

## When no route matches the method

1. **HEAD.** A HEAD request with no HEAD route is served by the GET route, unless `DisableAutoHead`. The response has no body.
2. **Mounts.** Next, a mount whose prefix matches serves the request.
3. **Scoped 404 handlers.** Next, a `RouteNotFound` handler whose pattern matches the path serves it with status 404.
4. **OPTIONS.** An OPTIONS request for a path that other methods match answers `204 No Content` with `Allow`, unless `DisableAutoOptions`.
5. **405.** A request for a path that other methods match answers `405 Method Not Allowed` with `Allow`, unless `DisableMethodNotAllowed`. The default body is the JSON error envelope; `App.MethodNotAllowed` replaces it.
6. **404.** Otherwise the answer is `404 Not Found`. The default body is the JSON error envelope; `App.NotFound` replaces it.

`Allow` lists the methods whose routes match the path, by either spelling, in a fixed order. It adds HEAD when GET is allowed and automatic HEAD is on, and OPTIONS when automatic OPTIONS is on.

## The route cache

Zinc has no route cache since 0.5; `RouteCacheSize` is accepted and ignored. v0.4.0's cache was invisible: its answers were the same at every size. The reference keeps it, so `FuzzRouterReference` still runs the v0.4.0 side with the cache at its default, off, or at 2 entries, and runs each request twice.

## What the fuzz test covers

| Covered | Values |
|---|---|
| Config | `StrictRouting`, `CaseSensitive`, `DisableAutoHead`, `DisableAutoOptions`, `DisableMethodNotAllowed`, and, for the reference only, cache sizes 1,000, off and 2 |
| Route tables | Up to 24 routes, from 0 to 4 segments deep |
| Route segments | Static (including mixed case, non-ASCII, and `:` or `*` inside a segment), `{param}` and `{name...}` |
| Route patterns | Sometimes a trailing slash |
| Methods | GET, POST, PUT, DELETE and PATCH |
| Requests | Near a registered route or random; with changed case, an extra or missing segment, a trailing or doubled slash, and encoded values |
| Request methods | Every method above, plus HEAD and OPTIONS |
| Scoped 404s and mounts | A `RouteNotFound` handler and a mount, in some tables |

Groups aren't generated. A group joins its prefix to the pattern before the router sees it, so the route tables already cover what the router does with grouped routes.
