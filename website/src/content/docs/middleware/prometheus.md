---
title: Prometheus
description: Expose dependency-free request metrics in Prometheus text format.
---

`prometheus` records a request counter and total duration for each route, method, and status, and serves them in Prometheus text format. It needs no dependencies. For histograms or the official client library, register `promhttp.Handler()` with [`HandleHTTP`](/guide/http-interoperability/) instead.

```go
import "github.com/0mjs/zinc/middleware/prometheus"

metrics := prometheus.NewMetrics(0)

app.Use(prometheus.New(prometheus.Config{Metrics: metrics}))
app.Get("/metrics", prometheus.Handler(metrics))
```

The handler writes:

```text
zinc_http_requests_total
zinc_http_request_duration_seconds_sum
zinc_http_request_duration_seconds_count
```

Labels are `method`, `route`, and `status`. Matched requests use registered route patterns, such as `/users/{id}`. Unmatched requests use `unmatched`; nonstandard methods use `OTHER`. Random request paths and methods therefore do not create new series.

## Registries

`prometheus.New()` without `Config.Metrics` creates its own isolated registry. `prometheus.Handler()` without an argument serves the registry the middleware attached to that request, and answers 503 when there is none. Reusing one middleware value shares its registry; pass explicit registries when composing several collectors.

`prometheus.NewMetrics(maxSeries)` caps the number of series; `0` means 10,000. New series beyond the cap are dropped and counted by `zinc_http_metrics_dropped_total`, while existing series keep updating. Direct calls to `Metrics.Observe` should use application-controlled labels.
