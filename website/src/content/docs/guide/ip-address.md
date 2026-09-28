---
title: Client IP and Proxies
description: Read the client's IP address correctly, both directly and behind load balancers and reverse proxies you trust.
---

Behind a load balancer, every request appears to come from the load balancer. The client's own address travels in a forwarding header such as `X-Forwarded-For`, but any client can send that header too. Zinc reads it only when the request comes from a proxy you have said you trust.

## Read the client's address

```go
app.Get("/whoami", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"client": c.IP(),
		"chain":  c.IPs(),
		"peer":   c.RemoteIP(),
	})
})
```

With no trusted proxies, the default, Zinc ignores the forwarding header. `c.IP()` and `c.RemoteIP()` return the same value:

```bash
# Request from the load balancer at 10.0.0.5
curl -H "X-Forwarded-For: 203.0.113.9" http://localhost:8080/whoami
# {"chain":["10.0.0.5"],"client":"10.0.0.5","peer":"10.0.0.5"}
```

| Helper | Returns | Reads the forwarding header? |
|---|---|---|
| `c.RemoteIP()` | The address that opened the connection, from `Request.RemoteAddr` | Never |
| `c.IP()` | The first address in the header, reading from the right, that isn't one of your proxies | Only when the request comes from a trusted proxy |
| `c.IPs()` | That address, then the trusted proxies after it | Only when the request comes from a trusted proxy |

## Trust your proxies

List the addresses or CIDR ranges of the proxies in front of your app:

```go
app := zinc.New(zinc.Config{
	ProxyHeader: zinc.HeaderXForwardedFor, // the default
	TrustedProxies: []string{
		"10.0.0.0/8", // internal load balancers
		"192.168.0.0/16",
	},
})
```

Now the same request returns the client's address:

```bash
# Request from the load balancer at 10.0.0.5
curl -H "X-Forwarded-For: 198.51.100.4" http://localhost:8080/whoami
# {"chain":["198.51.100.4"],"client":"198.51.100.4","peer":"10.0.0.5"}
```

A request that doesn't come from a trusted address still gets its own peer address, whatever header it sends:

```bash
# Request straight from 203.0.113.50
curl -H "X-Forwarded-For: 1.2.3.4" http://localhost:8080/whoami
# {"chain":["203.0.113.50"],"client":"203.0.113.50","peer":"203.0.113.50"}
```

:::danger[Only trust proxies you control]
Trusting forwarding headers from an address you do not operate lets any client choose its own IP. That breaks rate limits, audit logs, and IP allow lists without any error.
:::

## How Zinc reads the chain

Each proxy appends the address it received the request from, so the rightmost entries are the ones your own proxies wrote. Zinc walks the header from right to left, skipping addresses you trust, and returns the first one you don't. Anything to the left of that could have been written by the client, so Zinc ignores it.

A client that sends `X-Forwarded-For: 203.0.113.9` through a trusted load balancer arrives as `203.0.113.9, 198.51.100.4`. Zinc returns `198.51.100.4`, the address the load balancer saw, not the value the client chose:

```bash
# The load balancer appended 198.51.100.4
curl -H "X-Forwarded-For: 203.0.113.9, 198.51.100.4" http://localhost:8080/whoami
# {"chain":["198.51.100.4"],"client":"198.51.100.4","peer":"10.0.0.5"}
```

Behind two trusted proxies, `c.IPs()` also lists the inner proxy:

```bash
curl -H "X-Forwarded-For: 198.51.100.4, 192.168.1.2" http://localhost:8080/whoami
# {"chain":["198.51.100.4","192.168.1.2"],"client":"198.51.100.4","peer":"10.0.0.5"}
```

:::caution[List every proxy you operate]
If an inner proxy is missing from `TrustedProxies`, `c.IP()` returns that proxy's address for every request. Here `172.16.0.3` isn't trusted:

```bash
curl -H "X-Forwarded-For: 198.51.100.4, 172.16.0.3" http://localhost:8080/whoami
# {"chain":["172.16.0.3"],"client":"172.16.0.3","peer":"10.0.0.5"}
```
:::

## Use a single-value header

Proxies that set a single-value header also work. Set `ProxyHeader` to its name:

- nginx: `proxy_set_header X-Real-IP $remote_addr;` with `ProxyHeader: "X-Real-IP"`
- Cloudflare: `ProxyHeader: "CF-Connecting-IP"`, trusting only Cloudflare's published ranges

```bash
# nginx at 10.0.0.5, with ProxyHeader: "X-Real-IP"
curl -H "X-Real-IP: 198.51.100.4" http://localhost:8080/whoami
# {"chain":["198.51.100.4"],"client":"198.51.100.4","peer":"10.0.0.5"}
```

Whatever header you choose, trust only the addresses of the proxies that set it.

## HTTPS behind a proxy

When a proxy ends TLS, your app sees plain HTTP. `c.Scheme()` and `c.Secure()` read `X-Forwarded-Proto` to tell you what the client used, but only when the request comes from a trusted proxy:

```go
app.Get("/scheme", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"scheme": c.Scheme(), "secure": c.Secure()})
})
```

```bash
# From the trusted load balancer at 10.0.0.5
curl -H "X-Forwarded-Proto: https" http://localhost:8080/scheme
# {"scheme":"https","secure":true}

# Straight from 203.0.113.50
curl -H "X-Forwarded-Proto: https" http://localhost:8080/scheme
# {"scheme":"http","secure":false}
```

Zinc uses the last value in the header and accepts only `http` or `https`; anything else gives `http`. A request that arrives over TLS directly is always `https`. Configure your proxies to overwrite this header with the protocol they have checked, not append to it.

## Where this matters

A per-IP [rate limiter](/middleware/limiter/) keys on `c.IP()`, and the [logger](/middleware/logger/) records it as `remote_ip`. Configure your proxies before relying on either:

```go
app.Use(limiter.New(limiter.Config{
	Key: func(c *zinc.Context) string { return c.IP() },
}))
```

## Good to know

### Bad entries fall back to the peer address

If Zinc hits an entry that isn't an IP address while walking your proxies' part of the header, it ignores the header and returns the peer address. Bad entries to the left of the client's address are never read.

```bash
curl -H "X-Forwarded-For: 198.51.100.4, garbage" http://localhost:8080/whoami
# {"chain":["10.0.0.5"],"client":"10.0.0.5","peer":"10.0.0.5"}
```

### Every address trusted

If every address in the header is one you trust, `c.IP()` returns the leftmost.

### Trusted proxies are checked at startup

`zinc.New` validates `TrustedProxies` and keeps its own copy, so changing the slice later has no effect. An entry that isn't an IP address or CIDR range panics:

```text
zinc: invalid trusted proxy: lb.internal
```

## Next steps

- [Configuration](/guide/configuration/): every `zinc.Config` field, including `ProxyHeader` and `TrustedProxies`.
- [Limiter](/middleware/limiter/): rate-limit requests per client.
- [Logger](/middleware/logger/): log each request with the client's address.
