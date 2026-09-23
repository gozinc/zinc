---
title: Secure Headers
description: Set common security response headers.
---

`Secure` adds response headers that switch on browser protections against content sniffing, clickjacking, and referrer leaks. It is cheap and belongs in almost every app.

```go
app.Use(middleware.Secure())
```

Defaults include:

- `X-XSS-Protection: 0`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: SAMEORIGIN`
- `Referrer-Policy: no-referrer`
- `Cross-Origin-Resource-Policy: same-origin`

Use `SecureWithConfig` to add CSP or HSTS.

```go
app.Use(middleware.SecureWithConfig(middleware.SecureConfig{
	ContentSecurityPolicy: "default-src 'self'",
	HSTSMaxAge:            31536000,
}))
```

HSTS tells browsers to use HTTPS for your domain for `HSTSMaxAge` seconds. Zinc sends it only on HTTPS requests. Enable it once the whole site works over HTTPS, because browsers remember it.
