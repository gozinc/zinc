---
title: CSRF
description: Stop other websites from sending forms and requests that ride on your users' cookies.
---

CSRF protection stops another website from making a request to your app with your user's cookies attached. Without it, a page on `evil.example` can submit a hidden form to `/transfer`, and the browser sends your session cookie along. Add it when you sign users in with cookies; APIs that authenticate with an `Authorization` header don't need it, because browsers never attach that header on their own.

## Usage

```go
import "github.com/0mjs/zinc/middleware/csrf"

app.Use(csrf.New())

app.Get("/form", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"csrf": csrf.Token(c)})
})

app.Post("/transfer", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"verified": csrf.MustGet(c).Verified})
})
```

A `GET` gives the browser a token in a cookie. Your page reads it (or you render it into the page) and sends it back in the `X-CSRF-Token` header:

```bash
curl -i -c jar http://localhost:8080/form
# HTTP/1.1 200 OK
# Set-Cookie: _csrf=eoSMscxgGaK8BcIiFhie4shHt7PkzOJr3rxqRDwhSNs; Path=/; Expires=Tue, 29 Sep 2026 00:16:26 GMT; Max-Age=86400; SameSite=Lax
# Vary: Cookie
# {"csrf":"eoSMscxgGaK8BcIiFhie4shHt7PkzOJr3rxqRDwhSNs"}

curl -b jar -X POST -H 'X-CSRF-Token: eoSMscxgGaK8BcIiFhie4shHt7PkzOJr3rxqRDwhSNs' \
  http://localhost:8080/transfer
# {"verified":true}

curl -i -b jar -X POST http://localhost:8080/transfer
# HTTP/1.1 400 Bad Request
# {"error":{"status":400,"message":"Bad Request"}}
```

This works because another site can make the browser *send* your cookie, but it can't *read* it. Only your own pages can copy the token into the header, so a forged request arrives without it.

`GET`, `HEAD`, `OPTIONS` and `TRACE` requests are never checked, so keep them free of side effects. Every other method must carry the token.

## Defaults

| Setting | Default |
|---|---|
| Token | 32 random bytes, base64url-encoded (43 characters) |
| Token read from | the `X-CSRF-Token` header |
| Cookie | `_csrf`, `Path=/`, `Max-Age=86400` (24 hours), `SameSite=Lax`, not `HttpOnly`, not `Secure` |
| Checked methods | everything except `GET`, `HEAD`, `OPTIONS`, `TRACE` |
| Cross-site requests | rejected, based on the browser's `Sec-Fetch-Site` header |

The cookie isn't `HttpOnly` so that your JavaScript can read the token and put it in the header. Each response that passes through the middleware sends the cookie again, which renews its lifetime; the token itself stays the same.

## Configuration

Accept the token from an HTML form field as well as the header, harden the cookie, and trust a sibling origin:

```go
app.Use(csrf.New(csrf.Config{
	Readers: []csrf.Reader{
		csrf.FromFirst(
			csrf.FromHeader(zinc.HeaderXCSRFToken),
			csrf.FromForm("_csrf"),
		),
	},
	Cookie: csrf.Cookie{
		Name:     "__Host-csrf",
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	},
	ExposeHeader:   zinc.HeaderXCSRFToken,
	TrustedOrigins: []string{"https://admin.example.com"},
}))
```

```bash
curl -i https://example.com/form
# HTTP/1.1 200 OK
# Set-Cookie: __Host-csrf=w9rRa38qHQ5IVk0QflIUP_phvCnQWL-FbZ7WVXMfS-E; Path=/; Expires=Tue, 29 Sep 2026 00:16:26 GMT; Max-Age=86400; Secure; SameSite=Strict
# Vary: Cookie
# X-Csrf-Token: w9rRa38qHQ5IVk0QflIUP_phvCnQWL-FbZ7WVXMfS-E
```

In a server-rendered form, put the token in a hidden field named `_csrf`:

```html
<input type="hidden" name="_csrf" value="{{ .CSRF }}">
```

where `.CSRF` is `csrf.Token(c)` passed to your template.

| Field | Default | Meaning |
|---|---|---|
| `Readers` | `[FromHeader("X-CSRF-Token")]` | Where to look for the token on checked requests. The request passes if any reader finds a matching token. An empty non-nil list panics. |
| `TokenBytes` | `32` | Random bytes per token, for the default generator. |
| `Generate` | random, base64url | Makes a new token. Replaces the default generator, so `TokenBytes` is ignored. |
| `Cookie` | see below | The cookie that holds the token. |
| `ExposeHeader` | none | A response header to copy the token into, such as `X-CSRF-Token`, for clients that read headers rather than cookies. |
| `TrustedOrigins` | none | Origins, such as `https://admin.example.com`, allowed to send same-site or cross-site requests. Each must be a scheme and host with no path; a bad entry panics. |
| `AllowFetchSite` | none | A function that allows or rejects a same-site or cross-site request that isn't from a trusted origin. |
| `ErrorHandler` | returns the error | Runs when a request fails a check. Its return value is what the client gets. |

