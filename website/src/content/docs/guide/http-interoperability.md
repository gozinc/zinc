---
title: Zinc and net/http
description: Run Zinc on your own http.Server, serve routes with standard handlers, wrap the app in standard middleware, and mount existing muxes.
---

Zinc works with Go's `net/http` in both directions: standard handlers can serve Zinc routes, and standard middleware can wrap a Zinc app. Use this to add Zinc to a service one route at a time, or to plug in a library that only speaks `net/http`, such as Prometheus or OpenTelemetry.

```go
app := zinc.New()

// A Zinc route
app.Get("/health", func(c *zinc.Context) error {
	return c.String("ok")
})

// A standard handler on a Zinc route
app.HandleHTTP("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "user %s", r.PathValue("id"))
}))

// The app is an http.Handler
log.Fatal(http.ListenAndServe(":8080", app))
```

```bash
curl http://localhost:8080/users/42
# user 42
```

## Run Zinc on your own server

`app.Listen` is a shortcut. To control timeouts, TLS and shutdown yourself, give the app to an `http.Server`:

```go
server := &http.Server{
	Addr:              ":8080",
	Handler:           app,
	ReadHeaderTimeout: 5 * time.Second,
}
log.Fatal(server.ListenAndServe())
```

The [server settings](/guide/configuration/#server) in `zinc.Config`, such as `ReadTimeout`, only apply when Zinc starts the server. On your own server, set them there.

Because the app is a handler, it works anywhere a handler does: `httptest.NewServer(app)`, another router, or a serverless adapter. To keep Zinc's timeouts but supply your own listener, such as a Unix socket or one passed in by systemd, use `app.Serve(listener)`.

## Serve a route with a standard handler

`HandleHTTP` takes a `METHOD /pattern` string, the same shape as `http.ServeMux`, and any `http.Handler`:

```go
app.HandleHTTP("GET /metrics", promhttp.Handler())

app.HandleHTTP("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "user %s", r.PathValue("id"))
}))
```

```bash
curl http://localhost:8080/users/42
# user 42
```

The handler reads route parameters with `r.PathValue`, as it would under `http.ServeMux`. `r.Pattern` is the route's method and full path, here `GET /users/{id}`.

Groups have `HandleHTTP` too, and the group's middleware runs around the handler:

```go
admin := app.Group("/admin", requireAdmin) // requireAdmin: your Zinc middleware
admin.HandleHTTP("GET /debug/vars", expvar.Handler())
```

To use a standard handler anywhere a Zinc handler goes, convert it with `zinc.Wrap(handler)` or `zinc.WrapFunc(fn)`:

```go
app.Get("/legacy-report", requireAdmin, zinc.WrapFunc(legacyReport)) // legacyReport: an http.HandlerFunc
```

## Mount a whole subtree

When an existing handler owns every path below a prefix, mount it:

```go
legacyMux := http.NewServeMux()
legacyMux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "legacy saw %s", r.URL.Path)
})

app.Mount("/legacy", legacyMux)
```

```bash
curl http://localhost:8080/legacy/orders
# legacy saw /orders
```

`Mount` removes the prefix before calling the handler, so don't add `http.StripPrefix` as well. Use `HandleHTTP` for one endpoint and `Mount` for a subtree.

## Wrap the whole app in standard middleware

`UseHTTP` takes ordinary `func(http.Handler) http.Handler` middleware:

```go
app.UseHTTP(func(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "api")
})
```

It wraps the entire app, so it runs before routing and before any Zinc middleware. When you add several, the first one you add is the outermost:

```text
UseHTTP middleware
  → Zinc app middleware
    → group and route middleware
      → handler
```

Values that standard middleware puts in the request context, such as a trace span or a request ID, reach Zinc handlers through `c.Context()`:

```go
type requestIDKey struct{}

app.UseHTTP(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), requestIDKey{}, "req-123")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
})

app.Get("/whoami", func(c *zinc.Context) error {
	return c.String(c.Context().Value(requestIDKey{}).(string))
})
```

```bash
curl http://localhost:8080/whoami
# req-123
```

## Add standard middleware to a group or route

To run standard middleware on part of the app only, add it to a group with `UseHTTP`, or convert it with `zinc.FromHTTP` wherever Zinc middleware goes:

```go
creds := map[string]string{"admin": os.Getenv("ADMIN_PASSWORD")}
admin := app.Group("/admin").UseHTTP(chimw.BasicAuth("admin", creds)) // chimw: github.com/go-chi/chi/v5/middleware

app.Get("/reports", zinc.FromHTTP(otelhttp.NewMiddleware("reports")), listReports) // listReports: your handler
```

Inside the standard middleware, the rest of the Zinc chain runs as its `next` handler. If your handler returns an error, Zinc writes the error response before the middleware returns, so a logging or metrics middleware sees the final status:

```go
func logStatus(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d", r.Method, r.URL.Path, rec.status)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int)        { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

app.Get("/reports", zinc.FromHTTP(logStatus), func(c *zinc.Context) error {
	return zinc.NotFound("no reports yet")
})
```

```bash
curl http://localhost:8080/reports
# {"error":{"status":404,"message":"no reports yet"}}
```

```text
2026/09/28 10:15:02 GET /reports 404
```

:::caution[Wrapped writers need Unwrap]
If your middleware wraps the response writer, the wrapper must have an `Unwrap() http.ResponseWriter` method, as `statusRecorder` does. It's the same convention `http.ResponseController` uses. Without it, the request panics with `zinc: FromHTTP middleware hid the response writer; its wrapper must implement Unwrap() http.ResponseWriter`.
:::

`FromHTTP` calls your middleware function once, when you register the route, not on every request. Any setup it does before returning its handler happens at startup:

```go
app.Get("/a", zinc.FromHTTP(func(next http.Handler) http.Handler {
	log.Print("building middleware") // printed once, at registration
	return next
}), handleA) // handleA: your handler
```

If the middleware passes a new request to `next`, for example one with a new context, later Zinc handlers see that request. If it passes a wrapped writer, they write through the wrapper.

## Streaming, WebSockets and ResponseController keep working

Standard handlers get a response writer that keeps the features of the server's own writer: `http.Flusher`, `http.Hijacker`, `http.Pusher`, `io.ReaderFrom` and `Unwrap`. WebSocket libraries, streaming and `http.ResponseController` work the same as without Zinc.

## Good to know

### Path values are set only for standard code

Zinc copies the matched route into the request right before a standard handler, or standard middleware on a group or route (`FromHTTP`, `Group.UseHTTP`), runs: `r.PathValue` returns each parameter and `r.Pattern` is the route's `METHOD /path`, such as `GET /users/{id}`. A HEAD request answered by a GET route gets the GET pattern, as with `http.ServeMux`. Middleware on the whole app, from `App.UseHTTP` or `App.Use`, runs before routing, so it sees neither. Zinc handlers read parameters with `c.Param` instead, so `r.PathValue` is empty inside them unless standard middleware ran first.

### The writer is a thin wrapper

The writer a standard handler receives is Zinc's wrapper around the server's writer, not the server's writer itself. It only offers features the server's writer has: under `httptest.NewRecorder`, for example, it isn't an `http.Hijacker`. The request is passed through as it is, apart from the path values Zinc sets on it.

### Mounted handlers get a trimmed copy of the request

A handler mounted at `/legacy` sees `/legacy/orders` as `/orders`. Zinc gives it a copy of the request with the path trimmed, so your outer middleware still sees the original.

## Next steps

- [Adopt Zinc in net/http](/cookbook/existing-net-http-service/): add Zinc to an existing service step by step.
- [OpenTelemetry](/middleware/open-telemetry/): trace a Zinc app with the official instrumentation.
- [Testing](/guide/testing/): use `httptest` directly against the app.
