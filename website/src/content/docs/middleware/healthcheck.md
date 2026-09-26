---
title: Health Check
description: Answer load balancer and uptime checks before routing.
---

`healthcheck` answers `GET` and `HEAD` requests to a health path with `204 No Content`, before route handlers run.

```go
import "github.com/0mjs/zinc/middleware/healthcheck"

app.Use(healthcheck.New()) // answers /healthz
```

Give it a `Check` to report readiness. A non-nil error answers `503 Service Unavailable`; the error itself is kept for logs and never sent to the client.

```go
app.Use(healthcheck.New(healthcheck.Config{
	Path: "/readyz",
	Check: func(c *zinc.Context) error {
		return db.PingContext(c.Context())
	},
}))
```

Register it once per probe to serve both liveness and readiness. The [health and readiness cookbook](/cookbook/health-readiness/) covers probes that need application logic.
