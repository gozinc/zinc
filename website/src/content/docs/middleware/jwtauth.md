---
title: JWT
description: Parse bearer tokens, validate claims, and expose token data to handlers.
---

`jwtauth` authenticates requests that carry a signed JSON Web Token. It reads the token, verifies the signature and standard claims such as expiry, and makes the claims available to handlers. Invalid or missing tokens get `401` with a `WWW-Authenticate` challenge.

It lives in [`github.com/0mjs/contrib`](/middleware/overview/#contrib) rather than in Zinc, because it depends on [golang-jwt](https://github.com/golang-jwt/jwt):

```sh
go get github.com/0mjs/contrib/jwtauth
```

You will usually import golang-jwt as well, for the token type the key function receives:

```go
import (
	"github.com/0mjs/contrib/jwtauth"
	"github.com/golang-jwt/jwt/v5"
)

api := app.Group("/api", jwtauth.New(jwtauth.Config{
	KeyFunc: func(_ *zinc.Context, token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
		}
		return signingKey, nil
	},
}))
```

`KeyFunc` or `ParseTokenFunc` is required.

:::caution[Pin the signing algorithm]
Check `token.Method` in the key function, as above, or pass `ParserOptions: []jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"})}`. Accepting whatever algorithm the token names is a classic JWT vulnerability.
:::

## Extractors

By default the token comes from `Authorization: Bearer <token>`. Others:

- `jwtauth.FromAuthorizationHeader("Bearer")`
- `jwtauth.FromHeader(name)`
- `jwtauth.FromHeaderPrefix(name, prefix)`
- `jwtauth.FromCookie(name)`
- `jwtauth.FromQuery(name)`
- `jwtauth.FromFirst(extractors...)`

## Config

| Field | Meaning |
|---|---|
| `Extractor` | Custom token source |
| `NewClaims` | Constructs the claims value before parsing |
| `KeyFunc` | Resolves the verification key for a token |
| `ParseTokenFunc` | Replaces the parser entirely |
| `ParserOptions` | Options forwarded to golang-jwt |
| `Validate` | Runs your own checks after verification |
| `SuccessHandler` | Replaces the success path, which calls `c.Next()` |
| `ErrorHandler` | Replaces unauthorized handling |
| `Realm` | Used in the `WWW-Authenticate` challenge |

## Typed claims

```go
type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

app.Use(jwtauth.New(jwtauth.Config{
	NewClaims: func(*zinc.Context) jwt.Claims { return &Claims{} },
	KeyFunc: func(_ *zinc.Context, token *jwt.Token) (any, error) {
		return signingKey, nil
	},
}))
```

Then in a handler:

```go
claims, ok := jwtauth.Claims[*Claims](c) // or jwtauth.MustClaims[*Claims](c)
token, ok := jwtauth.Get(c)              // token.Raw is the original string
```

## Failure behavior

Missing, malformed, and invalid tokens are distinct errors (`jwtauth.ErrTokenMissing`, `jwtauth.ErrTokenMalformed`, `jwtauth.ErrTokenInvalid`), and each gets a `WWW-Authenticate: Bearer ...` challenge unless you replace `ErrorHandler`.

For a production token policy, pin allowed algorithms, issuer, audience, and required expiry:

```go
ParserOptions: []jwt.ParserOption{
	jwt.WithValidMethods([]string{"HS256"}),
	jwt.WithIssuer("https://issuer.example"),
	jwt.WithAudience("my-api"),
	jwt.WithExpirationRequired(),
},
```

The default parser validates expiry when present but does not require every token to contain it. Signature validity alone does not grant application permissions.
