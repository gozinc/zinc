---
title: Migrating to Zinc 0.3
description: Upgrade a Zinc 0.2 application to 0.3, a hardening release whose breaking changes are mostly behavioural.
slug: extra/migration-0.3
---

Zinc 0.3 is a hardening release. Very few signatures change, so most 0.2 applications compile without edits. The upgrade work is in behaviour: some unsafe configurations now panic at startup, and several defaults are stricter at runtime. Work through the checklist, then read the sections that apply to you.

## Checklist

| If your app uses | What changes | Section |
|---|---|---|
| Session middleware | Existing cookies are invalidated, secrets need 32 bytes, and `Set` and `Delete` return errors | [Sessions](#sessions) |
| CORS with credentials | `*` combined with credentials panics at startup | [CORS](#cors) |
| `TrustedProxies` | `c.IP()` reads forwarded chains from the right, and invalid entries panic | [Client IP and proxies](#client-ip-and-proxies) |
| Prometheus middleware | Each middleware has its own registry, not a shared global one | [Prometheus](#prometheus) |
| Rate Limiter | Keyed limiters are bounded to 10,000 keys | [Rate Limiter](#rate-limiter) |
| Uploads or forms over 4 MiB | `BodyLimit` now covers forms and multipart | [Request bodies](#request-bodies) |
| Clients that read error text | Binding failures return a generic `400` | [Errors and binding](#errors-and-binding) |
| Type assertions on `c.Writer()` | The writer is always Zinc's wrapper | [Responses](#responses) |
| `Static`, `File`, or `Mount` on groups | They now run the group's middleware | [Groups and static files](#groups-and-static-files) |
| Reverse proxy retries | Only idempotent requests are retried by default | [Reverse proxy](#reverse-proxy) |

## Upgrade

```sh
go get github.com/0mjs/zinc@v0.3.0
go build ./... && go test ./...
```

A clean build is not enough. Start the app once, because the checks that replaced silent misconfiguration run at construction. Then exercise sign-in, uploads, and your metrics endpoint before deploying.

## Sessions

**Every existing session cookie is invalidated.** The 0.3 cookie format carries a version and an authenticated expiry, so cookies written by 0.2 no longer verify. Users must sign in again. Plan the deploy for a quiet period, or warn users first. `PreviousSecrets` rotates keys within the new format; it does not keep 0.2 cookies alive.

**Secrets need at least 32 random bytes.** Shorter `Secret` or `PreviousSecrets` values panic at construction.

```sh
openssl rand -base64 48
```

**`Set` and `Delete` return errors, and they write the cookie immediately.** In 0.2 the response was buffered so changes could be made at any point. In 0.3 the cookie header is written as soon as you call them, so call them before writing or flushing the body, and check the error:

```go
// 0.2
session.Set("user_id", user.ID)
return c.JSON(user)

// 0.3
if err := session.Set("user_id", user.ID); err != nil {
	return err
}
return c.JSON(user)
```

Changes after the response has started return `ErrSessionCommitted`. Cookies over 4096 bytes return `ErrSessionTooLarge`. A failed change leaves the previous values in place.

**Expiry is enforced by the server.** A positive `MaxAge` sets the lifetime. Browser-session cookies (`MaxAge: 0`) now expire after `Lifetime`, which defaults to 24 hours.

## CORS

In 0.2, `AllowOrigins: ["*"]` with `AllowCredentials: true` reflected whatever origin the request named, which let any website make credentialed requests. In 0.3 that combination **panics at construction**. List the origins you trust:

```go
app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
	AllowOrigins:     []string{"https://app.example.com"},
	AllowCredentials: true,
}))
```

With credentials enabled, an empty origin list now denies all cross-origin access. A negative `MaxAge` also panics.

## Client IP and proxies

`TrustedProxies` entries are validated and copied when the app is created. An invalid IP address or CIDR panics.

`c.IP()` now walks the forwarding header **from right to left** and stops at the first address you do not trust. In 0.2 it returned the leftmost value, which any client could set. If traffic passes through several proxies, the IP you log, rate-limit, and allow-list by may change. Confirm that every proxy you operate is listed. See [Client IP and Proxies](/guide/ip-address/).

`c.Scheme()` accepts only `http` or `https`, taken from the last `X-Forwarded-Proto` value sent by a trusted peer.

## Prometheus

In 0.2, every `Prometheus()` middleware and `PrometheusHandler()` shared one package-level registry. In 0.3, each middleware construction owns an isolated registry.

If your `/metrics` route runs behind the same `Prometheus()` middleware, nothing changes. Otherwise, the handler finds no registry and returns `503`. Share one registry explicitly:

```go
metrics := middleware.NewPrometheusMetrics()

app.Use(middleware.Prometheus(metrics))
app.Get("/metrics", middleware.PrometheusHandler(metrics))
```

Labels are bounded as well. Unmatched requests use the route label `unmatched` instead of the raw path, and nonstandard methods use `OTHER`. Update dashboards and alerts that filter on raw paths. Registries hold at most 10,000 series by default; the overflow is counted in `zinc_http_metrics_dropped_total`, and `NewPrometheusMetrics(maxSeries)` changes the cap.

## Rate Limiter

Keyed limiters, such as per-IP or per-API-key limiters, now keep at most `MaxKeys` buckets (default 10,000) with keys of up to `MaxKeyBytes` (default 256). When the store is full, **new keys receive the limit response** rather than a fresh quota. Idle, fully refilled buckets expire after `IdleTTL` (default five minutes). Size `MaxKeys` for your client population:

```go
app.Use(middleware.RateLimiter(middleware.RateLimiterConfig{
	Rate:     10,
	Capacity: 20,
	IPLookup: func(c *zinc.Context) string { return c.IP() },
	MaxKeys:  100_000,
}))
```

A zero `Rate` or `Capacity` now uses the defaults of 10 tokens a second and a burst of 10. Negative or non-finite values panic. The default rejection handler honours `StatusCode`.

## Request bodies

`Config.BodyLimit`, 4 MiB by default, now applies to URL-encoded forms, multipart forms, and file uploads as well as JSON, XML, YAML, TOML, and raw bodies. Larger requests get `413 Request Entity Too Large`. If you accept big uploads, raise the application budget, and use the [Body Limit](/middleware/body-limit/) middleware to keep other routes lower:

```go
cfg := zinc.DefaultConfig
cfg.BodyLimit = 64 << 20 // 64 MiB for the upload routes
app := zinc.NewWithConfig(cfg)
```

Multipart temporary files belong to the request and are removed when it ends, so save uploads inside the handler. Form-value helpers that cannot return an error yield empty values when the body is over budget; use binding when you need to handle that error.

[Decompress](/middleware/decompress/) now limits expanded bodies to the application budget, or 4 MiB when none is set. A positive `MaxDecompressedSize` overrides that.

## Errors and binding

The default error handler now answers binding failures with a generic `400 Bad Request` and keeps HTTP causes such as `413`. Detailed decode messages no longer reach clients. If clients depend on that detail, return your own `HTTPError` from the handler or set `Config.ErrorHandler`.

Malformed JSON is classified as a client error. Invalid destination types and opaque codec failures remain `500`.

Error modifiers such as `WithMessage` return copies, so compare errors by status rather than by pointer:

```go
var he *zinc.HTTPError
if errors.As(err, &he) && he.Code == http.StatusNotFound {
	// ...
}
```

`Bind().All` now merges path and query values into YAML and TOML struct targets, as it already did for JSON and XML. Path, query, header, and form fields also accept `encoding.TextUnmarshaler` types and optional scalar pointers.

## Responses

`c.Writer()` is always Zinc's instrumented writer, including after `SetWriter`, so it is no longer the server's original writer. Asserting it to your own writer type fails. Use `http.ResponseController`, or call `Unwrap()`:

```go
rc := http.NewResponseController(c.Writer())
rc.Flush()
```

Choosing a status no longer commits the response; writing, flushing, or hijacking does. An error returned before the response is committed can still produce an error response, and the error handler runs at most once per request.

## Groups and static files

`Static`, `StaticFS`, `File`, `FileFS`, and `Mount` registered on a group now run that group's middleware, including its parents'. Public assets registered on an authenticated group now require authentication. Move them to the app or to a public group.

Global middleware runs before prefix middleware, and each prefix chain runs at most once per request.

Static paths that contain backslashes, dot segments, or repeated separators are rejected instead of normalised. Symlinks cannot lead outside the root. Zinc keeps one directory handle per `Static` root after the first request, so renaming the directory does not take effect until the app is recreated. If you serve the app from your own `http.Server`, call `app.Close()` after that server stops. See [Static Files](/guide/static-files/).

## Routing

- Case-insensitive static routes win over parameter routes, whatever the request's spelling.
- Mounted handlers see the stripped path in both `URL.Path` and `RequestURI`. Outer middleware still sees the original request.
- `app.URL` rejects empty values and values containing `/` for single-segment parameters. It escapes `?`, `#`, and `%` in catch-all values, so the generated URL routes back to the same value.

## Reverse proxy

The proxy's `Director` runs after forwarding headers are set and hop-by-hop headers are removed. Retries default to idempotent methods with replayable bodies; set `RetryFilter` to choose your own policy.

## Smaller changes

- Informational (`1xx`) headers no longer use up the final status.
- Gzip sends small flushed responses uncompressed and keeps that encoding for later writes. `Vary: Accept-Encoding` is set on uncompressed responses too.
- Recover re-panics `http.ErrAbortHandler` instead of turning it into a `500`.
- Accept negotiation honours explicit `q=0` exclusions before wildcards.
- Grouped routes keep their trailing slash when `StrictRouting` is on.
- Unicode case folding keeps the original bytes of captured parameters.
- Explicit XML binding returns the same `BindError` context as other sources.
- Request-owned values are cleared as soon as a context is released, including after a panic. Copy anything you need before starting background work.
- Response header values belong to each response. This adds one small allocation to common string responses.

## New in 0.3

- `app.Close()` releases static roots when you run your own server.
- `c.BodyLimit()` reports the application's body budget to middleware that transforms request bodies.
- Session `PreviousSecrets`, `Lifetime`, `ErrSessionCommitted`, and `ErrSessionTooLarge`.
- Rate Limiter `MaxKeys`, `MaxKeyBytes`, `IdleTTL`, and `Now`.
- `NewPrometheusMetrics(maxSeries)`.

The [release notes](https://github.com/0mjs/zinc/releases/tag/v0.3.0) cover performance changes and the full list of fixes.
