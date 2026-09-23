---
title: Configuration
description: Every Zinc setting, its default, and the safe way to change it.
---

`zinc.New()` uses `zinc.DefaultConfig`, which suits most applications. To change a setting, copy the defaults, adjust what you need, and build the app with `NewWithConfig`.

```go
cfg := zinc.DefaultConfig
cfg.BodyLimit = 16 << 20 // 16 MB
cfg.TrustedProxies = []string{"10.0.0.0/8"}

app := zinc.NewWithConfig(cfg)
```

:::caution[Always start from DefaultConfig]
Several defaults are `true`. A bare `zinc.Config{BodyLimit: 16 << 20}` literal switches off automatic `HEAD` and `OPTIONS`, 405 responses, and the route cache, because Go fills omitted booleans with `false` and numbers with `0`. `NewWithConfig` restores defaults only for limits, timeouts, the proxy header, and the codec, binder, and error handler.
:::

## Routing

| Field | Default | Effect |
|---|---|---|
| `CaseSensitive` | `false` | When `true`, `/Users` and `/users` are different routes. |
| `StrictRouting` | `false` | When `true`, `/users` and `/users/` are different routes. |
| `AutoHead` | `true` | Answers `HEAD` with the matching `GET` route, without a body. |
| `AutoOptions` | `true` | Answers `OPTIONS` with `204` and an `Allow` header. |
| `HandleMethodNotAllowed` | `true` | Returns `405` with `Allow` when the path exists for other methods. When `false`, returns `404`. |
| `RouteCacheSize` | `1000` | Number of concrete dynamic paths cached for faster matching. `0` disables the cache. |

## Server

These apply when Zinc starts the server with `Listen`, `ListenTLS`, or `Serve`. When you run your own `http.Server`, set its fields directly instead.

| Field | Default | Effect |
|---|---|---|
| `ReadTimeout` | `5s` | Maximum time to read the whole request, including the body. |
| `WriteTimeout` | `10s` | Maximum time to write the response. |
| `IdleTimeout` | `120s` | How long keep-alive connections stay open between requests. |
| `ServerHeader` | `""` | When set, sent as the `Server` header on every response. |

Streaming responses and long uploads may need a longer `ReadTimeout` or `WriteTimeout`.

## Requests

| Field | Default | Effect |
|---|---|---|
| `BodyLimit` | `4 MB` | Largest body that binding reads. Larger bodies get `413`. |

## Proxies

| Field | Default | Effect |
|---|---|---|
| `ProxyHeader` | `X-Forwarded-For` | Header that `c.IP()` reads for the client address. |
| `TrustedProxies` | none | Peers allowed to set that header. With none, forwarding headers are ignored. |

[Client IP and Proxies](/guide/ip-address/) explains how to set these safely.

## Extension points

| Field | Replaces |
|---|---|
| `ErrorHandler` | How returned errors become responses |
| `Validator` | Validation after every bind (none by default) |
| `Renderer` | Template rendering for `c.Render` (none by default) |
| `JSONCodec` | JSON encoding and decoding |
| `RequestBinder` | Request decoding for `c.Bind()` |

[Customization](/guide/customization/) shows each one in use.

## Next steps

- [Customization](/guide/customization/) for plugging in your own validator, codec, and error handler.
- [Config API](/api/config/) for the type definition.
