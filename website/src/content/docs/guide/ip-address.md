---
title: Client IP and Proxies
description: Read the client's IP address correctly, both directly and behind load balancers and reverse proxies you trust.
---

Behind a load balancer, every request appears to come from the load balancer. The real client address travels in a forwarding header such as `X-Forwarded-For`, but any client can send that header too. Zinc reads it only when the request comes from a proxy you have said you trust.

## The two helpers

| Helper | Returns | Trusts headers? |
|---|---|---|
| `c.RemoteIP()` | The address of the direct peer, from `Request.RemoteAddr` | Never |
| `c.IP()` | The nearest untrusted address in the forwarding header when the peer is trusted, otherwise the peer address | Only from trusted proxies |

`c.IPs()` returns the verified part of the chain, starting with the address `c.IP()` returns.

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

### How Zinc reads the chain

Each proxy appends the address it received the request from, so the rightmost entries are the ones your own proxies wrote. Zinc walks the header from right to left, skipping addresses you trust, and returns the first one you do not. Anything to the left of that hop could have been written by the client, so it is ignored.

A client that sends `X-Forwarded-For: 203.0.113.9` through a trusted load balancer arrives as `203.0.113.9, 198.51.100.4`. Zinc returns `198.51.100.4`, the address the load balancer saw, and not the value the client chose. Malformed entries make Zinc fall back to the direct peer.

List **every** proxy you operate. If an inner proxy is missing, `c.IP()` returns that proxy's address for every request.

Proxies that set a single-value header also work:

- nginx: `proxy_set_header X-Real-IP $remote_addr;` with `ProxyHeader = "X-Real-IP"`
- Cloudflare: `ProxyHeader = "CF-Connecting-IP"`, trusting only Cloudflare's published ranges

Whatever header you choose, trust only the addresses of the proxies that set it.

## Where this matters

[Rate Limiter](/middleware/rate-limiter/)'s per-IP mode and the [RealIP](/middleware/utility/#realip) middleware both use `c.IP()`. Configure proxies before relying on either.

Trust entries are validated and copied when the app is created. Invalid IP addresses or CIDRs panic.

`Scheme()` accepts only `http` or `https` from the last `X-Forwarded-Proto` value supplied by a trusted direct peer. Direct TLS always returns `https`. Proxies should overwrite this header with the protocol they have verified.
