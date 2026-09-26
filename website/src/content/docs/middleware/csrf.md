---
title: CSRF
description: Cookie-backed CSRF protection with configurable token readers and fetch metadata checks.
---

`csrf` issues a token for safe requests and verifies it on unsafe requests.

```go
import "github.com/0mjs/zinc/middleware/csrf"

app.Use(csrf.New())
```

By default Zinc:

- stores the token in a cookie named `_csrf`
- reads request tokens from `X-CSRF-Token`
- issues tokens on safe methods like `GET` and `HEAD`
- verifies tokens on unsafe methods like `POST`, `PUT`, `PATCH`, and `DELETE`

Put the token in a page or form with `csrf.Token(c)`.

## Custom readers

Combine readers when your app accepts tokens from more than one place:

```go
app.Use(csrf.New(csrf.Config{
	Readers: []csrf.Reader{
		csrf.FromFirst(
			csrf.FromHeader(zinc.HeaderXCSRFToken),
			csrf.FromForm("_csrf"),
			csrf.FromQuery("csrf"),
		),
	},
	ExposeHeader: zinc.HeaderXCSRFToken,
}))
```

## Config

| Field | Meaning |
|---|---|
| `Readers` | Token readers for unsafe requests |
| `Generate` | Custom token generator |
| `TokenBytes` | Random token size for the default generator |
| `Cookie` | Cookie name and attributes, as a `csrf.Cookie` |
| `ExposeHeader` | Optional response header that publishes the token |
| `TrustedOrigins` | Additional origins accepted for fetch metadata checks |
| `AllowFetchSite` | Custom fetch-site decision hook |
| `ErrorHandler` | Replaces CSRF failure behavior |

`csrf.Cookie` has `Name`, `Domain`, `Path`, `MaxAge`, `Secure`, `HTTPOnly`, and `SameSite`. With `SameSite=None`, Zinc forces `Secure=true`.

## Reading CSRF state

```go
token := csrf.Token(c)     // "" without the middleware
state, ok := csrf.Get(c)   // or csrf.MustGet(c)
```

The state holds the token, whether it was newly issued, whether the request was verified, the cookie name, and the normalized fetch-site value.

## Failure model

Failures are returned as `*csrf.Violation`, with a `Reason` such as `csrf.ReasonTokenMissing`, `ReasonCookieMissing`, `ReasonTokenInvalid`, or `ReasonFetchSiteRejected`. You can shape API error responses without losing why the request was rejected.
