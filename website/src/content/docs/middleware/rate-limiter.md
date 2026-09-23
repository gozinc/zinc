---
title: Rate Limiter
description: Limit request rates per client, per API key, or globally with a token bucket.
---

`RateLimiter` refuses requests that arrive faster than a set rate, answering `429 Too Many Requests`. It uses a token bucket: each request spends one token, and tokens refill at a steady rate up to a maximum burst.

For most public APIs, limit per client IP:

```go
app.Use(middleware.IPRateLimiter(20, 40)) // 20 requests per second, bursts up to 40
```

Configure [trusted proxies](/guide/ip-address/) first when the app runs behind a load balancer. Otherwise every request shares the load balancer's IP and one bucket.

## Choose what to limit by

| Constructor | One bucket per |
|---|---|
| `IPRateLimiter(rate, burst)` | Client IP, from `c.IP()` |
| `RateLimiter(config)` with `KeyGenerator` | Whatever the function returns, such as an API key or user ID |
| `RateLimiter()` with no arguments | Nothing: **one bucket shared by every request** |

:::caution[RateLimiter() is global]
`RateLimiter()` without a config caps the whole app, or the route it is attached to, at 10 requests per second in total. That suits a single expensive endpoint, such as `app.Post("/exports", middleware.RateLimiter(), startExport)`, but not an entire service.
:::

## Limit by API key

```go
app.Use(middleware.RateLimiter(middleware.RateLimiterConfig{
	Rate:     30, // tokens per second
	Capacity: 60, // maximum burst
	KeyGenerator: func(c *zinc.Context) string {
		return c.GetHeader("X-API-Key")
	},
	LimitReachedHandler: func(c *zinc.Context) error {
		c.SetHeader("Retry-After", "1")
		return c.Status(zinc.StatusTooManyRequests).JSON(zinc.Map{"error": "rate limit exceeded"})
	},
}))
```

When you pass a config, set both `Rate` and `Capacity`. They default to `0`, which rejects every request.

## Configuration

| Field | Default | Meaning |
|---|---|---|
| `Rate` | `10` without a config, else required | Tokens added per second |
| `Capacity` | `10` without a config, else required | Largest burst |
| `KeyGenerator` | none | Returns the bucket key for a request |
| `IPLookup` | none | Returns a client identity; used when `KeyGenerator` is not set |
| `StatusCode` | `429` | Status when the limit is hit |
| `LimitReachedHandler` | plain-text `429` | Writes the response when the limit is hit |

Keys are chosen in this order: `KeyGenerator`, then `IPLookup`, then one global bucket.

## Memory use

Each distinct key keeps its bucket for the life of the process. With keys from a bounded set, such as your API keys or user IDs, that is fine. Per-IP limiting on public traffic grows with the number of distinct clients. Restart periodically, or enforce the limit at your load balancer or API gateway.

## Related

- [Throttle](/middleware/utility/#throttle) limits concurrent requests instead of request rate.
- [Client IP and Proxies](/guide/ip-address/) makes per-IP keys accurate.
