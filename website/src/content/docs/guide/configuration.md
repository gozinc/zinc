---
title: Configuration
description: Every Zinc setting, its default, and how to change it.
---

`zinc.New()` uses defaults that suit most applications. To change a setting, pass a `zinc.Config` with only the fields you need. Every field you leave out keeps its default:

```go
app := zinc.New(zinc.Config{
	BodyLimit:      16 << 20, // 16 MB
	TrustedProxies: []string{"10.0.0.0/8"},
})
```

For limits and timeouts, `0` means the default and a negative value turns the limit off. Switches that are on by default are named `Disable…`, so leaving them out keeps them on.

## Routing

| Field | Default | Effect |
|---|---|---|
| `CaseSensitive` | `false` | When `true`, `/Users` and `/users` are different routes. |
| `StrictRouting` | `false` | When `true`, `/users` and `/users/` are different routes. |
| `DisableAutoHead` | `false` | Automatic `HEAD` is on: `HEAD` is answered by the matching `GET` route, without a body. Set `true` to turn it off. |
| `DisableAutoOptions` | `false` | Automatic `OPTIONS` is on: it is answered with `204` and an `Allow` header. Set `true` to turn it off. |
| `DisableMethodNotAllowed` | `false` | When the path exists for other methods, Zinc returns `405` with `Allow`. Set `true` to return `404` instead. |
| `RouteCacheSize` | `1000` | Number of concrete dynamic paths cached for faster matching. `-1` disables the cache. |

## Server

These apply when Zinc starts the server with `Listen`, `ListenContext`, `ListenTLS`, or `Serve`. When you run your own `http.Server`, set its fields directly instead.

| Field | Default | Effect |
|---|---|---|
| `ReadTimeout` | `5s` | Maximum time to read the whole request, including the body. `-1` for none. |
| `WriteTimeout` | `10s` | Maximum time to write a response, or one event of a server-sent event stream. `-1` for none. |
| `IdleTimeout` | `120s` | How long keep-alive connections stay open between requests. `-1` for none. |
| `ShutdownTimeout` | `10s` | How long `ListenContext` waits for in-flight requests when its context ends. `-1` waits until they finish. |
| `ServerHeader` | `""` | When set, sent as the `Server` header on every response. |

Long uploads and slow streaming responses may need a longer `ReadTimeout` or `WriteTimeout`.

## Graceful shutdown

`ListenContext` serves until its context ends, then stops accepting connections and lets in-flight requests finish:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

if err := app.ListenContext(ctx, ":8080"); err != nil {
	log.Fatal(err)
}
```

It returns `nil` after a clean shutdown. If requests are still running when `ShutdownTimeout` expires, their connections are closed and `ListenContext` returns an error. Zinc never installs signal handlers itself. See [Graceful Shutdown](/cookbook/graceful-shutdown/).

## Requests

| Field | Default | Effect |
|---|---|---|
| `BodyLimit` | `4 MB` | Largest body that binding and form parsing read. Larger bodies get `413`. `-1` for no limit. |

## Proxies

| Field | Default | Effect |
|---|---|---|
| `ProxyHeader` | `X-Forwarded-For` | Header that `c.IP()` reads for the client address. |
| `TrustedProxies` | none | Peers allowed to set that header. With none, forwarding headers are ignored. |

[Client IP and Proxies](/guide/ip-address/) explains how to set these safely.

## Extension points

| Field | Replaces |
|---|---|
| `ErrorHandler` | How returned errors become responses (`zinc.DefaultErrorHandler`) |
| `Validator` | Validation after every bind (none by default) |
| `Renderer` | Template rendering for `c.Render` (none by default) |
| `Decoders` | Request body formats beyond JSON, XML, and forms, or a different JSON library |
| `Encoders` | Response formats for `c.Encode` and `c.Negotiate`, or a different JSON library |

[Customization](/guide/customization/) shows each one in use.

## Defaults as constants

The defaults are exported for code that needs them: `zinc.DefaultBodyLimit`, `DefaultReadTimeout`, `DefaultWriteTimeout`, `DefaultIdleTimeout`, `DefaultShutdownTimeout`, `DefaultRouteCacheSize`, and `DefaultProxyHeader`.

## Next steps

- [Customization](/guide/customization/) for plugging in your own validator, codec, and error handler.
- [Config API](/api/config/) for the type definition.
