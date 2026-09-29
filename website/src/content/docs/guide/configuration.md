---
title: Configuration
description: Every Zinc setting, its default, and how to change it.
---

Configuration lets you change Zinc's defaults: body size, server timeouts, routing rules, proxies and more. The defaults suit most apps, so you usually change two or three settings, if any.

Pass a `zinc.Config` with only the fields you want to change. Every field you leave out keeps its default:

```go
app := zinc.New(zinc.Config{
	BodyLimit:    16 << 20, // 16 MiB
	ServerHeader: "zinc",
})
```

```bash
curl -i http://localhost:8080/
# HTTP/1.1 200 OK
# Server: zinc
# ...
```

:::note[Zero means default]
For limits and timeouts, `0` means the default and a negative value, such as `-1`, turns the limit off. Settings that are on by default are named `Disable…`, so leaving them out keeps them on.
:::

## Find the setting you need

| I want to… | Set |
|---|---|
| Accept larger uploads | [`BodyLimit`](#requests), and maybe `ReadTimeout` |
| Keep a slow stream open longer | [`WriteTimeout`](#server) |
| Read the client's IP behind a load balancer | [`TrustedProxies`](#proxies) |
| Treat `/Users` and `/users` as different routes | [`CaseSensitive`](#routing) |
| Give every cookie a `SameSite` mode | [`CookieSameSite`](#cookies) |
| Name the API in its spec, or stop serving the spec | [`OpenAPI`, `OpenAPIPath`](#openapi) |
| Change the error format, validator, templates or body formats | [`ErrorHandler`, `Validator`, `Renderer`, `Decoders`, `Encoders`](#replace-a-built-in-default) |

## Routing

| Field | Default | Effect |
|---|---|---|
| `CaseSensitive` | `false` | When `true`, `/Users` and `/users` are different routes. |
| `StrictRouting` | `false` | When `true`, `/users` and `/users/` are different routes. |
| `DisableAutoHead` | `false` | `HEAD` requests run the matching `GET` route and send no body. Set `true` to turn this off. |
| `DisableAutoOptions` | `false` | `OPTIONS` requests get `204` and an `Allow` header. Set `true` to turn this off. |
| `DisableMethodNotAllowed` | `false` | A path that exists for other methods gets `405` with `Allow`. Set `true` to send `404` instead. |
| `RouteCacheSize` | ignored | Does nothing. See the note below. |

[Routing](/guide/routing/#which-route-wins) explains why case and trailing slashes don't matter by default.

:::note[RouteCacheSize]
Zinc 0.5 removed the route cache, because matching without it is faster on typical traffic. `RouteCacheSize` still compiles, so upgrading needs no change. You can delete it from your config whenever it suits you.
:::

## Server

These apply when Zinc starts the server for you, with `Listen`, `ListenContext`, `ListenTLS` or `Serve`.

| Field | Default | Effect |
|---|---|---|
| `ReadTimeout` | `5s` | Longest time to read the whole request, body included. `-1` for none. |
| `WriteTimeout` | `10s` | Longest time to write a response, or one event of a server-sent event stream. `-1` for none. |
| `IdleTimeout` | `120s` | How long a keep-alive connection stays open between requests. `-1` for none. |
| `ShutdownTimeout` | `10s` | How long `ListenContext` waits for running requests when its context ends. `-1` waits until they finish. |
| `ServerHeader` | `""` | When set, sent as the `Server` header on every response. |

Long uploads and slow streaming responses may need a longer `ReadTimeout` or `WriteTimeout`.

:::caution[Your own http.Server]
If you pass the app to your own `http.Server`, these timeouts don't apply. Set them on the server instead. [Zinc and net/http](/guide/http-interoperability/#run-zinc-on-your-own-server) shows how.
:::

To stop the server cleanly on `Ctrl+C` or `SIGTERM`, use `ListenContext` with `signal.NotifyContext`. The [Graceful Shutdown](/cookbook/graceful-shutdown/) recipe has a complete program.

## Requests

| Field | Default | Effect |
|---|---|---|
| `BodyLimit` | `4 MiB` (`4 << 20`) | Largest body Zinc reads when binding, parsing a form or calling `c.BodyBytes`. Larger bodies get `413`. `-1` for no limit. |

```go
app := zinc.New(zinc.Config{BodyLimit: 1 << 20}) // 1 MiB
```

A route that binds the body then answers a bigger request with `413`:

```bash
curl -i -X POST http://localhost:8080/upload \
  -H 'Content-Type: application/json' --data-binary @big.json
# HTTP/1.1 413 Request Entity Too Large
# {"error":{"status":413,"message":"Request Entity Too Large"}}
```

## Cookies

| Field | Default | Effect |
|---|---|---|
| `CookieSameSite` | not set | `SameSite` mode for cookies written with `c.SetCookie` and `c.ClearCookie` that don't set one themselves, for example `http.SameSiteLaxMode`. |

```go
app := zinc.New(zinc.Config{CookieSameSite: http.SameSiteLaxMode})

app.Get("/login", func(c *zinc.Context) error {
	c.SetCookie(&http.Cookie{Name: "session", Value: "abc"})
	return c.NoContent()
})
```

```bash
curl -i http://localhost:8080/login
# HTTP/1.1 204 No Content
# Set-Cookie: session=abc; SameSite=Lax
```

## Proxies

| Field | Default | Effect |
|---|---|---|
| `ProxyHeader` | `X-Forwarded-For` | Header that `c.IP()` reads for the client address. |
| `TrustedProxies` | none | IP addresses or CIDRs allowed to set that header. With none, forwarding headers are ignored. |

[Client IP and Proxies](/guide/ip-address/) explains how to set these safely.

## OpenAPI

Every app serves an [OpenAPI](/guide/openapi/) spec of its routes.

| Field | Default | Effect |
|---|---|---|
| `OpenAPIPath` | `"/openapi.json"` | Where the spec is served, for `GET` and `HEAD`. `"-"` serves none. |
| `OpenAPI` | Your module's name and version | The spec's title, version, security schemes and more: an `OpenAPIConfig`. |

The spec lists every route that isn't hidden. For a private API, protect it with middleware or set `OpenAPIPath: "-"`.

## Replace a built-in default

Each of these fields swaps one part of Zinc's behavior for your own:

| Field | Replaces |
|---|---|
| `ErrorHandler` | How returned errors become responses (`zinc.DefaultErrorHandler`) |
| `Validator` | Validation after every bind (none by default) |
| `Renderer` | Template rendering for `c.Render` (none by default) |
| `Decoders` | Request body formats beyond JSON, XML and forms, or a different JSON library |
| `Encoders` | Response formats for `c.Encode` and `c.Negotiate`, or a different JSON library |

[Customization](/guide/customization/) shows each one in use.

## Good to know

### Settings are read once

`zinc.New` reads the config when it creates the app. Changing your `Config` value, or a slice or map inside it, afterwards has no effect. An invalid `TrustedProxies` entry panics in `zinc.New`, so a typo stops the app at startup.

### Defaults as constants

The defaults are exported for code that needs them: `zinc.DefaultBodyLimit`, `DefaultReadTimeout`, `DefaultWriteTimeout`, `DefaultIdleTimeout`, `DefaultShutdownTimeout` and `DefaultProxyHeader`. `DefaultRouteCacheSize` remains, deprecated, for code that names it.

### The body limit covers Zinc's readers

`BodyLimit` applies when Zinc reads the body for you. If you read `c.Request().Body` directly, wrap it in `http.MaxBytesReader` yourself.

## Next steps

- [Customization](/guide/customization/): plug in your own error handler, validator, templates or body formats.
- [Client IP and Proxies](/guide/ip-address/): set `TrustedProxies` for your load balancer.
- [Config API](/api/config/): the type definition.
