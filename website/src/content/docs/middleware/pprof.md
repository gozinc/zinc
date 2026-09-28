---
title: pprof
description: Serve Go's runtime profiler behind authentication, to see where a running service spends CPU and memory.
---

pprof serves Go's `net/http/pprof` endpoints, so you can point `go tool pprof` at a running service. Add it when you need to find out where a live service spends CPU or memory, or why goroutines pile up. Always put authentication in front of it.

## Usage

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

```bash
curl -i http://localhost:8080/debug/pprof/
# HTTP/1.1 401 Unauthorized
# Www-Authenticate: Basic realm="Restricted"

curl -u "ops:$PPROF_PASSWORD" 'http://localhost:8080/debug/pprof/goroutine?debug=1'
# goroutine profile: total 2
# ...
```

`UsePrefix` middleware runs before routing, in the order given, so every request under `/debug/pprof` is authenticated before it reaches the profiler.

Then, from your machine, record a 30-second CPU profile and open it in the browser:

```bash
go tool pprof -http=:0 "https://ops:$PPROF_PASSWORD@api.example.com/debug/pprof/profile?seconds=30"
```

:::danger[Never expose pprof publicly]
Profiles reveal your code's internals and command line, and a CPU profile or trace slows the server while it runs. Don't add `pprof.New()` with plain `app.Use` on an internet-facing app. Put authentication, a private network, or both, in front of it.
:::

## Defaults

| Setting | Default |
|---|---|
| Prefix | `/debug/pprof` (`pprof.DefaultPrefix`) |

## Configuration

Serve the endpoints under another path:

```go
app.UsePrefix("/internal/pprof",
	requireAdmin, // your auth middleware
	pprof.New(pprof.Config{Prefix: "/internal/pprof"}),
)
```

| Field | Default | Meaning |
|---|---|---|
| `Prefix` | `/debug/pprof` | Path the endpoints are served under. A trailing slash is ignored and a missing leading slash is added. |

The endpoints are the standard ones: the index at the prefix, `/cmdline`, `/profile`, `/symbol`, `/trace`, and each named profile such as `/heap`, `/goroutine`, `/allocs` and `/block`. Any other request passes down the chain.

## Security

- Check that `PPROF_PASSWORD` is set before you start the server. `basicauth.Static` compares against whatever it's given, so an empty variable means an empty password.
- The endpoints answer any method, not only `GET`.
- `/cmdline` returns the process's command line, including any flags or secrets passed on it.

## Related

- [Basic Auth](/middleware/basicauth/) protects the endpoints with a username and password.
- [Middleware](/guide/groups-and-middleware/) covers `UsePrefix` and when it runs.
- [Prometheus](/middleware/prometheus/) exports request metrics for ongoing monitoring.
