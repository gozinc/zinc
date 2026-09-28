---
title: JWT
description: Accept only requests that carry a valid signed token, and read its claims in your handlers.
---

JWT auth lets through only requests that carry a valid JSON Web Token, and gives your handlers the token's claims, such as the user ID and role. Use it when another service issues signed tokens, or when you want clients to prove who they are without a server-side session. For a fixed secret per client, [Key Auth](/middleware/keyauth/) is simpler.

:::note[Contrib package]
This middleware lives in its own module, `github.com/0mjs/contrib/jwtauth`, because it depends on [golang-jwt](https://github.com/golang-jwt/jwt) (`github.com/golang-jwt/jwt/v5`). Zinc itself needs nothing outside the standard library.

```sh
go get github.com/0mjs/contrib/jwtauth
```
:::

## Usage

```go
import (
	"github.com/0mjs/contrib/jwtauth"
	"github.com/golang-jwt/jwt/v5"
)

api := app.Group("/api", jwtauth.New(jwtauth.Config{
	KeyFunc: func(_ *zinc.Context, token *jwt.Token) (any, error) {
		return signingKey, nil // your HMAC key, as []byte
	},
	ParserOptions: []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256"}),
	},
}))

api.Get("/me", func(c *zinc.Context) error {
	claims := jwtauth.MustClaims[jwt.MapClaims](c)
	return c.JSON(zinc.Map{"sub": claims["sub"]})
})
```

```bash
curl -i http://localhost:8080/api/me
# HTTP/1.1 401 Unauthorized
# Www-Authenticate: Bearer
# {"error":{"status":401,"message":"Unauthorized"}}

curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/me
# {"sub":"ada"}

curl -i -H "Authorization: Bearer $EXPIRED_TOKEN" http://localhost:8080/api/me
# HTTP/1.1 401 Unauthorized
# Www-Authenticate: Bearer error="invalid_token"
```

The middleware checks the signature, and the `exp` (expiry) and `nbf` (not before) times when the token has them, before your handler runs.

:::caution[Pin the signing algorithm]
Always say which algorithms you accept, with `jwt.WithValidMethods` as above, or by checking `token.Method` in `KeyFunc`. A token names its own algorithm, and trusting that choice is a well-known JWT attack.
:::

## Defaults

| Setting | Default |
|---|---|
| `KeyFunc` or `ParseTokenFunc` | **required**: `New` panics without one |
| Where the token comes from | `Authorization: Bearer <token>` |
| Claims type | `jwt.MapClaims` |
| Expiry | checked when the token has `exp`, but not required |
| Realm | none |
| On success | calls `c.Next()` |
| On failure | `401` with a `WWW-Authenticate: Bearer` challenge |

## Configuration

Parse claims into your own struct and add a check of your own after the signature passes:

```go
type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

api := app.Group("/api", jwtauth.New(jwtauth.Config{
	NewClaims: func(*zinc.Context) jwt.Claims { return &Claims{} },
	KeyFunc: func(_ *zinc.Context, token *jwt.Token) (any, error) {
		return signingKey, nil
	},
	ParserOptions: []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256"}),
	},
	Validate: func(c *zinc.Context, token *jwt.Token) error {
		if token.Claims.(*Claims).Role == "banned" {
			return zinc.ErrForbidden
		}
		return nil
	},
}))

api.Get("/me", func(c *zinc.Context) error {
	claims := jwtauth.MustClaims[*Claims](c)
	return c.JSON(zinc.Map{"sub": claims.Subject, "role": claims.Role})
})
```

```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/api/me
# {"role":"admin","sub":"ada"}

curl -i -H "Authorization: Bearer $BANNED_TOKEN" http://localhost:8080/api/me
# HTTP/1.1 403 Forbidden
# {"error":{"status":403,"message":"Forbidden"}}
```

| Field | Default | Meaning |
|---|---|---|
| `KeyFunc` | required, unless `ParseTokenFunc` is set | Returns the key that verifies a token. Gets the parsed but unverified token, so it can pick a key by `kid`. |
| `ParseTokenFunc` | built from `KeyFunc` | Parses and verifies the token string itself. Replaces `KeyFunc`, `NewClaims` and `ParserOptions`. |
| `NewClaims` | `jwt.MapClaims{}` | Returns a fresh claims value to parse into, such as `&Claims{}`. |
| `ParserOptions` | none | Options passed to golang-jwt, such as `jwt.WithValidMethods`, `jwt.WithIssuer`. |
| `Validate` | none | Runs after the token is verified. Return an error to reject the request. |
| `Extractor` | `FromAuthorizationHeader("Bearer")` | Reads the token string from the request. |
| `Realm` | none | Adds `realm="..."` to the `WWW-Authenticate` challenge. |
| `SuccessHandler` | calls `c.Next()` | Runs after the token passes. Call `c.Next()` in it to continue. |
| `ErrorHandler` | `401` with challenge | Runs when any step fails, with the error. Its return value is what the client gets. |

## Read the token from somewhere else

| Extractor | Reads |
|---|---|
| `jwtauth.FromAuthorizationHeader(scheme)` | `Authorization: <scheme> <token>`; an empty scheme means `Bearer` |
| `jwtauth.FromHeader(name)` | The whole header value |
| `jwtauth.FromHeaderPrefix(name, prefix)` | The value after `prefix`, matched without regard to case |
| `jwtauth.FromCookie(name)` | A cookie value |
| `jwtauth.FromQuery(name)` | A query value |
| `jwtauth.FromFirst(extractors...)` | The first extractor that finds a token |

`FromFirst` tries the next extractor only when the token is missing. A malformed value, such as `Authorization: Basic ...` where a bearer token was expected, stops the search.

## Reading state

```go
claims, ok := jwtauth.Claims[*Claims](c) // ok is false if absent or of another type
claims := jwtauth.MustClaims[*Claims](c) // panics instead

token, ok := jwtauth.Get(c) // the verified *jwt.Token
token := jwtauth.MustGet(c)
token.Raw                   // the original token string
```

Ask for the type `NewClaims` returns: `*Claims` above, or `jwt.MapClaims` by default.

## Errors

The default `ErrorHandler` answers `401` and sets a `WWW-Authenticate` challenge that says what went wrong:

| Problem | Error | Challenge |
|---|---|---|
| No token | `jwtauth.ErrTokenMissing` | `Bearer` |
| Wrong scheme or empty value | `jwtauth.ErrTokenMalformed` | `Bearer error="invalid_request"` |
| Bad signature, expired, not a JWT, wrong algorithm | golang-jwt's error, such as `jwt.ErrTokenExpired`, `jwt.ErrTokenSignatureInvalid`, `jwt.ErrTokenMalformed` | `Bearer error="invalid_token"` |
| A custom `ParseTokenFunc` returned a token that isn't valid | `jwtauth.ErrTokenInvalid` | `Bearer error="invalid_token"` |

With `Realm: "api"`, the challenge becomes `Bearer realm="api", error="invalid_token"`.

A `*zinc.HTTPError` with a status other than `401`, such as `zinc.ErrForbidden` from `Validate`, is sent as is, with no challenge. Use `errors.Is` in a custom `ErrorHandler` to tell the cases apart:

```go
ErrorHandler: func(c *zinc.Context, err error) error {
	if errors.Is(err, jwt.ErrTokenExpired) {
		return zinc.NewError(http.StatusUnauthorized, "token expired")
	}
	return zinc.ErrUnauthorized
},
```

```bash
curl -H "Authorization: Bearer $EXPIRED_TOKEN" http://localhost:8080/api/me
# {"error":{"status":401,"message":"token expired"}}
```

A custom handler replaces the challenge too: set `WWW-Authenticate` yourself if clients rely on it.

## Security

For production, pin the algorithm, issuer and audience, and require an expiry:

```go
ParserOptions: []jwt.ParserOption{
	jwt.WithValidMethods([]string{"HS256"}),
	jwt.WithIssuer("https://issuer.example"),
	jwt.WithAudience("my-api"),
	jwt.WithExpirationRequired(),
},
```

Without `jwt.WithExpirationRequired()`, a token with no `exp` claim never expires.

A valid signature proves who issued the token, not what the user may do. Check roles or scopes in `Validate`, in your handlers, or with [Casbin Auth](/middleware/casbin/).

## Related

- [JWT recipe](/cookbook/jwt/): a complete program that issues and checks tokens.
- [Key Auth](/middleware/keyauth/): opaque API keys, with no claims.
- [Casbin Auth](/middleware/casbin/): decide what the token's subject may do.
- [Middleware overview](/middleware/overview/#contrib): other contrib packages.
