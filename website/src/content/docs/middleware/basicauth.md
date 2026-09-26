---
title: Basic Auth
description: Extract, validate, and expose HTTP Basic auth credentials.
---

`basicauth` protects routes with HTTP Basic authentication, the browser's built-in username and password prompt. It suits admin panels, internal tools, and operational endpoints.

```go
import "github.com/0mjs/zinc/middleware/basicauth"

admin := app.Group("/admin", basicauth.New(basicauth.Config{
	Validator: basicauth.Static("admin", os.Getenv("ADMIN_PASSWORD")),
}))
```

`Validator` is required. Basic credentials are only base64-encoded, so serve these routes over HTTPS. Unauthenticated requests get `401` with a `WWW-Authenticate` challenge.

## Validators

- `basicauth.Static(username, password)`
- `basicauth.StaticPairs(pairs...)`

Both compare in constant time, so response timing does not reveal how much of a guess was right. Any `func(*zinc.Context, basicauth.Credentials) (bool, error)` works too.

## Extractors

The default reads the `Authorization` header. Others:

- `basicauth.FromAuthorizationHeader()`
- `basicauth.FromHeader(name)`
- `basicauth.FromHeaderPrefix(name, prefix)`
- `basicauth.FromFirst(extractors...)`

## Config

| Field | Meaning |
|---|---|
| `Validator` | Checks the credentials; required |
| `Extractor` | Reads credentials from a custom source |
| `SuccessHandler` | Replaces the success path, which calls `c.Next()` |
| `ErrorHandler` | Replaces the error and challenge behavior |
| `Realm` | Sets the `WWW-Authenticate` realm |

## Reading the authenticated identity

```go
identity, ok := basicauth.Get(c)   // identity.Username, identity.Source
identity := basicauth.MustGet(c)   // panics without the middleware
```

`Source` tells you whether credentials came from the authorization header or a custom header-based extractor.

## When to use this

Use Basic Auth when you genuinely want Basic Auth. For opaque API keys, use [Key Auth](/middleware/keyauth/). Use [JWT](/middleware/jwtauth/) for signed bearer tokens with claims.
