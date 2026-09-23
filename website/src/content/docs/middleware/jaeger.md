---
title: Jaeger
description: Propagate Jaeger uber-trace-id headers and observe spans.
---

`Jaeger` reads and continues Jaeger's `Uber-Trace-Id` header and reports a span for each request to a function you provide. It has no dependencies. For new systems, prefer [OpenTelemetry](/middleware/open-telemetry/).

```go
app.Use(middleware.Jaeger(func(c *zinc.Context, span middleware.JaegerSpan) error {
	log.Printf("%s %s %s", span.TraceID, span.Operation, span.Duration)
	return nil
}))
```

If a request already has an `Uber-Trace-Id`, Zinc keeps the trace ID and uses the incoming span ID as the parent span. Otherwise Zinc generates a new trace ID and span ID.

Inside handlers, read the current span with:

```go
span, ok := middleware.JaegerCurrent(c)
```

Use `JaegerWithConfig` to customize operation names or timing.
