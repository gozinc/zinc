---
title: pprof
description: Expose Go's runtime profiler behind authentication for CPU, memory, and goroutine analysis.
---

`pprof` serves the standard `net/http/pprof` handlers, so `go tool pprof` can profile a running service. It answers requests below `/debug/pprof` and passes everything else through.

Profiles reveal internals and can slow a server down, so always put authentication in front:

```go
import (
	"github.com/0mjs/zinc/middleware/basicauth"
	"github.com/0mjs/zinc/middleware/pprof"
)

app.UsePrefix("/debug/pprof",
	basicauth.New(basicauth.Config{Validator: basicauth.Static("ops", os.Getenv("PPROF_PASSWORD"))}),
	pprof.New(),
)
```

`UsePrefix` runs before routing, in order, so every profiling request is authenticated first.

Then, from your machine:

```bash
go tool pprof -http=:0 "https://ops:$PPROF_PASSWORD@api.example.com/debug/pprof/profile?seconds=30"
```

## Use another path

```go
app.UsePrefix("/internal/pprof", requireAdmin, pprof.New(pprof.Config{Prefix: "/internal/pprof"}))
```

:::danger[Never expose pprof publicly]
Do not register `pprof.New()` with plain `app.Use` on an internet-facing app. Protect it with authentication, a private network, or both.
:::
