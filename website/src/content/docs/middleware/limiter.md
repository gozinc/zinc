---
title: Rate Limiter
description: Limit request rates per client, per API key, or globally with a token bucket, and cap concurrent requests.
---

`limiter` refuses requests that arrive faster than a set rate, answering `429 Too Many Requests`. It uses a token bucket: each request spends one token, and tokens refill at a steady rate up to a maximum burst.

For most public APIs, limit per client IP:

```go
import "github.com/0mjs/zinc/middleware/limiter"

app.Use(limiter.New(limiter.Config{
	Rate:     20, // tokens per second
	Capacity: 40, // largest burst
	Key:      (*zinc.Context).IP,
}))
```

Configure [trusted proxies](/guide/ip-address/) first when the app runs behind a load balancer. Otherwise every request shares the load balancer's IP and one bucket.

## Choose what to limit by

`Key` returns the bucket for a request: a client IP, an API key, a user ID. Without `Key`, **one bucket is shared by every request**.

:::caution[limiter.New() is global]
`limiter.New()` without a `Key` caps the whole app, or the route it is attached to, at 10 requests per second in total. That suits a single expensive endpoint, such as `app.Post("/exports", limiter.New(), startExport)`, but not an entire service.
:::

## Limit by API key

```go
app.Use(limiter.New(limiter.Config{
	Rate:     30,
	Capacity: 60,
	Key: func(c *zinc.Context) string {
		return c.Header("X-API-Key")
	},
	LimitReached: func(c *zinc.Context) error {
		c.SetHeader("Retry-After", "1")
		return zinc.TooManyRequests("rate limit exceeded")
	},
}))
```

## Config

| Field | Default | Meaning |
|---|---|---|
| `Rate` | `10` | Tokens added per second |
| `Capacity` | `10` | Largest burst |
| `Key` | none: one global bucket | Returns the bucket key for a request |
| `LimitReached` | returns a `429` error with the message `rate limit exceeded` | Answers a request over the limit |
| `MaxKeys` | `10000` | Most buckets kept at once |
| `MaxKeyBytes` | `256` | Longest key kept |
| `IdleTTL` | five minutes | Idle time before a full bucket may expire |
| `Now` | `time.Now` | Clock, for deterministic tests |

Negative or non-finite settings panic when the middleware is created.

## Memory use

Keyed limiters keep at most `MaxKeys` buckets. When full, new keys get the limit response, while existing quotas stay in force. A bucket expires once it has been idle for `IdleTTL` and its tokens have fully refilled. Cleanup is bounded and runs during requests, with no background goroutine. Size `MaxKeys` for your expected client population: when it is full, new clients are denied rather than given a fresh quota by evicting someone else's.

## Limit concurrent requests

`limiter.Concurrency(n)` bounds how many requests run downstream at once, and answers `429` to any beyond it instead of queueing them:

```go
app.Post("/reports", limiter.Concurrency(4), buildReport)
```

## Related

- [Client IP and Proxies](/guide/ip-address/) makes per-IP keys accurate.
