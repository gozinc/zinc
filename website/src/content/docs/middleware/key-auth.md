---
title: Key Auth
description: Validate API keys from headers, query values, or cookies.
---

`KeyAuth` protects routes with API keys: opaque strings you issue to clients. By default it reads `Authorization: Bearer <key>` and answers `401` when the key is missing or invalid.

```go
app.Use(middleware.KeyAuth(middleware.KeyAuthStatic(os.Getenv("API_KEY"))))
```

Use another extractor when keys live somewhere else.

```go
app.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
	Extractor: middleware.KeyAuthFromHeader("X-API-Key"),
	Validator: middleware.KeyAuthStatic("secret"),
}))
```

Available extractors:

- `KeyAuthFromAuthorizationHeader()`
- `KeyAuthFromHeader(header)`
- `KeyAuthFromHeaderPrefix(header, prefix)`
- `KeyAuthFromQuery(name)`
- `KeyAuthFromCookie(name)`
- `KeyAuthFromFirst(...)`

Inside handlers, use `KeyAuthCurrent(c)` or `MustKeyAuthCurrent(c)` to read the accepted key source.
