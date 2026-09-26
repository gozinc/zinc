---
title: Utility
description: Small middleware helpers for caching, health checks, real IPs, throttling, and conditional middleware.
---

These helpers cover small request and response behaviors that show up often.

## NoCache

```go
app.Use(middleware.NoCache())
```

`NoCache` sets headers that stop browsers and proxies from caching the response.

Use it for auth pages, dashboards, and any route where cached browser history is not wanted.

## Heartbeat

```go
app.Use(middleware.Heartbeat("/healthz"))
```

Requests to the heartbeat path return `204 No Content` before route handlers run.

Use this for load balancers and uptime checks when the check does not need application logic.

## RealIP

```go
app.Use(middleware.RealIP())
```

`RealIP` replaces `Request.RemoteAddr` with `c.IP()`, so later code and standard handlers that read `RemoteAddr` see the client address.

It is only as trustworthy as your proxy configuration. Set `TrustedProxies` first, and read [Client IP and Proxies](/guide/ip-address/) before relying on it.

```go
app := zinc.New(zinc.Config{TrustedProxies: []string{"10.0.0.0/8"}})
app.Use(middleware.RealIP())
```

## Throttle

```go
app.Use(middleware.Throttle(64))
```

`Throttle` limits how many requests run at once. Requests over the limit get `429 Too Many Requests` immediately rather than waiting. It is different from [Rate Limiter](/middleware/rate-limiter/), which limits requests per second.

Use `Throttle` when the protected resource is concurrency-sensitive: database pools, expensive exports, slow upstream calls, and similar work.

## Maybe

```go
app.Use(middleware.Maybe(func(c *zinc.Context) bool {
	return c.GetHeader("X-Debug") == "1"
}, debugMiddleware))
```

`Maybe` applies a middleware only when the predicate returns true.

That keeps conditional middleware out of the handler.

## SetHeader

```go
app.Use(middleware.SetHeader("X-App", "zinc"))
```

`SetHeader` writes a response header and continues the chain.

Use it for small static headers. Use `Secure` for common browser security headers.
