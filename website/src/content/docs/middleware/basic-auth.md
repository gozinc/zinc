---
title: Basic Auth
description: Extract, validate, and expose HTTP Basic auth credentials.
---

`BasicAuth` protects routes with HTTP Basic authentication, the browser's built-in username and password prompt. It suits admin panels, internal tools, and operational endpoints.

```go
admin := app.Group("/admin",
	middleware.BasicAuth(middleware.BasicAuthStatic("admin", os.Getenv("ADMIN_PASSWORD"))),
)
```

Basic credentials are only base64-encoded, so serve these routes over HTTPS. Unauthenticated requests get `401` with a `WWW-Authenticate` challenge.

## Extractor helpers

- `BasicAuthFromAuthorizationHeader()`
- `BasicAuthFromHeader(name)`
- `BasicAuthFromHeaderPrefix(name, prefix)`
- `BasicAuthFromFirst(extractors...)`

## Validator helpers

- `BasicAuthStatic(username, password)`
- `BasicAuthStaticPairs(pairs...)`

Both static validators compare in constant time, so response timing does not reveal how much of a guess was right.

## Config fields

| Field | Meaning |
|---|---|
| `Skipper` | Skip auth for selected requests |
| `Extractor` | Read credentials from a custom source |
| `Validator` | Required validator |
| `SuccessHandler` | Override the success path |
| `ErrorHandler` | Override error/challenge behavior |
| `Realm` | Sets the `WWW-Authenticate` realm |

## Reading the authenticated identity

```go
identity, ok := middleware.BasicAuthCurrent(c)
username, ok := middleware.BasicAuthUsername(c)
```

Zinc stores:

- `Username`
- `Source`

That lets you distinguish whether credentials came from the normal authorization header or a custom header-based extractor.

## When to use this

Use Basic Auth when you genuinely want Basic Auth. For opaque API keys, use
[`Key Auth`](/middleware/key-auth/). Use [`JWT`](/middleware/jwt/) for signed bearer tokens with
claims.
