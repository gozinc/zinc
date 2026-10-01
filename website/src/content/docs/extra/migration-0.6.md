---
title: Upgrading to 0.6
description: What changed in Zinc 0.6. Every app now serves an OpenAPI spec at /openapi.json, which a private API must turn off or protect; one misuse of redirect or rewrite now fails at startup.
slug: extra/migration-0.6
---

Most apps move from 0.5 to 0.6 with no code changes, but read the first section below: a private API needs one line. Update the module and run your tests:

```bash
go get github.com/0mjs/zinc@v0.6.0
go test ./...
```

0.6 adds [OpenAPI](/guide/openapi/): Zinc describes your API from the types you already write, and serves the spec. Nothing about how your routes serve requests changes.

## What might need a change

### Every app serves its spec at /openapi.json

An app now answers `GET /openapi.json` with an OpenAPI spec that lists every route that isn't hidden, including admin and internal ones. **If your API is private, or you don't want its shape public, you need to change this before you deploy 0.6.** Turn it off:

```go
app := zinc.New(zinc.Config{OpenAPIPath: "-"})
```

or protect it with middleware, which replaces the default spec:

```go
app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{}, requireAPIKey) // requireAPIKey: your middleware
```

Middleware added with `app.Use` also runs for the default spec, so an app whose every request needs auth is already protected. A route of your own at `/openapi.json` still wins. To keep a route out of the spec, use `.Hidden()` on it, or on its group.

### Redirect and rewrite on a group fail at startup

`redirect` and `rewrite` must run before routing. On a group they run after a route has matched, where they can't work: 0.5.2 logged a warning the first time. 0.6 panics when you register them on a group:

```go
app.Group("/api").Use(redirect.New(cfg))
// panic: zinc: redirect middleware on group "/api" would run after routing, where it can't work; register it with app.Use, or app.UsePrefix("/api", ...) for the group's paths
```

Move it to `app.Use`, or `app.UsePrefix` for the group's paths. On a single route it still works for that path, and still warns.

## What's new

- [OpenAPI](/guide/openapi/): every app serves an OpenAPI 3.1 spec, described by `Config.OpenAPI`; `app.OpenAPI(path, cfg)` serves it elsewhere or behind middleware, and `app.OpenAPISpec(cfg)` returns it for tests and tooling. Typed handlers are described with no extra code.
- New `Route` methods for the spec: `Summary`, `Description`, `Tags`, `Deprecated`, `Hidden`, `Input`, `Output`, `Response`, `Errors`, `Security` and `SecurityAll`. `Group` gains `Tags`, `Security`, `SecurityAll` and `Hidden`. `Name` also sets the operation ID.
- [Binding](/guide/binding/) reads cookies with a `cookie` tag (in typed handlers and `c.Bind().Cookie`), and fills a `default` tag's value when the request leaves a field out.
- [API Docs](/middleware/apidocs/): a browsable page for the spec, with Scalar, Swagger UI, Stoplight Elements or ReDoc.
- `zinc.SchemaProvider`, for a type that describes its own JSON Schema, and `zinc.EnumProvider`, for a named type that lists its values.
- [Generate an API Client](/cookbook/openapi-client/): a recipe for a typed Go client with oapi-codegen.

## Changes in 0.6.1

- **Routes OpenAPI can't tell apart** make the spec fail to build, with an error naming both. Before, one replaced the other silently, or the spec had two paths differing only in parameter names, which OpenAPI forbids. Rename the parameters to match, or hide one route. See [Routes OpenAPI can't tell apart](/guide/openapi/#routes-openapi-cant-tell-apart).
- **A type of yours named `Error`** keeps its own schema, listed under its package, such as `main.Error`. Before, Zinc's error schema replaced it.
- **The served spec** picks up metadata changed after the first request, such as `.Hidden()` or `.Summary(...)`. Before, only new routes rebuilt it.
- **Embedded structs bind.** Tagged fields of an embedded struct used to be ignored by binding, though the spec listed them; now they [bind](/guide/binding/#embedded-structs-bind-too).
- **A field binding can't fill** (a `map`, a `[]bool`) panics when its typed handler is registered, and binders return the error, instead of failing only on a request that carried the value.
- **A typed handler's own `c.Status(...)` wins** over `.Status(...)` on the route, `200` included. Before, `200` was treated as unset and replaced.
- **`zinc.Form[T]`** reports a body over the limit as `413`, and a form that can't be parsed as a `400` with the cause, instead of "is required".
- **CSRF's `FromForm`** honors `Config.BodyLimit`: a larger body gets `413`.

## Benchmarks are scored with ties

The [benchmark](/extra/benchmarks/) scores count a scenario as a tie when Zinc and the fastest rival are within 3% of each other, either way. Before, the lower median won however small the gap, so a 0.2% difference counted as much as a 50% one. The headline is lower than 0.5's as a result, and every tie is listed.

The suite also gains `APIProductionStack`: an API route behind a request ID, an access log, panic recovery and CORS, each framework using its own middleware.

## Next steps

- [OpenAPI](/guide/openapi/): describe, serve and export your API's spec.
- [Release notes](https://github.com/0mjs/zinc/releases): every change in 0.6.0.
