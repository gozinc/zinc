---
title: Rate Limiter
description: Refuse requests over a set rate with 429, per client IP, per API key or across the app, and cap concurrent requests.
---

Rate Limiter refuses requests that arrive faster than a rate you set, with `429 Too Many Requests`. Add it to public APIs so one client can't use up capacity meant for everyone, or to an expensive endpoint so it can't be called in a tight loop.

## Usage

For most public APIs, limit each client IP:

```go
import "github.com/0mjs/zinc/middleware/limiter"

app.Use(limiter.New(limiter.Config{
	Rate:     20, // tokens added per second
	Capacity: 40, // largest burst
	Key:      (*zinc.Context).IP,
}))
```

```bash
curl -i http://localhost:8080/   # once the client's burst is used up
# HTTP/1.1 429 Too Many Requests
# Retry-After: 1
# {"error":{"status":429,"message":"rate limit exceeded"}}
```

`Retry-After` says how many seconds until the client's next token, rounded up to a whole second. A `LimitReached` handler can change it or remove it.

Each client gets a bucket of `Capacity` tokens. Each request spends one, and tokens refill at `Rate` per second, so a client can send a burst of 40 and then 20 a second after that.

Configure [trusted proxies](/guide/ip-address/) first when the app runs behind a load balancer. Otherwise every request has the load balancer's IP, and all clients share one bucket.

:::caution[Without Key, the limit is for everyone]
`limiter.New()` with no `Key` keeps one bucket for every request it sees: 10 requests a second in total, across all clients. That suits one expensive endpoint, such as `app.Post("/exports", limiter.New(), startExport)`, but not a whole service.
:::

## Defaults

| Setting | Default |
|---|---|
| Rate | 10 tokens per second |
| Burst | 10 |
| Key | none: one bucket shared by every request |
| Response over the limit | `429` with the message `rate limit exceeded`, and `Retry-After` in seconds |
| Most buckets kept | 10,000 |
| Longest key | 256 bytes |
| Idle time before a bucket expires | 5 minutes |

## Configuration

This limits each API key, and tells clients when to retry:

```go
app.Use(limiter.New(limiter.Config{
	Rate:     30,
	Capacity: 60,
	Key: func(c *zinc.Context) string {
		return c.Header("X-API-Key")
	},
	LimitReached: func(c *zinc.Context) error {
		c.SetHeader("Retry-After", "1")
		return zinc.TooManyRequests("slow down")
	},
}))
```

```bash
curl -i -H 'X-API-Key: k1' http://localhost:8080/   # over k1's limit
# HTTP/1.1 429 Too Many Requests
# Retry-After: 1
# {"error":{"status":429,"message":"slow down"}}
```

| Field | Default | Meaning |
|---|---|---|
| `Rate` | `10` | Tokens added per second. May be a fraction, such as `0.5` for one every two seconds. |
| `Capacity` | `10` | Largest burst. Must be at least `1`. |
| `Key` | none | `func(*zinc.Context) string` that returns the bucket for a request: an IP, an API key, a user ID. Nil uses one bucket for every request. |
| `LimitReached` | returns a `429` error with the message `rate limit exceeded` | Handler that answers a request over the limit |
| `MaxKeys` | `10000` | Most buckets kept at once |
| `MaxKeyBytes` | `256` | Longest key allowed. A request whose key is longer gets the limit response. |
| `IdleTTL` | 5 minutes | How long a bucket must sit idle, and full, before it can be removed |
| `Now` | `time.Now` | Clock, for tests |

A zero value keeps the default. A negative, NaN or infinite value, or a `Capacity` below `1`, panics when the middleware is created.

:::note[Requests with an empty key share a bucket]
In the example above, every request without an `X-API-Key` header has the key `""`, so they all share one bucket. Reject those requests earlier, with [Key Auth](/middleware/keyauth/), if they shouldn't reach the limiter at all.
:::

## Limit concurrent requests

`limiter.Concurrency(n)` caps how many requests run at the same time, rather than how many arrive per second. Requests over the cap get `429` straight away instead of waiting:

```go
app.Post("/reports", limiter.Concurrency(4), buildReport) // buildReport: your handler
```

```bash
curl -i -X POST http://localhost:8080/reports   # while 4 reports are already running
# HTTP/1.1 429 Too Many Requests
# {"error":{"status":429,"message":"Too Many Requests"}}
```

`n` must be greater than zero.

## Memory use

A limiter with a `Key` keeps at most `MaxKeys` buckets. When it's full, requests with a new key get the limit response, and clients already tracked keep their buckets. Size `MaxKeys` for the number of clients you expect at once.

A bucket is removed once it has been idle for `IdleTTL` and has refilled completely, so removing it never gives a client a fresh quota early. Cleanup happens during requests, a few buckets at a time; there's no background goroutine.

## Errors

A request over the limit gets whatever `LimitReached` returns. The default is `zinc.TooManyRequests("rate limit exceeded")`, which the default error handler sends as `429`. `limiter.Concurrency` returns `zinc.ErrTooManyRequests`.

To list the `429` in the [OpenAPI](/guide/openapi/#describe-what-middleware-adds) spec, pass `limiter.Doc()` to `Document` beside the middleware. It describes the default `LimitReached`.

```go
app.Use(limiter.New())
app.Document(limiter.Doc())
```

## Related

- [Client IP and proxies](/guide/ip-address/) makes per-IP keys accurate behind a load balancer.
- [Key Auth](/middleware/keyauth/) checks API keys before they reach the limiter.
- [Context Timeout](/middleware/timeout/) stops slow requests from holding their slot.
