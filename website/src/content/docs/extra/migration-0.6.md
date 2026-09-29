---
title: Upgrading to 0.6
description: What changed in Zinc 0.6. Most apps need no code changes; one misuse of redirect or rewrite now fails at startup.
slug: extra/migration-0.6
---

Most apps move from 0.5 to 0.6 with no code changes. Update the module and run your tests:

```bash
go get github.com/0mjs/zinc@v0.6.0
go test ./...
```

0.6 adds [OpenAPI](/guide/openapi/): Zinc describes your API from the types you already write, and serves the spec and a docs page if you ask it to. Nothing about how routes serve requests changes, and nothing is served until you call `app.OpenAPI`.

## What might need a change

### Redirect and rewrite on a group fail at startup

`redirect` and `rewrite` must run before routing. On a group they run after a route has matched, where they can't work: 0.5.2 logged a warning the first time. 0.6 panics when you register them on a group:

```go
app.Group("/api").Use(redirect.New(cfg))
// panic: zinc: redirect middleware on group "/api" would run after routing, where it can't work; register it with app.Use, or app.UsePrefix("/api", ...) for the group's paths
```

Move it to `app.Use`, or `app.UsePrefix` for the group's paths. On a single route it still works for that path, and still warns.

## What's new

- [OpenAPI](/guide/openapi/): `app.OpenAPI(path, cfg)` serves an OpenAPI 3.1 spec; `app.OpenAPISpec(cfg)` returns it for tests and tooling. Typed handlers are described with no extra code.
- New `Route` methods for the spec: `Summary`, `Description`, `Tags`, `Deprecated`, `Hidden`, `Input`, `Output`, `Response`, `Errors` and `Security`. `Group` gains `Tags` and `Security`. `Name` also sets the operation ID.
- [API Docs](/middleware/apidocs/): a browsable page for the spec, with Scalar, Swagger UI, Stoplight Elements or ReDoc.
- `zinc.SchemaProvider`, for a type that describes its own JSON Schema.
- [Generate an API Client](/cookbook/openapi-client/): a recipe for a typed Go client with oapi-codegen.

## Benchmarks are scored with ties

The [benchmark](/extra/benchmarks/) scores count a scenario as a tie when Zinc and the fastest rival are within 3% of each other, either way. Before, the lower median won however small the gap, so a 0.2% difference counted as much as a 50% one. The headline is lower than 0.5's as a result, and every tie is listed.

The suite also gains `APIProductionStack`: an API route behind a request ID, an access log, panic recovery and CORS, each framework using its own middleware.

## Next steps

- [OpenAPI](/guide/openapi/): describe, serve and export your API's spec.
- [Release notes](https://github.com/0mjs/zinc/releases): every change in 0.6.0.
