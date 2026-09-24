---
title: JWT
description: Parse bearer tokens, validate claims, and expose token data to handlers.
---

`JWT` authenticates requests that carry a signed JSON Web Token. It reads the token, verifies the signature and standard claims such as expiry, and makes the claims available to handlers. Invalid or missing tokens get `401` with a `WWW-Authenticate` challenge.

## Quick start

```go
api := app.Group("/api", middleware.JWT(func(_ *zinc.Context, token *jwt.Token) (any, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
	}
	return signingKey, nil
}))
```

:::caution[Pin the signing algorithm]
Check `token.Method` in the key function, as above, or pass `ParserOptions: []jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"})}` in `JWTConfig`. Accepting whatever algorithm the token names is a classic JWT vulnerability.
:::

## Default extraction

By default Zinc reads bearer tokens from the `Authorization` header.

```http
Authorization: Bearer <token>
```

## Extractor helpers

- `JWTFromAuthHeader("Bearer")`
- `JWTFromHeader(name)`
- `JWTFromHeaderPrefix(name, prefix)`
- `JWTFromCookie(name)`
- `JWTFromQuery(name)`
- `JWTFromFirst(extractors...)`

## Config fields

| Field | Meaning |
|---|---|
| `Skipper` | Skip auth for selected requests |
| `Extractor` | Custom token source |
| `NewClaims` | Construct the claims value before parsing |
| `KeyFunc` | Resolve the verification key for a token |
| `ParseTokenFunc` | Replace Zinc's parser entirely |
| `ParserOptions` | Forward parser options to `jwt/v5` |
| `Validate` | Run custom post-parse validation |
| `SuccessHandler` | Override the success path |
| `ErrorHandler` | Override unauthorized handling |
| `Realm` | Used in the `WWW-Authenticate` challenge |

## Typed claims

```go
type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

app.Use(middleware.JWTWithConfig(middleware.JWTConfig{
	NewClaims: func(*zinc.Context) jwt.Claims {
		return &Claims{}
	},
	KeyFunc: func(_ *zinc.Context, token *jwt.Token) (any, error) {
		return signingKey, nil
	},
}))
```

Then in a handler:

```go
claims, ok := middleware.JWTClaims[*Claims](c)
token, ok := middleware.JWTToken(c)
raw, ok := middleware.JWTTokenString(c)
```

## Failure behavior

When validation fails, Zinc sets a `WWW-Authenticate: Bearer ...` challenge automatically unless you override `ErrorHandler`.

Malformed tokens are treated differently from missing tokens so you can distinguish:

- missing credentials
- malformed credentials
- invalid credentials

## When to customize `ParseTokenFunc`

Use `ParseTokenFunc` only when Zinc's normal `KeyFunc` + `NewClaims` path is too small for your needs. Most apps should stick to the standard config flow.

For a production token policy, explicitly pin allowed algorithms, issuer, audience, and required expiry:

```go
ParserOptions: []jwt.ParserOption{
    jwt.WithValidMethods([]string{"HS256"}),
    jwt.WithIssuer("https://issuer.example"),
    jwt.WithAudience("my-api"),
    jwt.WithExpirationRequired(),
},
```

These are options for `JWTConfig`; use the issuer and audience from your own token contract. The default parser validates expiry when present but does not require every token to contain it. Signature validity alone does not grant application permissions.
