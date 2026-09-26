---
title: Migrating to Zinc 0.4
description: Upgrade a Zinc 0.3 application to 0.4, which reshapes the public API around shorter handlers and safer defaults.
slug: extra/migration-0.4
---

:::note[In progress]
Zinc 0.4 is under development. This guide grows with each change, and it is complete when 0.4.0 is released.
:::

Zinc 0.4 simplifies the public API and fixes several defaults that could silently leave an application unprotected. Work through the checklist, then read the sections that apply to you.

## Checklist

| If your app uses | What changes | Section |
|---|---|---|
| `group.Use` after registering routes on that group | It panics at startup | [Group middleware order](#group-middleware-order) |
| Binding into structs with untagged fields | Path, query, header, and form values bind only to tagged fields | [Binding requires tags](#binding-requires-tags) |
| `errors.Is` against `zinc.Err*` values | Copies and wrapped errors now match by status | [Matching HTTP errors](#matching-http-errors) |
| `c.OriginalURL()` after a rewrite | It returns the URL as received | [Original URL](#original-url) |
| `c.SSE` with manual flushing | Each event is flushed for you, and streams outlive `WriteTimeout` | [Server-sent events](#server-sent-events) |
| Clients that read error bodies | Errors are JSON by default | [JSON error bodies](#json-error-bodies) |
| `WithMessage`, `WithCause`, `WithMeta`, `Meta` | Replaced by constructors, `Wrap`, and `WithDetail` | [Building errors](#building-errors) |
| A `Validator` | Failures answer 422 instead of 500 | [Validation errors](#validation-errors) |
| `c.Fail`, `c.AbortWithStatus`, `c.AbortWithJSON`, `c.Error` | Removed or renamed | [Context error helpers](#context-error-helpers) |
| `middleware.RateLimiter` with the default handler | It returns a 429 error instead of writing text | [JSON error bodies](#json-error-bodies) |
| Anything from `github.com/0mjs/zinc/middleware` | Each middleware is its own package with `New(config ...Config)` | [Middleware packages](#middleware-packages) |
| `Skipper` fields, `middleware.Maybe` | `zinc.Skip` wraps any middleware | [Middleware packages](#middleware-packages) |
| `middleware.JWT` | Moved to `github.com/0mjs/contrib/jwtauth` | [Middleware packages](#middleware-packages) |
| `middleware.Static`, `middleware.Jaeger`, `middleware.RealIP` | Removed | [Middleware packages](#middleware-packages) |
| `c.YAML`, `c.TOML`, `c.Bind().YAML`, `c.Bind().TOML`, YAML or TOML request bodies | Zinc no longer includes YAML or TOML; add them with `Config.Decoders` and `Config.Encoders` | [Body formats](#body-formats) |
| `Config.JSONCodec`, `Config.RequestBinder` | Removed; an `application/json` entry in `Decoders` and `Encoders` swaps the JSON library | [Body formats](#body-formats) |
| A custom JSON codec whose decode errors answered 500 | Decoder errors answer 400 unless they carry a status | [Body formats](#body-formats) |
| `zinc.DefaultConfig`, `zinc.NewWithConfig` | Removed: pass a `Config` literal to `zinc.New` | [Configuration](#configuration) |
| `AutoHead`, `AutoOptions`, `HandleMethodNotAllowed` | Renamed and inverted: `DisableAutoHead` and friends | [Configuration](#configuration) |
| `RouteCacheSize: 0` to turn the cache off | `0` now means the default; use `-1` | [Configuration](#configuration) |
| A goroutine calling `Listen` plus `Shutdown` on a signal | `ListenContext` does both | [Graceful shutdown](#graceful-shutdown) |
| `zinc.GetVersion()`, `zinc.GetVersionHeader()` | Removed: use `zinc.Version` | [Configuration](#configuration) |
| `c.GetString`, `c.GetInt`, `c.MustGet`, and the other typed getters | Replaced by `zinc.Value[T]` and `zinc.MustValue[T]` | [Request values](#request-values) |
| `c.ParamOr`, `c.QueryOr`, `c.PostForm*` | Replaced by `zinc.QueryOr`, `zinc.Form`, and `c.FormValue` | [Request values](#request-values) |
| `c.GetHeader`, `c.RequestID` | `c.Header`; the request ID comes from the middleware | [Request values](#request-values) |
| `c.Blob`, `c.JSONBlob`, `c.XMLBlob`, `c.HTMLBlob`, `c.Download` | `c.Data` with `zinc.MIME*` constants; `c.Attachment` | [Responses](#responses) |
| `c.Redirect(code, url)`, `c.Negotiate(status, offers)` | The status moves to `c.Status(...)` | [Responses](#responses) |
| `c.ClearCookie(names...)`, `c.SetSameSite` | `c.ClearCookie(*http.Cookie)`, `Config.CookieSameSite` | [Responses](#responses) |
| `zinc.SSEvent`, `zinc.NewContext`, `c.PathParams` | `zinc.Event`; the other two are internal | [Responses](#responses) |
| `app.Handle(zinc.RouteSpec{Name: ...})` | `app.Get(...).Name(...)`; `TryHandle` keeps `RouteSpec` | [Routing](#routing) |
| `Any`, `RoutesByMethod`, `RoutesByPrefix`, `NewGroup`, `RouteHandler` | Removed | [Routing](#routing) |
| `err := app.Static(...)` and the other file helpers | They no longer return an error | [Routing](#routing) |

## Group middleware order

A route, mount, static directory, or child group captures its group's middleware when it is registered. In 0.3, calling `group.Use` afterwards silently left those registrations without the new middleware, so an authentication middleware added at the end of a group protected nothing above it. In 0.4, `group.Use` **panics** once the group has any of them, and the message names the first one:

```text
zinc: Use on group "/admin" after route GET /admin/secret; register group middleware before its routes and child groups
```

Move `Use` above the group's routes, or pass the middleware to `Group`:

```go
// 0.3: compiles, but /admin/secret is public
admin := app.Group("/admin")
admin.Get("/secret", secret)
admin.Use(requireAdmin)

// 0.4
admin := app.Group("/admin", requireAdmin)
admin.Get("/secret", secret)
```

`app.Use` is unaffected. Global middleware runs on every request whenever it is registered.

## Binding requires tags

**This closes a mass-assignment hole. Check every struct you bind.** In 0.3, an exported field without a `path`, `query`, `header`, or `form` tag still bound under its lower-cased name. A struct written for a JSON body could therefore be filled from the query string, including fields hidden from JSON:

```go
type UpdateUser struct {
	Name    string `json:"name"`
	IsAdmin bool   `json:"-"` // 0.3: set by ?isadmin=true through c.Bind().All
}
```

In 0.4, each of those sources binds only fields that carry its tag. Add tags to the fields you intend to accept:

```go
type ListUsers struct {
	Page  int    `query:"page"`
	Limit int    `query:"limit"`
	Sort  string `query:"sort"`
}
```

A tag with options but no name, such as `query:",omitempty"`, still opts in under the lower-cased field name. Body binding through `json` and `xml` is unchanged; YAML and TOML moved out of Zinc, as [Body formats](#body-formats) describes.

## Matching HTTP errors

`errors.Is` now matches HTTP errors by status. Errors made by the constructors and copies made by `Wrap`, `WithDetail`, or `WithHeader` match the sentinel for their status, as do errors that wrap them:

```go
err := fmt.Errorf("load: %w", zinc.NotFound("user not found"))
errors.Is(err, zinc.ErrNotFound) // 0.3: false; 0.4: true
```

A target that carries a message also requires that message. If you compared `HTTPError.Code` with `errors.As` only because `errors.Is` did not work, you can simplify that code.

## Original URL

`c.OriginalURL()` now returns the request URI as received, before `SetPath`, the Rewrite middleware, or Trailing Slash changed it, as the documentation always described. In 0.3 it returned the rewritten path. Use `c.Request().URL` for the current target.

## Server-sent events

`c.SSE` now flushes each event as it writes it, so events reach the client immediately. `Config.WriteTimeout` now applies to each event instead of to the whole response. In 0.3, a stream served through `Listen` was cut off when the write timeout expired, which is 10 seconds by default.

Manual flushing after `c.SSE` still works, but you can remove it:

```go
if err := c.SSE(zinc.Event{Data: msg}); err != nil {
	return err
}
// no longer needed:
// if f, ok := c.Writer().(http.Flusher); ok { f.Flush() }
```

## JSON error bodies

The default error handler now writes JSON instead of plain text. That includes the router's 404 and 405 responses, static-file misses, and errors from built-in middleware:

```text
0.3: HTTP/1.1 404 Not Found
     Content-Type: text/plain; charset=utf-8

     user not found

0.4: HTTP/1.1 404 Not Found
     Content-Type: application/json; charset=utf-8

     {"error":{"status":404,"message":"user not found"}}
```

Binding failures add a `fields` object naming what was wrong, and `WithDetail` values appear under `details`. The rules for what reaches the client are unchanged: an unknown error sends only its status text.

If clients depend on text bodies, keep them:

```go
cfg.ErrorHandler = zinc.TextErrors
```

The rate limiter's default handler used to write `Rate limit exceeded` itself. It now returns a 429 error, so the response follows your error handler like every other error.

Two error responses changed status. **A missing file served by `c.FileFS`** was a 500 and is now a 404. **Static directories** now send their 404 and 405 responses through the error handler instead of net/http's `404 page not found` text.

## Building errors

`WithMessage`, `WithCause`, `WithMeta`, and the `Meta` field are removed:

| 0.3 | 0.4 |
|---|---|
| `zinc.ErrNotFound.WithMessage("user not found")` | `zinc.NotFound("user not found")` |
| `zinc.ErrBadRequest.WithMessage("bad cursor").WithCause(err)` | `zinc.BadRequest("bad cursor").Wrap(err)` |
| `zinc.ErrTeapot.WithMessage("no coffee")` | `zinc.NewError(zinc.StatusTeapot, "no coffee")` |
| `err.WithMeta("field", "email")` | `err.WithDetail("field", "email")`, now written to the body |
| `httpErr.Meta` | `httpErr.Details` |

Constructors exist for 400, 401, 403, 404, 409, 410, 422, 429, 500, and 503. `NewError(code, message)` covers every other status.

**Return binding errors unchanged.** `return zinc.ErrBadRequest.WithMessage("invalid body").WithCause(err)` after a failed bind still compiles as `zinc.BadRequest("invalid body").Wrap(err)`, but it now hides the field details that the default handler would send. Use `return err`.

Domain errors can choose their own status by implementing `StatusCode() int`, which removes most error-mapping code from handlers. See [Errors](/guide/errors/).

## Validation errors

In 0.3, a `Validator` failure reached the default handler as an unknown error and answered **500**. In 0.4 it is wrapped in `*zinc.ValidationError` and answers **422 Unprocessable Entity**. If the validator's error has a `Fields() map[string]string` method, the fields appear in the body. A validator that already returned an HTTP error keeps its status.

## Context error helpers

| 0.3 | 0.4 |
|---|---|
| `return c.Fail(err)` | `return err` |
| `return c.AbortWithStatus(code)` | `return zinc.NewError(code)` |
| `return c.AbortWithJSON(code, v)` | `return c.Status(code).JSON(v)` |
| `c.Error(err)` | `c.HandleError(err)` |

`c.HandleError` is for middleware that needs the final status, such as a logger. Handlers should return errors.

A custom error handler that inspects `*zinc.HTTPError` keeps working. To log failures without replacing the response format, wrap the default:

```go
cfg.ErrorHandler = func(c *zinc.Context, err error) {
	if zinc.StatusCode(err) >= 500 {
		slog.Error("request failed", "err", err)
	}
	zinc.DefaultErrorHandler(c, err)
}
```

## Configuration

**A zero-value `Config` now means the defaults.** In 0.3, a literal such as `zinc.Config{BodyLimit: 16 << 20}` quietly turned off automatic `HEAD` and `OPTIONS`, 405 responses, and the route cache, because Go fills omitted booleans with `false`. The docs asked you to copy `zinc.DefaultConfig` instead. In 0.4, pass only the fields you change:

```go
// 0.3
cfg := zinc.DefaultConfig
cfg.BodyLimit = 16 << 20
app := zinc.NewWithConfig(cfg)

// 0.4
app := zinc.New(zinc.Config{BodyLimit: 16 << 20})
```

`zinc.NewWithConfig` and the `zinc.DefaultConfig` variable are removed. The defaults are constants now: `zinc.DefaultBodyLimit`, `DefaultReadTimeout`, `DefaultWriteTimeout`, `DefaultIdleTimeout`, `DefaultShutdownTimeout`, `DefaultRouteCacheSize`, and `DefaultProxyHeader`.

The three switches that default to on are renamed and inverted, so that leaving them out keeps them on:

| 0.3 | 0.4 |
|---|---|
| `AutoHead: false` | `DisableAutoHead: true` |
| `AutoOptions: false` | `DisableAutoOptions: true` |
| `HandleMethodNotAllowed: false` | `DisableMethodNotAllowed: true` |

For limits and timeouts, `0` now always means the default, and a negative value turns the limit off. **Check any code that set `RouteCacheSize: 0` to disable the cache; it now gets the default cache. Use `-1`.** The same applies to `BodyLimit`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and the new `ShutdownTimeout`.

`zinc.GetVersion()` and `zinc.GetVersionHeader()` are removed. Use the `zinc.Version` constant.

## Graceful shutdown

`app.ListenContext(ctx, addr)` serves until `ctx` ends, then drains in-flight requests for up to `Config.ShutdownTimeout` (10 seconds by default). It replaces the goroutine, signal, and `Shutdown` wiring most programs needed:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

if err := app.ListenContext(ctx, ":8080"); err != nil {
	log.Fatal(err)
}
```

`Listen`, `Serve`, and `Shutdown` still work as before.

## Request values

Zinc 0.4 adds typed accessors. They are package functions, because Go methods cannot be generic:

```go
id, err := zinc.Param[int64](c, "id")      // 400 naming "id" if it isn't a number
since, err := zinc.Query[time.Time](c, "since")
page := zinc.QueryOr(c, "page", 1)         // T inferred from the fallback
user := zinc.MustValue[*User](c, userKey)  // replaces c.MustGet(userKey).(*User)
```

| 0.3 | 0.4 |
|---|---|
| `strconv.Atoi(c.Param("id"))` plus your own 400 | `zinc.Param[int](c, "id")` |
| `c.QueryOr("page", "1")` | `zinc.QueryOr(c, "page", "1")`, or `zinc.QueryOr(c, "page", 1)` for an `int` |
| `c.ParamOr("id", "me")` | `c.Param("id")`, falling back yourself when it is empty |
| `c.GetString(key)`, `c.GetInt(key)`, … | `v, _ := zinc.Value[string](c, key)` |
| `c.MustGet(key)` | `zinc.MustValue[T](c, key)` |
| `c.PostForm(name)` | `c.FormValue(name)` or `zinc.Form[string](c, name)` |
| `c.PostFormOr(name, fallback)` | `zinc.FormOr(c, name, fallback)` |
| `c.PostFormArray`, `c.PostFormMap` | Bind the form into a struct with `c.Bind().Form(&v)` |
| `c.GetHeader(name)` | `c.Header(name)` |
| `c.RequestID()` | `requestid.Get(c)`, or `c.Header(zinc.HeaderXRequestID)` |

`c.FormValue` and `zinc.Form` follow `http.Request.FormValue`: a body value wins, and the query string is a fallback. `c.PostForm` read only the body.

## Responses

The status argument moves out of the response helpers and into `c.Status`:

| 0.3 | 0.4 |
|---|---|
| `c.Redirect(302, "/login")` | `c.Redirect("/login")` (302 by default) |
| `c.Redirect(301, "/new")` | `c.Status(301).Redirect("/new")` |
| `c.Negotiate(200, offers)` | `c.Negotiate(offers)` |
| `c.JSONBlob(200, b)` | `c.Data(zinc.MIMEJSON, b)` |
| `c.Blob(201, ct, b)` | `c.Status(201).Data(ct, b)` |
| `c.XMLBlob`, `c.HTMLBlob` | `c.Data(zinc.MIMEXML, b)`, `c.Data(zinc.MIMEHTML, b)` |
| `c.Download(path)` | `c.Attachment(path)` |
| `c.ClearCookie("session")` | `c.ClearCookie(&http.Cookie{Name: "session"})` |
| `c.SetSameSite(mode)` | `zinc.Config{CookieSameSite: mode}` |
| `zinc.SSEvent{...}` | `zinc.Event{...}` |

`ClearCookie` now keeps the path and domain you pass, so it can remove a cookie set with `Path: "/admin"` or a domain. In 0.3 it always cleared at `/` without a domain, and couldn't remove those cookies.

`zinc.NewContext` and `c.PathParams` are internal. A `Context` without an app could not bind or encode JSON safely. Use `app.AcquireContext` in the rare adapter that needs one, and `httptest` against `app.ServeHTTP` in tests.

## Routing

Registration methods return the `Route` they create, so naming a route is one chained call:

```go
// 0.3
app.Handle(zinc.RouteSpec{Name: "users.show", Method: zinc.MethodGet, Path: "/users/{id}", Handler: showUser})

// 0.4
app.Get("/users/{id}", showUser).Name("users.show")
```

`Handle(RouteSpec)` is removed. `TryHandle(RouteSpec) error` stays for routes from configuration or plugins, and reports every problem, including a duplicate name, as an error.

| 0.3 | 0.4 |
|---|---|
| `app.Any(path, h)` | `app.All(path, h)` |
| `app.RoutesByMethod(m)`, `app.RoutesByPrefix(p)` | Filter `app.Routes()` |
| `zinc.NewGroup(app, prefix)` | `app.Group(prefix)` |
| `zinc.RouteHandler` | `zinc.HandlerFunc` |
| `if err := app.Static(...); err != nil` | `app.Static(...)` |

`Static`, `StaticFS`, `File`, and `FileFS` no longer return an error; `File` and `FileFS` return the `Route`. The only failure they reported, a nil filesystem, now panics at startup like every other registration mistake.

Standard middleware can now run on a group or a single route, not only the whole app: `group.UseHTTP(mw)` or `zinc.FromHTTP(mw)`. See [Zinc and net/http](/guide/http-interoperability/).

The router's internal types are no longer exported: `Router`, `Route` (the old static-route entry), `RouteMap`, `RouteHandlerMap`, and `RouteCache`. `zinc.Route` is now the handle registration returns.

## Middleware packages

The single `middleware` package is now one package per middleware, in the style of Fiber: `github.com/0mjs/zinc/middleware/cors`, `.../logger`, and so on. Each has a `New` function that takes an optional `Config`, whose zero value means the defaults. The `XWithConfig` constructors, the `DefaultXConfig` functions, and the shorthand constructors are gone, and names lose their prefix inside their package: `middleware.CORSConfig` is `cors.Config`, `middleware.ErrBasicAuthCredentialsMissing` is `basicauth.ErrCredentialsMissing`.

```go
// 0.3
app.Use(middleware.RequestID(), middleware.RequestLogger(), middleware.Recover())
app.Use(middleware.CORS("https://app.example.com"))

// 0.4
app.Use(requestid.New(), logger.New(), recover.New())
app.Use(cors.New(cors.Config{AllowOrigins: []string{"https://app.example.com"}}))
```

| 0.3 | 0.4 |
|---|---|
| `BasicAuth(v)`, `BasicAuthWithConfig(cfg)` | `basicauth.New(basicauth.Config{Validator: v})` |
| `BasicAuthCurrent(c)`, `BasicAuthUsername(c)` | `basicauth.Get(c)`, then `.Username` |
| `BodyDump(observe)` | `bodydump.New(bodydump.Config{Observe: observe})` |
| `BodyLimit(n)` | `bodylimit.New(bodylimit.Config{Limit: n})` |
| `CasbinAuth(e, subject)` | `casbin.New(casbin.Config{Enforcer: e, Subject: subject})` |
| `ContextTimeout(d)` | `timeout.New(timeout.Config{Timeout: d})` |
| `ContextTimeoutCurrent(c)`, `ErrContextTimeout` | `timeout.Get(c)`, `timeout.ErrExceeded` |
| `CORS(origins...)`, `CORSWithOptions(...)` | `cors.New(cors.Config{AllowOrigins: origins})` |
| `CSRF()`, `CSRFToken(c)` | `csrf.New()`, `csrf.Token(c)` |
| `Decompress()` | `decompress.New()` |
| `Gzip()`, `GzipWithConfig(cfg)` | `compress.New()`, `compress.New(compress.Config{...})` |
| `KeyAuth(v)`, `KeyAuthCurrent(c)` | `keyauth.New(keyauth.Config{Validator: v})`, `keyauth.Get(c)` |
| `MethodOverride()` | `methodoverride.New()` |
| `Pprof()`, `PprofWithPrefix(p)` | `pprof.New()`, `pprof.New(pprof.Config{Prefix: p})` |
| `Prometheus(m)`, `PrometheusHandler(m)`, `NewPrometheusMetrics()` | `prometheus.New(prometheus.Config{Metrics: m})`, `prometheus.Handler(m)`, `prometheus.NewMetrics(0)` |
| `Proxy(url)` | `proxy.New(proxy.Config{Target: url})` |
| `ProxyConfig.Modify` | `proxy.Config.ModifyResponse` |
| `RateLimiter(cfg)`, `IPRateLimiter(rate, burst)` | `limiter.New(limiter.Config{Rate: rate, Capacity: burst, Key: (*zinc.Context).IP})` |
| `RateLimiterConfig.KeyGenerator`, `IPLookup` | `limiter.Config.Key` |
| `RateLimiterConfig.LimitReachedHandler`, `StatusCode` | `limiter.Config.LimitReached` |
| `Throttle(n)` | `limiter.Concurrency(n)` |
| `Recover()` | `recover.New()` |
| `Redirect(from, to)`, `RedirectWithRules(rules)` | `redirect.New(redirect.Config{Rules: rules})` |
| `RequestID()`, `RequestIDValue(c)` | `requestid.New()`, `requestid.Get(c)` |
| `RequestIDConfig.Generator`, `StaticRequestID(id)` | `requestid.Config.Generate`, `requestid.Static(id)` |
| `RequestLogger()`, `Logger()` | `logger.New()` |
| `Rewrite(from, to)`, `RewriteWithRules(rules)` | `rewrite.New(rewrite.Config{Rules: rules})` |
| `Secure()` | `secure.New()` |
| `SessionCookie(name, secret)`, `MustSession(c)` | `session.New(session.Config{Name: name, Secret: secret})`, `session.MustGet(c)` |
| `SessionConfig.HTTPOnly` | Always on unless `session.Config.DisableHTTPOnly` |
| `TrailingSlash()`, `AddTrailingSlash()` | `trailingslash.New()`, `trailingslash.New(trailingslash.Config{Add: true})` |
| `NoCache()` | `nocache.New()` |
| `Heartbeat(path)` | `healthcheck.New(healthcheck.Config{Path: path})`, which also takes a `Check` |
| `AllowContentType(types...)`, `AllowContentEncoding(encs...)` | `contenttype.New(contenttype.Config{Types: types, Encodings: encs})` |
| `SetHeader(k, v)`, `RouteHeaders(routes...)` | `headers.New(headers.Config{Set: map[string]string{k: v}, Routes: routes})` |
| `Maybe(pred, mw)` | `zinc.Skip(func(c *zinc.Context) bool { return !pred(c) }, mw)` |

Accessors are `Get` and `MustGet` in every package that publishes request state. The named `ErrorHandler` types are plain `func` fields now, which you set the same way.

### The request logger

`logger.Config` has four fields: `Logger`, `Log` (formerly `LogValuesFunc`), `Headers`, and `QueryParams` (formerly `LogHeaders` and `LogQueryParams`). The twelve `LogX` switches are gone: every field of `logger.Values` is always filled, and the default line includes the route pattern. `HandleError` is gone too, because returned errors always go through the error handler before the line is written. `BeforeNextFunc` is gone; put that code in a middleware registered before the logger.

### Skipping middleware

Every `Skipper` field is removed. Wrap the middleware with `zinc.Skip`, which works with any middleware, including your own:

```go
// 0.3
app.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
	Skipper: func(c *zinc.Context) bool { return c.Path() == "/healthz" },
}))

// 0.4
app.Use(zinc.Skip(func(c *zinc.Context) bool { return c.Path() == "/healthz" }, logger.New()))
```

### Moved and removed

- **JWT** depends on golang-jwt, so it moved to its own module, `github.com/0mjs/contrib/jwtauth`, and Zinc's `go.mod` no longer requires it. The package is named `jwtauth` so it does not clash with golang-jwt, which you import for the token type. The API follows the same shape: `jwtauth.New(jwtauth.Config{KeyFunc: ...})`, `jwtauth.Claims[T](c)`, and `jwtauth.Get(c)` for the token, whose `Raw` field replaces `JWTTokenString`.
- **Gzip** is now the `compress` package, so it does not clash with the standard library's `compress/gzip`.
- **Static middleware** is removed; `app.Static` and `app.StaticFS` serve files. To serve files on the same paths as routes, see [Static Files](/guide/static-files/#files-and-routes-on-the-same-paths).
- **Jaeger** is removed. Use [OpenTelemetry](/middleware/open-telemetry/), which exports to Jaeger.
- **RealIP** is removed. It rewrote `RemoteAddr` from `c.IP()`, which Zinc already computes from `TrustedProxies`; read `c.IP()` instead.

## Body formats

Zinc's go.mod no longer requires any module. YAML and TOML were its only dependencies, and every app paid for them, so they are no longer built in. Add them back with the library of your choice; any library's `Unmarshal` and `Marshal` plug in directly:

```go
import "go.yaml.in/yaml/v3"

app := zinc.New(zinc.Config{
	Decoders: map[string]zinc.Decoder{
		"application/yaml":   yaml.Unmarshal,
		"application/x-yaml": yaml.Unmarshal,
		"text/yaml":          yaml.Unmarshal,
	},
	Encoders: map[string]zinc.Encoder{"application/yaml": yaml.Marshal},
})
```

| 0.3 | 0.4 |
|---|---|
| A YAML or TOML body with `c.Bind().Body` or `.All` | Unchanged once the decoder is configured |
| `c.Bind().YAML(&v)`, `c.Bind().TOML(&v)` | `c.Bind().Body(&v)`, which picks the decoder from `Content-Type` |
| `c.YAML(v)`, `c.TOML(v)` | `c.Encode("application/yaml", v)`, `c.Encode("application/toml", v)` |
| YAML or TOML in `c.Negotiate` | Unchanged once the encoder is configured |
| `Config{JSONCodec: myCodec}` | `Config{Decoders: map[string]zinc.Decoder{"application/json": myUnmarshal}, Encoders: map[string]zinc.Encoder{"application/json": myMarshal}}` |
| `Config{RequestBinder: myBinder}` | Removed. Use `Decoders` for other formats and `Validator` for checks. |

Two behaviors to know:

- A custom decoder's error, including a custom JSON decoder's, answers `400 Bad Request` with "invalid request body", keeping the error for logs. In 0.3, errors from a custom JSON codec answered 500. Return a Zinc error, such as `zinc.InternalServerError(...)`, from a decoder when the failure is the server's. The same applies to an error from a type's own `UnmarshalJSON` method.
- Zinc writes a custom encoder's bytes exactly as returned. The built-in JSON encoder ends each body with a newline; a replacement may not.

See [Body formats](/guide/customization/#body-formats) for TOML, a different JSON library, and strict decoding.

## New: typed handlers

Zinc 0.4 adds `zinc.Typed`, which turns a function whose signature is the request contract into a handler. It is optional, and existing handlers are unaffected:

```go
api.Post("/users", zinc.Typed(func(c *zinc.Context, in CreateUser) (User, error) {
	return users.Create(c.Context(), in)
})).Status(zinc.StatusCreated)
```

`RouteInfo` gains a `Status` field, which reports a status declared with `Route.Status`. See [Typed Handlers](/guide/typed-handlers/).
