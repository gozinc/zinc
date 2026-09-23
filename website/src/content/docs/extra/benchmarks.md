---
title: Benchmarks
description: How Zinc performs against Gin, Echo, and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Apple M1 Pro, `go1.26.1`, Zinc commit `5f77c75`. Lower is better.

| Framework | Fastest in |
|---|---|
| **Zinc** | **62 of 77** |
| Gin | 10 of 77 |
| Chi | 5 of 77 |
| Echo | 0 of 77 |

Zinc was fastest, or within 2% of the fastest, in 63 rows. Static, parameter, and not-found routing allocate nothing per request.

| Benchmark | Zinc | Gin | Echo | Chi |
|---|---:|---:|---:|---:|
| Hello world | **65.27** | 89.27 | 138.2 | 198.9 |
| Route parameter | **80.65** | 94.61 | 132.4 | 345.9 |
| Middleware chain | **365.1** | 424.2 | 540.5 | 950.0 |
| JSON response | **322.6** | 394.5 | 387.9 | 508.2 |
| API request, bind and respond | **1,186** | 3,024 | 2,287 | 1,737 |
| Parallel route parameter | **13.68** | 53.86 | 56.11 | 315.4 |

Times are nanoseconds per operation.

## Where Zinc is not fastest

- **Not found and method mismatch.** Gin answers routing misses faster on several route sets, for example `NotFound` at 58.46 ns against Zinc's 72.18 ns.
- **Route registration.** Chi builds large route tables faster in most realistic scenarios. This happens once at startup.
- **Rejecting invalid JSON.** Gin is about 9% faster (694 ns against 761 ns). Multipart uploads are within 1% across all four frameworks.
- **Missing static files.** Gin answers a static-file miss faster (1,031 ns against 1,536 ns).

## Run it yourself

```bash
git clone https://github.com/0mjs/zinc
cd zinc/benchmarks
go test -run='^$' -bench=. -benchmem -count=10 | tee results.txt
```

Results depend on the machine. Compare runs with [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) rather than trusting a single sample.

The [full report](https://github.com/0mjs/zinc/blob/dev/BENCHMARKS.md) lists all 77 comparable rows and the scorecard.
