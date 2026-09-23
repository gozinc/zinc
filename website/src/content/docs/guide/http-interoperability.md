---
title: Zinc and net/http
description: Run Zinc on your own http.Server, serve routes with standard handlers, wrap the app in standard middleware, and mount existing muxes.
---

Zinc is built on `net/http`, not beside it. A Zinc app is an `http.Handler`, standard handlers can serve Zinc routes, and standard middleware wraps Zinc apps without adapters. You can adopt Zinc one route at a time and leave working code alone.

## Run Zinc on your own server

`app.Listen` is a shortcut. For full control over timeouts, TLS, and lifecycle, give the app to an `http.Server`:

```go
server := &http.Server{
	Addr:              ":8080",
	Handler:           app,
	ReadHeaderTimeout: 5 * time.Second,
}
log.Fatal(server.ListenAndServe())
```

Because the app is a handler, it also works anywhere a handler is accepted: `httptest.NewServer(app)`, another router, or a serverless adapter.

## Serve a route with a standard handler

`HandleHTTP` takes a `METHOD /pattern` string, the same shape as `http.ServeMux`, and any `http.Handler`:

```go
app.HandleHTTP("GET /metrics", promhttp.Handler())

app.HandleHTTP("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "user %s", r.PathValue("id"))
}))
```

The standard handler receives the real request and response writer. Zinc does not copy or translate them. Route parameters are copied into `r.PathValue` just before the handler runs, so Zinc handlers, which use `c.Param`, never pay that cost.

Groups support `HandleHTTP` too, and group middleware still runs around the handler:

```go
admin := app.Group("/admin", requireAdmin)
admin.HandleHTTP("GET /debug/vars", expvar.Handler())
```

To use a standard handler inside a Zinc middleware chain, convert it with `zinc.Wrap(handler)` or `zinc.WrapFunc(fn)`.

## Mount a whole subtree

When an existing handler owns every path below a prefix, mount it:

```go
app.Mount("/legacy", legacyMux)
```

`Mount` removes the prefix before calling the handler, so `GET /legacy/orders` reaches `legacyMux` as `GET /orders`. Do not add `http.StripPrefix` as well. Use `HandleHTTP` for one endpoint and `Mount` for a subtree.

## Wrap the app in standard middleware

`UseHTTP` accepts ordinary `func(http.Handler) http.Handler` middleware:

```go
app.UseHTTP(func(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "api")
})
```

Standard middleware wraps the entire application and runs before any Zinc middleware. When you register several, the first is outermost:

```text
UseHTTP middleware
  → Zinc app middleware
    → group and route middleware
      → handler
```

Values that standard middleware adds to the request context, such as a trace span, are visible in Zinc handlers through `c.Context()`.

## Writer capabilities are preserved

Handlers receive the server's own response writer, so `http.Flusher`, `http.Hijacker`, `io.ReaderFrom`, and `Unwrap` keep working. WebSocket libraries, streaming, and `http.ResponseController` behave exactly as they do without Zinc.

## Next steps

- [Adopt Zinc in net/http](/cookbook/existing-net-http-service/) adds Zinc to an existing service step by step.
- [OpenTelemetry](/middleware/open-telemetry/) traces a Zinc app with the official instrumentation.
- [Testing](/guide/testing/) uses `httptest` directly against the app.
