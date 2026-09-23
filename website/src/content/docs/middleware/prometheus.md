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

Matched requests are labelled with the route pattern, such as `/users/{id}`, so one route is one series. Requests that match no route are labelled with their raw path, so scanners probing random URLs add a new series for each path. On public apps, attach the middleware to a [group](/guide/groups-and-middleware/) rather than `app.Use`. Group middleware only runs for matched routes, so only routed traffic is recorded.

Create a separate `NewPrometheusMetrics()` for each app in tests or multi-app processes.
