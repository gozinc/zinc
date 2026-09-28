---
title: OpenTelemetry
description: Trace every request with OpenTelemetry's net/http instrumentation, added through app.UseHTTP.
---

This page is a recipe, not a Zinc package: it adds OpenTelemetry's standard `otelhttp` instrumentation to your app with `app.UseHTTP`. Use it when you send traces to a backend such as Jaeger, Tempo or Honeycomb, and want a span for every request, named after its route.

## Usage

Install the instrumentation, the SDK, and an exporter. This example prints spans to standard output:

```sh
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp \
  go.opentelemetry.io/otel/sdk \
  go.opentelemetry.io/otel/exporters/stdout/stdouttrace
```

```go
package main

import (
	"log"
	"net/http"

	"github.com/0mjs/zinc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	// Print spans to stdout. In production, use an OTLP exporter instead.
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatal(err)
	}
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter)))
	otel.SetTextMapPropagator(propagation.TraceContext{})

	app := zinc.New()

	// Start a server span for every request, before routing.
	app.UseHTTP(func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "http.server")
	})

	// Once routing has run, name the span after the route pattern.
	app.Use(func(c *zinc.Context) error {
		err := c.Next()
		if route := c.FullPath(); route != "" {
			span := trace.SpanFromContext(c.Context())
			span.SetName(c.Method() + " " + route)
			span.SetAttributes(attribute.String("http.route", route))
		}
		return err
	})

	app.Get("/users/{id}", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"id": c.Param("id")})
	})

	log.Fatal(app.Listen(":8080"))
}
```

```bash
curl http://localhost:8080/users/42
# {"id":"42"}
```

A few seconds later, when the batch is exported, the server prints the span (shortened here):

```text
{
	"Name": "GET /users/{id}",
	"SpanKind": 2,
	"Attributes": [
		{ "Key": "http.request.method", "Value": { "Type": "STRING", "Value": "GET" } },
		{ "Key": "url.path", "Value": { "Type": "STRING", "Value": "/users/42" } },
		{ "Key": "http.route", "Value": { "Type": "STRING", "Value": "/users/{id}" } },
		{ "Key": "http.response.status_code", "Value": { "Type": "INT64", "Value": 200 } },
		...
	],
	...
}
```

`UseHTTP` wraps the whole app, so the span covers all of Zinc's middleware and routing. Handlers that pass `c.Context()` to database drivers and HTTP clients add their spans to the same trace.

## Defaults

With no options, `otelhttp.NewHandler`:

| Setting | Default |
|---|---|
| Span name | the request method, such as `GET` |
| Tracer provider | the global one, from `otel.SetTracerProvider` |
| Propagator | the global one, from `otel.SetTextMapPropagator` |
| Which requests are traced | all of them |

The `operation` argument (`"http.server"` above) doesn't become the span name in current `otelhttp` versions; the method does.

## Configuration

`otelhttp.NewHandler` takes options. This one leaves health probes untraced:

```go
app.UseHTTP(func(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "http.server",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/healthz" // false means no span
		}),
	)
})
```

The options you'll use most:

| Option | Meaning |
|---|---|
| `otelhttp.WithTracerProvider(p)` | Use `p` instead of the global tracer provider |
| `otelhttp.WithPropagators(p)` | Use `p` to read incoming trace headers instead of the global propagator |
| `otelhttp.WithFilter(f)` | Trace only requests for which `f` returns `true` |
| `otelhttp.WithSpanNameFormatter(f)` | Name spans with `f`. It runs before routing, so it can't see the route pattern. |

The [otelhttp package documentation](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) lists the rest.

## Name spans after routes

`otelhttp` starts the span before Zinc routes the request, so it can only name it after the method. The `app.Use` middleware in the example renames it once the route is known.

Read `c.FullPath()` **after** `c.Next()`. Middleware added with `app.Use` runs before routing, so before `c.Next()` returns, `c.FullPath()` is still empty. Requests that match no route keep the method as their name.

## Send traces to Jaeger or another backend

Replace the stdout exporter with the OTLP exporter (`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` or `otlptracehttp`), pointed at your collector. Jaeger accepts OTLP directly, so there's no separate Jaeger middleware.

## Related

- [Zinc and net/http](/guide/http-interoperability/) covers `UseHTTP` and other standard middleware.
- [Prometheus](/middleware/prometheus/) exports request counts and durations.
- [Request ID](/middleware/requestid/) gives each request an ID you can log beside the trace ID.
