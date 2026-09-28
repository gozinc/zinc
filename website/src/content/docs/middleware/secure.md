---
title: Secure Headers
description: Turn on the browser's protections against content sniffing, clickjacking and referrer leaks with a set of response headers.
---

Secure Headers adds the response headers that tell browsers to turn on their built-in protections: no guessing content types, no framing your pages on other sites, no leaking URLs in the `Referer` header. It costs one line and suits almost every app, API or website.

## Usage

```go
import "github.com/0mjs/zinc/middleware/secure"

app.Use(secure.New())
```

```bash
curl -i http://localhost:8080/
# HTTP/1.1 200 OK
# Cross-Origin-Resource-Policy: same-origin
# Referrer-Policy: no-referrer
# X-Content-Type-Options: nosniff
# X-Frame-Options: SAMEORIGIN
# X-Xss-Protection: 0
```

The headers are set before your handler runs, so they're on every response, errors included.

## Defaults

| Header | Default | What it does |
|---|---|---|
| `X-XSS-Protection` | `0` | Turns off the old browser XSS filter, which could itself be abused. |
| `X-Content-Type-Options` | `nosniff` | Browsers use your `Content-Type` instead of guessing. |
| `X-Frame-Options` | `SAMEORIGIN` | Only your own pages can show yours in a frame. |
| `Referrer-Policy` | `no-referrer` | Links to other sites don't reveal the page URL. |
| `Cross-Origin-Resource-Policy` | `same-origin` | Other sites can't embed your responses. |
| `Strict-Transport-Security` | not sent | Set `HSTSMaxAge` to turn it on. |
| `Content-Security-Policy` | not sent | Set `ContentSecurityPolicy`. |
| `Permissions-Policy` | not sent | Set `PermissionsPolicy`. |

## Configuration

A site served over HTTPS, with a content security policy and no camera or microphone access:

```go
app.Use(secure.New(secure.Config{
	ContentSecurityPolicy: "default-src 'self'",
	PermissionsPolicy:     "camera=(), microphone=(), geolocation=()",
	XFrameOptions:         "DENY",
	ReferrerPolicy:        "strict-origin-when-cross-origin",
	HSTSMaxAge:            31536000, // one year
}))
```

```bash
curl -i https://example.com/
# HTTP/1.1 200 OK
# Content-Security-Policy: default-src 'self'
# Cross-Origin-Resource-Policy: same-origin
# Permissions-Policy: camera=(), microphone=(), geolocation=()
# Referrer-Policy: strict-origin-when-cross-origin
# Strict-Transport-Security: max-age=31536000; includeSubDomains
# X-Content-Type-Options: nosniff
# X-Frame-Options: DENY
# X-Xss-Protection: 0
```

| Field | Default | Header |
|---|---|---|
| `XSSProtection` | `"0"` | `X-XSS-Protection` |
| `ContentTypeNosniff` | `"nosniff"` | `X-Content-Type-Options` |
| `XFrameOptions` | `"SAMEORIGIN"` | `X-Frame-Options` |
| `ReferrerPolicy` | `"no-referrer"` | `Referrer-Policy` |
| `CrossOriginResourcePolicy` | `"same-origin"` | `Cross-Origin-Resource-Policy` |
| `ContentSecurityPolicy` | none | `Content-Security-Policy` |
| `ContentSecurityPolicyReportOnly` | none | `Content-Security-Policy-Report-Only`: reports violations without blocking. |
| `PermissionsPolicy` | none | `Permissions-Policy` |
| `HSTSMaxAge` | `0` (off) | `Strict-Transport-Security: max-age=<seconds>`, on HTTPS requests only. |
| `HSTSExcludeSubdomains` | `false` | Leaves `; includeSubDomains` off the HSTS header. |

Empty strings keep the default, so you can change a default header but not remove it through `Config`. To drop one for a single route, delete it in the handler:

```go
app.Get("/embed", func(c *zinc.Context) error {
	c.Writer().Header().Del(zinc.HeaderXFrameOptions) // this page may be framed
	return c.HTML(widgetHTML) // your markup
})
```

## Turn on HSTS

HSTS tells browsers to use only HTTPS for your domain, for `HSTSMaxAge` seconds. The header is sent only on HTTPS requests: a direct TLS connection, or `X-Forwarded-Proto: https` from a [trusted proxy](/guide/ip-address/#https-behind-a-proxy).

```go
secure.New(secure.Config{HSTSMaxAge: 300, HSTSExcludeSubdomains: true})
```

```text
Strict-Transport-Security: max-age=300
```

:::caution[Browsers remember HSTS]
Once a browser has seen the header, it refuses plain HTTP for your domain until `max-age` runs out. `includeSubDomains` is on by default, so every subdomain needs HTTPS too. Start with a short `HSTSMaxAge`, such as `300`, and raise it once everything works.
:::

## Try a content security policy

A content security policy can break scripts and styles your pages rely on. Send it as report-only first: the browser logs what it would have blocked and blocks nothing.

```go
secure.New(secure.Config{
	ContentSecurityPolicyReportOnly: "default-src 'self'; report-uri /csp",
})
```

```text
Content-Security-Policy-Report-Only: default-src 'self'; report-uri /csp
```

When the reports are clean, move the value to `ContentSecurityPolicy`.

## Related

- [CORS](/middleware/cors/): let other origins call your API.
- [CSRF](/middleware/csrf/): stop forged requests that use your users' cookies.
- [No Cache](/middleware/nocache/): headers that stop caching of sensitive responses.
- [Client IP and Proxies](/guide/ip-address/): when Zinc treats a request as HTTPS.
