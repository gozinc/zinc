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

For lightweight Jaeger-compatible trace propagation without the OpenTelemetry SDK, Zinc also includes [Jaeger middleware](/middleware/jaeger/).
