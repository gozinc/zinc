---
title: Client IP and Proxies
description: Read the client's IP address correctly, both directly and behind load balancers and reverse proxies you trust.
---

Behind a load balancer, every request appears to come from the load balancer. The real client address travels in a forwarding header such as `X-Forwarded-For`, but any client can send that header too. Zinc reads it only when the request comes from a proxy you have said you trust.

## The two helpers

| Helper | Returns | Trusts headers? |
|---|---|---|
| `c.RemoteIP()` | The address of the direct peer, from `Request.RemoteAddr` | Never |
| `c.IP()` | The first address in the forwarding header when the peer is trusted, otherwise the peer address | Only from trusted proxies |

`c.IPs()` returns the whole forwarded chain.

## Configure trusted proxies

List the addresses or CIDR ranges of the proxies in front of your app:

```go
cfg := zinc.DefaultConfig
cfg.ProxyHeader = zinc.HeaderXForwardedFor // the default
cfg.TrustedProxies = []string{
	"10.0.0.0/8",     // internal load balancers
	"192.168.0.0/16",
}
app := zinc.NewWithConfig(cfg)

app.Get("/whoami", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"client": c.IP(),
		"chain":  c.IPs(),
		"peer":   c.RemoteIP(),
	})
})
```

With no trusted proxies, the default, `c.IP()` and `c.RemoteIP()` return the same value.

:::danger[Only trust proxies you control]
Trusting forwarding headers from an address you do not operate lets any client choose its own IP. That silently breaks rate limits, audit logs, and IP allow lists.
:::

### Make sure the proxy overwrites the header

`c.IP()` returns the **first**, leftmost address in the header. Many load balancers append to an `X-Forwarded-For` header the client already sent instead of replacing it. A client that sends `X-Forwarded-For: 203.0.113.9` then appears as `203.0.113.9`, even behind a trusted proxy.

Configure the proxy at your edge to **replace** the header with the address it observed, or read a header that only the proxy sets:

- nginx: `proxy_set_header X-Real-IP $remote_addr;` with `ProxyHeader = "X-Real-IP"`
- Cloudflare: `ProxyHeader = "CF-Connecting-IP"`, trusting only Cloudflare's published ranges

Whatever header you choose, trust only the addresses of the proxies that set it.

## Where this matters

[Rate Limiter](/middleware/rate-limiter/)'s per-IP mode and the [RealIP](/middleware/utility/#realip) middleware both use `c.IP()`. Configure proxies before relying on either.

Trust entries are validated and copied at application construction. Invalid IP addresses or CIDRs panic. Zinc walks forwarded addresses from right to left and stops at the first untrusted hop; values to its left are ignored. Malformed addresses encountered during that walk fall back to the direct peer. Configure every proxy you operate and ensure it appends or overwrites forwarding headers correctly.

`Scheme()` accepts only `http` or `https` from the last `X-Forwarded-Proto` value supplied by a trusted direct peer. Direct TLS always returns `https`. Proxies should overwrite this header with the protocol they have verified.
