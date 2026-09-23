---
title: Configuration
description: Reference for zinc.Config and zinc.DefaultConfig.
---

`zinc.Config` holds every application setting. `zinc.DefaultConfig` holds the defaults that `zinc.New()` uses.

```go
cfg := zinc.DefaultConfig // copy the defaults
cfg.BodyLimit = 16 << 20  // change what you need
app := zinc.NewWithConfig(cfg)
```

## Fields

| Field | Type | Default | Purpose |
|---|---|---|---|
| `CaseSensitive` | `bool` | `false` | Match literal route segments case-sensitively |
| `StrictRouting` | `bool` | `false` | Treat `/users` and `/users/` as different routes |
| `AutoHead` | `bool` | `true` | Serve `HEAD` from the matching `GET` route |
| `AutoOptions` | `bool` | `true` | Answer `OPTIONS` with `204` and `Allow` |
| `HandleMethodNotAllowed` | `bool` | `true` | Answer `405` with `Allow` instead of `404` |
| `RouteCacheSize` | `int` | `1000` | Cached dynamic paths; `0` disables |
| `BodyLimit` | `int64` | `4 << 20` | Maximum body size read by binding |
| `ReadTimeout` | `time.Duration` | `5s` | Server read timeout |
| `WriteTimeout` | `time.Duration` | `10s` | Server write timeout |
| `IdleTimeout` | `time.Duration` | `120s` | Keep-alive idle timeout |
| `ServerHeader` | `string` | `""` | `Server` response header, when set |
| `ProxyHeader` | `string` | `"X-Forwarded-For"` | Header read by `c.IP()` |
| `TrustedProxies` | `[]string` | `nil` | IPs and CIDR ranges allowed to set `ProxyHeader` |
| `ErrorHandler` | `ErrorHandler` | plain text | Turns returned errors into responses |
| `Validator` | `Validator` | `nil` | Runs after every bind |
| `Renderer` | `Renderer` | `nil` | Renders templates for `c.Render` |
| `JSONCodec` | `JSONCodec` | `encoding/json` | Encodes and decodes JSON |
| `RequestBinder` | `RequestBinder` | built in | Decodes requests for `c.Bind()` |

## How defaults are applied

`NewWithConfig` fills these fields from `DefaultConfig` when they are zero: `BodyLimit`, the three timeouts, `ProxyHeader`, `JSONCodec`, `RequestBinder`, and `ErrorHandler`.

It cannot tell an unset boolean from `false`, so it leaves `AutoHead`, `AutoOptions`, `HandleMethodNotAllowed`, and `RouteCacheSize` exactly as given. Start from a copy of `DefaultConfig`, not a bare `zinc.Config{}` literal, to keep them on.

## Related

- [Configuration guide](/guide/configuration/) explains each setting in context.
- [Customization](/guide/customization/) shows each extension point in use.
