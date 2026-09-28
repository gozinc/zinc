---
title: Health Check
description: Answer load balancer and uptime probes with 204, or 503 when a check fails.
---

Health Check answers load balancer and orchestrator probes, such as a Kubernetes liveness check. It replies `204 No Content` without running your routes, and `503 Service Unavailable` when a check you supply fails.

## Usage

```go
import "github.com/0mjs/zinc/middleware/healthcheck"

app.Use(healthcheck.New()) // answers GET and HEAD /healthz
```

```bash
curl -i http://localhost:8080/healthz
# HTTP/1.1 204 No Content
```

Only `GET` and `HEAD` requests for exactly `/healthz` are answered. Other methods and paths continue down the chain as usual.

Register it before [`logger`](/middleware/logger/) if you don't want a log line for every probe.

## Defaults

| Setting | Default |
|---|---|
| Path | `/healthz` (`healthcheck.DefaultPath`) |
| Check | none: always healthy |

## Configuration

Give it a `Check` to report readiness. This one fails while the database is unreachable:

```go
app.Use(healthcheck.New(healthcheck.Config{
	Path: "/readyz",
	Check: func(c *zinc.Context) error {
		return db.PingContext(c.Context()) // db: your *sql.DB
	},
}))
```

```bash
curl -i http://localhost:8080/readyz   # while the database is down
# HTTP/1.1 503 Service Unavailable
# {"error":{"status":503,"message":"unhealthy"}}
```

| Field | Default | Meaning |
|---|---|---|
| `Path` | `/healthz` | Path to answer. It's matched exactly: case and a trailing slash both count. |
| `Check` | none | `func(*zinc.Context) error`. A non-nil error answers `503`. Nil means always healthy. |

## Serve liveness and readiness

Register it once per probe:

```go
app.Use(
	healthcheck.New(), // liveness: the process is up
	healthcheck.New(healthcheck.Config{Path: "/readyz", Check: checkDependencies}), // checkDependencies: your check
)
```

## Errors

A failing check returns `zinc.ServiceUnavailable("unhealthy")`, wrapping your error. The client only ever sees `unhealthy`; your error's text is not sent. It's still available to middleware and the error handler with `errors.Is` and `errors.As`.

The logger's `error` key shows `unhealthy`, the message of the outer error:

```text
2026/09/28 01:17:01 ERROR REQUEST_ERROR method=GET uri=/readyz status=503 ... error=unhealthy
```

:::note[It answers before your routes]
Added with `app.Use`, the middleware answers `GET` and `HEAD` for its path before routing. A route you register at the same path only receives other methods.
:::

## Related

- [Health and readiness](/cookbook/health-readiness/) covers probes that need application logic.
- [Graceful shutdown](/cookbook/graceful-shutdown/) covers stopping the server without dropping requests.
- [Prometheus](/middleware/prometheus/) exposes request metrics next to your probes.