`csrf.Cookie` fields:

| Field | Default | Meaning |
|---|---|---|
| `Name` | `_csrf` | Cookie name. |
| `Domain` | none | Cookie domain. |
| `Path` | `/` | Cookie path. |
| `MaxAge` | `86400` | Lifetime in seconds. |
| `Secure` | `false` | Send only over HTTPS. Forced on when `SameSite` is `None`. |
| `HTTPOnly` | `false` | Hide the cookie from JavaScript. Turn on only if you render the token into the page or use `ExposeHeader`. |
| `SameSite` | `Lax` | Cookie `SameSite` mode. |

Fields left at their zero value keep the defaults above.

## Check where a request came from

Modern browsers send a `Sec-Fetch-Site` header saying where a request started: `same-origin` (your own pages), `same-site` (another subdomain of your site), `cross-site` (anyone else), or `none` (the user typed the URL). The middleware uses it as a second check on top of the token.

On checked methods, `same-origin`, `none` and a missing header pass. `same-site` and `cross-site` are rejected with `403`, unless the request's `Origin` is in `TrustedOrigins` or `AllowFetchSite` returns `true`:

```go
AllowFetchSite: func(c *zinc.Context, d csrf.Decision) (bool, error) {
	// d.Site is csrf.FetchSiteSameSite or csrf.FetchSiteCrossSite; d.Origin is the Origin header.
	return d.Site == csrf.FetchSiteSameSite && strings.HasSuffix(d.Origin, ".example.com"), nil
},
```

A request that passes this check still needs a valid token.

## Reading state

```go
token := csrf.Token(c)     // "" without the middleware
state, ok := csrf.Get(c)   // ok is false without the middleware
state := csrf.MustGet(c)   // panics without the middleware
```

| `State` field | Meaning |
|---|---|
| `Token` | The token for this request. |
| `Issued` | `true` if this request created a new token. |
| `Verified` | `true` if this request was checked and passed. |
| `CookieName` | The cookie that holds the token. |
| `FetchSite` | The request's `Sec-Fetch-Site` value, lower-cased. |

## Errors

Failed checks come back as a `*csrf.Violation`:

| `Reason` | `errors.Is` target | When | Status |
|---|---|---|---|
| `csrf.ReasonTokenMissing` | `csrf.ErrTokenMissing` | No reader found a token | `400` |
| `csrf.ReasonCookieMissing` | `csrf.ErrCookieMissing` | The request has no token cookie | `403` |
| `csrf.ReasonTokenInvalid` | `csrf.ErrTokenInvalid` | The token doesn't match the cookie | `403` |
| `csrf.ReasonFetchSiteRejected` | `csrf.ErrFetchSiteRejected` | `Sec-Fetch-Site` check failed | `403` |

The checks run in this order: fetch site, then cookie, then token. A reader that fails for another reason, such as a form body that can't be parsed, gives a plain `400` rather than a `Violation`.

Use `ErrorHandler` to send your own body while keeping the reason:

```go
ErrorHandler: func(c *zinc.Context, err error) error {
	var v *csrf.Violation
	if errors.As(err, &v) {
		return c.Status(http.StatusForbidden).JSON(zinc.Map{"error": "csrf", "reason": v.Reason})
	}
	return err
},
```

```bash
curl -X POST -b '_csrf=abc' http://localhost:8080/transfer
# {"error":"csrf","reason":"token_missing"}
```

To list these in the [OpenAPI](/guide/openapi/#describe-what-middleware-adds) spec, pass `csrf.Doc()` to `Document` beside the middleware. It describes the default readers: on `POST`, `PUT`, `PATCH` and `DELETE`, the `X-CSRF-Token` header as a security scheme named `csrf`, required with the route's own security, and `400` and `403`. A generated client sets the token once rather than on every call. With other `Readers`, write your own `zinc.MiddlewareDoc`.

```go
api := app.Group("/api", csrf.New()).Document(csrf.Doc())
```

## Related

- [Session](/middleware/session/): the cookie-based sign-in that CSRF protection guards.
- [CORS](/middleware/cors/): which other origins may read your API's responses.
- [Secure Headers](/middleware/secure/): other browser protections.
- [Cookies](/guide/cookies/): cookie attributes in Zinc.
