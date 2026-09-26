---
title: Key Auth
description: Validate API keys from headers, query values, or cookies.
---

`keyauth` protects routes with API keys: opaque strings you issue to clients. By default it reads `Authorization: Bearer <key>` and answers `401` when the key is missing or invalid.

```go
import "github.com/0mjs/zinc/middleware/keyauth"

app.Use(keyauth.New(keyauth.Config{
	Validator: keyauth.Static(os.Getenv("API_KEY")),
}))
```

`Validator` is required. `keyauth.Static` and `keyauth.StaticKeys` compare in constant time.

Use another extractor when keys live somewhere else:

```go
app.Use(keyauth.New(keyauth.Config{
	Extractor: keyauth.FromHeader("X-API-Key"),
	Validator: keyauth.StaticKeys("current", "previous"),
}))
```

Available extractors:

- `keyauth.FromAuthorizationHeader()`
- `keyauth.FromHeader(header)`
- `keyauth.FromHeaderPrefix(header, prefix)`
- `keyauth.FromQuery(name)`
- `keyauth.FromCookie(name)`
- `keyauth.FromFirst(extractors...)`

Inside handlers, `keyauth.Get(c)` returns the accepted key and where it came from; `keyauth.MustGet(c)` panics without the middleware.
