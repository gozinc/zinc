---
title: Key Auth
description: Require an API key on requests, read from a header, query value or cookie.
---

Key Auth lets through only requests that carry an API key you issued. Use it for machine clients and integrations that send a fixed secret; for signed tokens with claims, use [JWT](/middleware/jwtauth/) instead.

## Usage

```go
import "github.com/0mjs/zinc/middleware/keyauth"

app.Use(keyauth.New(keyauth.Config{
	Validator: keyauth.Static(os.Getenv("API_KEY")),
}))
```

```bash
curl -i http://localhost:8080/
# HTTP/1.1 401 Unauthorized
# Www-Authenticate: Bearer
# {"error":{"status":401,"message":"Unauthorized"}}

curl -i -H 'Authorization: Bearer k-123' http://localhost:8080/
# HTTP/1.1 200 OK
```

A missing key and a wrong key both get the same `401`, so a client can't tell which one it got wrong.

## Defaults

| Setting | Default |
|---|---|
| `Validator` | **required**: `New` panics without it |
| Where the key comes from | `Authorization: Bearer <key>` |
| On success | calls `c.Next()` |
| On failure | `401` with `WWW-Authenticate: Bearer` |

## Configuration

Read the key from an `X-API-Key` header, fall back to a query value, and accept two keys while you rotate:

```go
app.Use(keyauth.New(keyauth.Config{
	Extractor: keyauth.FromFirst(
		keyauth.FromHeader("X-API-Key"),
		keyauth.FromQuery("api_key"),
	),
	Validator: keyauth.StaticKeys(os.Getenv("API_KEY"), os.Getenv("API_KEY_PREVIOUS")),
}))
```

```bash
curl -H 'X-API-Key: previous-key' http://localhost:8080/
curl 'http://localhost:8080/?api_key=current-key'
```

Both requests get through.

| Field | Default | Meaning |
|---|---|---|
| `Validator` | required | Checks the key. Returns `(true, nil)` to let the request through. |
| `Extractor` | `FromAuthorizationHeader()` | Reads the key from the request. |
| `SuccessHandler` | calls `c.Next()` | Runs after the key passes. Call `c.Next()` in it to continue the chain. |
| `ErrorHandler` | `401` with `WWW-Authenticate: Bearer` | Runs when extraction or validation fails, with the error. Its return value is what the client gets. |

## Check a key

`keyauth.Static(key)` accepts one key, and `keyauth.StaticKeys(keys...)` accepts any key in a list. Both compare in constant time, so response timing doesn't reveal how close a guess was.

To look keys up in your own store, write a `Validator`:

```go
Validator: func(c *zinc.Context, cred keyauth.Credentials) (bool, error) {
	return apiKeys.Exists(c.Context(), cred.Key) // your data layer
},
```

Return `(false, nil)` for an unknown key; the client gets a `401`. Return an error only when the lookup itself failed. That error goes to the app's error handler, so the client gets a `500`.

## Read the key from somewhere else

| Extractor | Reads |
|---|---|
| `keyauth.FromAuthorizationHeader()` | `Authorization: Bearer <key>` |
| `keyauth.FromHeader(name)` | The whole header value |
| `keyauth.FromHeaderPrefix(name, prefix)` | The value after `prefix`, matched without regard to case |
| `keyauth.FromQuery(name)` | A query value, such as `?api_key=...` |
| `keyauth.FromCookie(name)` | A cookie value |
| `keyauth.FromFirst(extractors...)` | The first extractor that finds a key |

`FromFirst` tries the next extractor only when the key is missing. Surrounding spaces are trimmed from every value.

:::caution[Keys in URLs get logged]
A key in a query string ends up in access logs, browser history and `Referer` headers. Prefer a header, and keep `FromQuery` for clients that can't set one.
:::

## Reading state

```go
state, ok := keyauth.Get(c) // ok is false without the middleware
state := keyauth.MustGet(c) // panics without the middleware

state.Key    // the accepted key
state.Source // keyauth.SourceAuthorizationHeader, SourceHeader, SourceQuery or SourceCookie
```

The state is set only after the key passes.

## Errors

The default `ErrorHandler` answers `401` with `WWW-Authenticate: Bearer` for these errors:

| Error | When |
|---|---|
| `keyauth.ErrKeyMissing` | No key in the request |
| `keyauth.ErrKeyInvalid` | The validator returned `false` |

Any other error is returned as is. To tell the client which case it hit, replace the handler:

```go
app.Use(keyauth.New(keyauth.Config{
	Validator: keyauth.Static(os.Getenv("API_KEY")),
	ErrorHandler: func(c *zinc.Context, err error) error {
		if errors.Is(err, keyauth.ErrKeyMissing) {
			return zinc.NewError(http.StatusUnauthorized, "API key required")
		}
		return zinc.NewError(http.StatusForbidden, "API key not recognized")
	},
}))
```

```bash
curl http://localhost:8080/
# {"error":{"status":401,"message":"API key required"}}

curl -H 'Authorization: Bearer wrong' http://localhost:8080/
# {"error":{"status":403,"message":"API key not recognized"}}
```

## Related

- [Basic Auth](/middleware/basicauth/): username and password with the browser prompt.
- [JWT](/middleware/jwtauth/): signed bearer tokens with claims.
- [Casbin Auth](/middleware/casbin/): authorize by key with `casbin.SubjectFromKeyAuth()`.
- [Rate Limiter](/middleware/limiter/): limit each client by its key.
