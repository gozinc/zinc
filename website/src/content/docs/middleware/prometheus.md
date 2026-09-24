---
title: Prometheus
description: Expose dependency-free request metrics in Prometheus text format.
---

`Prometheus` records a request counter and total duration for each route, method, and status, and serves them in Prometheus text format. It needs no dependencies. For histograms or the official client library, register `promhttp.Handler()` with [`HandleHTTP`](/guide/http-interoperability/) instead.

```go
metrics := middleware.NewPrometheusMetrics()

app.Use(middleware.Prometheus(metrics))
app.Get("/metrics", middleware.PrometheusHandler(metrics))
```

The handler writes Prometheus text format:

```text
zinc_http_requests_total
zinc_http_request_duration_seconds_sum
zinc_http_request_duration_seconds_count
```

Labels include:

- `method`
- `route`
- `status`

Matched requests use registered route patterns, such as `/users/{id}`. Unmatched requests use `unmatched`; nonstandard methods use `OTHER`. Random request paths and methods therefore do not create new series.

Each `Prometheus()` construction creates an isolated registry. `PrometheusHandler()` without an argument uses the registry attached by middleware on that request, including when collection is skipped. Without middleware or an explicit registry, the handler returns 503. Reusing one middleware instance intentionally shares its registry; supply explicit registries when composing multiple collectors.

`NewPrometheusMetrics(maxSeries)` optionally sets a series cap (default 10,000). New series beyond the cap are dropped, counted by `zinc_http_metrics_dropped_total`. Existing series continue updating. Direct calls to `Observe` should use application-controlled labels.
