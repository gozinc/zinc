---
title: Configuration
description: Reference for zinc.Config and the default constants.
---

`zinc.Config` holds every application setting. Pass it to `zinc.New`; every field you leave out keeps its default.

```go
app := zinc.New(zinc.Config{
	BodyLimit: 16 << 20, // change what you need
})
```

## Fields

| Field | Type | Zero value means | Purpose |
|---|---|---|---|
| `CaseSensitive` | `bool` | off | Match literal route segments case-sensitively |
| `StrictRouting` | `bool` | off | Treat `/users` and `/users/` as different routes |
| `DisableAutoHead` | `bool` | automatic `HEAD` on | Stop serving `HEAD` from the matching `GET` route |
| `DisableAutoOptions` | `bool` | automatic `OPTIONS` on | Stop answering `OPTIONS` with `204` and `Allow` |
| `DisableMethodNotAllowed` | `bool` | `405` on | Answer method mismatches with `404` instead of `405` and `Allow` |
| `RouteCacheSize` | `int` | `1000` | Cached dynamic paths; `-1` disables |
| `BodyLimit` | `int64` | `4 << 20` | Maximum body size read by binding and forms; `-1` for no limit |
| `ReadTimeout` | `time.Duration` | `5s` | Server read timeout; `-1` for none |
| `WriteTimeout` | `time.Duration` | `10s` | Server write timeout, per event for SSE; `-1` for none |
| `IdleTimeout` | `time.Duration` | `120s` | Keep-alive idle timeout; `-1` for none |
| `ShutdownTimeout` | `time.Duration` | `10s` | How long `ListenContext` drains requests; `-1` waits indefinitely |
| `ServerHeader` | `string` | none | `Server` response header, when set |
| `ProxyHeader` | `string` | `"X-Forwarded-For"` | Header read by `c.IP()` |
| `TrustedProxies` | `[]string` | none | IPs and CIDR ranges allowed to set `ProxyHeader` |
| `ErrorHandler` | `ErrorHandler` | `DefaultErrorHandler` (JSON) | Turns returned errors into responses; `zinc.TextErrors` sends plain text |
| `Validator` | `Validator` | none | Runs after every bind |
| `Renderer` | `Renderer` | none | Renders templates for `c.Render` |
| `JSONCodec` | `JSONCodec` | `encoding/json` | Encodes and decodes JSON |
| `RequestBinder` | `RequestBinder` | built in | Decodes requests for `c.Bind()` |

For limits and timeouts, `0` selects the default and a negative value turns the limit off. Switches that are on by default are named `Disable…`, so an omitted field never turns a feature off.

## Constants

| Constant | Value |
|---|---|
| `DefaultBodyLimit` | `4 << 20` |
| `DefaultReadTimeout` | `5 * time.Second` |
| `DefaultWriteTimeout` | `10 * time.Second` |
| `DefaultIdleTimeout` | `120 * time.Second` |
| `DefaultShutdownTimeout` | `10 * time.Second` |
| `DefaultRouteCacheSize` | `1000` |
| `DefaultProxyHeader` | `"X-Forwarded-For"` |

## Related

- [Configuration guide](/guide/configuration/) explains each setting in context.
- [Customization](/guide/customization/) shows each extension point in use.
