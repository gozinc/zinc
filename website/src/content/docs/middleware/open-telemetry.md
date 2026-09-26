---
title: OpenTelemetry
description: Trace Zinc requests with the standard OpenTelemetry net/http instrumentation.
---

Zinc implements `http.Handler`, so the official OpenTelemetry HTTP instrumentation works without a framework adapter.

```go
app := zinc.New()
app.Get("/users/{id}", showUser)

traced := otelhttp.NewHandler(app, "zinc")

server := &http.Server{
	Addr:    ":8080",
	Handler: traced,
}

log.Fatal(server.ListenAndServe())
```

Install the instrumentation package:

```sh
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
```

Configure the OpenTelemetry SDK, resource, exporter, and propagator during application startup. The wrapper creates a server span around Zinc's complete middleware and routing lifecycle.

## Name spans after routes

The wrapper names every span `zinc`, because it runs before routing. Rename the span to the matched route pattern from a Zinc middleware, which runs after routing:

```go
app.Use(func(c *zinc.Context) error {
	if route := c.FullPath(); route != "" {
		trace.SpanFromContext(c.Context()).SetName(c.Method() + " " + route)
	}
	return c.Next()
})
```

`trace` is `go.opentelemetry.io/otel/trace`. Handlers that pass `c.Context()` to clients and databases continue the same trace.

Jaeger reads OpenTelemetry directly, so there is no separate Jaeger middleware; export to Jaeger with the OTLP exporter.
