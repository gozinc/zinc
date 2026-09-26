---
title: Secure Headers
description: Set common security response headers.
---

`secure` adds response headers that switch on browser protections against content sniffing, clickjacking, and referrer leaks. It is cheap and belongs in almost every app.

```go
import "github.com/0mjs/zinc/middleware/secure"

app.Use(secure.New())
```

Defaults include:

- `X-XSS-Protection: 0`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: SAMEORIGIN`
- `Referrer-Policy: no-referrer`
- `Cross-Origin-Resource-Policy: same-origin`

Set a field in `secure.Config` to add a header or change a default; fields you leave empty keep their defaults.

```go
app.Use(secure.New(secure.Config{
	ContentSecurityPolicy: "default-src 'self'",
	HSTSMaxAge:            31536000,
}))
```

HSTS tells browsers to use HTTPS for your domain for `HSTSMaxAge` seconds. Zinc sends it only on HTTPS requests. Enable it once the whole site works over HTTPS, because browsers remember it.
