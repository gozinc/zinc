---
title: Prometheus
description: Count requests and their durations per route, and serve them for Prometheus to scrape, with no dependencies.
---

Prometheus counts requests and their total duration for each route, method and status, and serves the numbers in the text format Prometheus scrapes. Add it when you want request rates, error rates and average latency without adding the Prometheus client library. For histograms or custom metrics, use the official client instead ([see below](#use-the-official-client-instead)).

## Usage

```go
import "github.com/0mjs/zinc/middleware/prometheus"

metrics := prometheus.NewMetrics(0)

app.Use(prometheus.New(prometheus.Config{Metrics: metrics}))
app.Get("/metrics", prometheus.Handler(metrics))
```

After two requests to `/users/{id}` and one to a path with no route:

```bash
curl http://localhost:8080/metrics
# # TYPE zinc_http_requests_total counter
# zinc_http_requests_total{method="GET",route="/users/{id}",status="200"} 2
# zinc_http_requests_total{method="GET",route="unmatched",status="404"} 1
# # TYPE zinc_http_request_duration_seconds summary
# zinc_http_request_duration_seconds_sum{method="GET",route="/users/{id}",status="200"} 0.000007583
# zinc_http_request_duration_seconds_sum{method="GET",route="unmatched",status="404"} 0.000001458
# zinc_http_request_duration_seconds_count{method="GET",route="/users/{id}",status="200"} 2
# zinc_http_request_duration_seconds_count{method="GET",route="unmatched",status="404"} 1
# # TYPE zinc_http_metrics_dropped_total counter
# zinc_http_metrics_dropped_total 0
```

The response's `Content-Type` is `text/plain; version=0.0.4; charset=utf-8`. Register the middleware with `app.Use`, so requests that match no route are counted too.

## Defaults

| Setting | Default |
|---|---|
| Registry | a new one for this middleware |
| Most series kept | 10,000 |

## Configuration

Pass a registry you create, so `Handler` and the middleware share it:

```go
metrics := prometheus.NewMetrics(5000) // keep at most 5,000 series

app.Use(prometheus.New(prometheus.Config{Metrics: metrics}))
app.Get("/metrics", prometheus.Handler(metrics))
```

| Field | Default | Meaning |
|---|---|---|
| `Metrics` | a new registry | The `*prometheus.Metrics` to record into |
| `Now` | `time.Now` | Clock, for tests |

`prometheus.NewMetrics(maxSeries)` creates a registry. `0` means 10,000 series; a negative number panics.

## What it records

| Metric | Type | Meaning |
|---|---|---|
| `zinc_http_requests_total` | counter | Requests handled |
| `zinc_http_request_duration_seconds_sum` | summary | Total time spent, in seconds |
| `zinc_http_request_duration_seconds_count` | summary | Requests timed |
| `zinc_http_metrics_dropped_total` | counter | Observations dropped because the series limit was reached |

Each series has three labels:

- **`method`**: the request method. Methods other than the nine standard ones become `OTHER`.
- **`route`**: the matched route pattern, such as `/users/{id}`, not the path. Requests with no matching route become `unmatched`.
- **`status`**: the status code sent to the client, after your error handler has run.

Because labels use patterns and a fixed set of methods, random paths and methods from clients don't create new series. Divide `_sum` by `_count` for the average duration.

## Use the official client instead

For histograms, percentiles or your own metrics, use the Prometheus client library and register its handler with [`HandleHTTP`](/guide/http-interoperability/):

```go
app.HandleHTTP("GET /metrics", promhttp.Handler()) // promhttp: github.com/prometheus/client_golang/prometheus/promhttp
```

## Share or separate registries

When you don't pass `Config.Metrics`, the middleware creates its own registry. `prometheus.Handler()` with no argument then serves the registry of the middleware that ran for that request:

```go
app.Use(prometheus.New())
app.Get("/metrics", prometheus.Handler())
```

If no prometheus middleware ran for the request, `Handler()` answers `503` with the body `Prometheus middleware is not configured`.

Each call to `prometheus.New()` without `Config.Metrics` gets a separate registry. To have several middleware values record into one set of numbers, create a registry with `NewMetrics` and pass it to each.

You can also record your own observations with `metrics.Observe(method, route, status, duration)`, and read the text output with `metrics.Text()`. Pass only labels your code controls, not values taken from the request, or each new value becomes a new series.

## Errors

When the registry holds its maximum number of series, observations for a new label set are dropped and counted in `zinc_http_metrics_dropped_total`. Series already recorded keep updating.

## Related

- [pprof](/middleware/pprof/) profiles CPU and memory in a running service.
- [OpenTelemetry](/middleware/open-telemetry/) traces each request.
- [Zinc and net/http](/guide/http-interoperability/) covers `HandleHTTP` for the official client.
