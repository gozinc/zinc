---
title: pprof
description: Expose Go's runtime profiler behind authentication for CPU, memory, and goroutine analysis.
---

`Pprof` serves the standard `net/http/pprof` handlers, so `go tool pprof` can profile a running service. It answers requests below `/debug/pprof` and passes everything else through.

Profiles reveal internals and can slow a server down, so always put authentication in front:

```go
app.UsePrefix("/debug/pprof",
	middleware.BasicAuth(middleware.BasicAuthStatic("ops", os.Getenv("PPROF_PASSWORD"))),
	middleware.Pprof(),
)
```

`UsePrefix` runs before routing, in order, so every profiling request is authenticated first.

Then, from your machine:

```bash
go tool pprof -http=:0 "https://ops:$PPROF_PASSWORD@api.example.com/debug/pprof/profile?seconds=30"
```

## Use another path

```go
app.UsePrefix("/internal/pprof", requireAdmin, middleware.PprofWithPrefix("/internal/pprof"))
```

:::danger[Never expose pprof publicly]
Do not register `Pprof` with plain `app.Use` on an internet-facing app. Protect it with authentication, a private network, or both.
:::
