---
title: Benchmarks
description: How Zinc performs against Gin, Echo, and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Latest Zinc run: 25 September 2026, Apple M1 Pro, `go1.27.1`, commit `42e11d2`. Gin, Echo, and Chi samples are reused from 24 September. These mixed-date figures are an indicative snapshot; close results need a contemporaneous rerun. Lower is better.

| Framework | Fastest in |
|---|---|
| **Zinc** | **60 of 77** |
| Gin | 12 of 77 |
| Chi | 3 of 77 |
| Echo | 2 of 77 |

Zinc was fastest, or within 2% of the fastest, in 64 rows. The 77-scenario score is a useful comparison, not a release threshold. Common string-response routing paths allocate 16 B and one object per request with the hardened response-header ownership.

| Benchmark | Zinc | Gin | Echo | Chi |
|---|---:|---:|---:|---:|
| Hello world | **86.75** | 136.9 | 152.9 | 180.8 |
| Route parameter | **114.2** | 143.9 | 159.1 | 348.1 |
| Middleware chain | **242.8** | 522.5 | 326.2 | 893.2 |
| JSON response | **568.2** | 596.3 | 577.2 | 665.8 |
| API happy path | **1,078.5** | 3,292 | 1,548.5 | 1,912.5 |
| Parallel route parameter | **40.05** | 55.91 | 49.04 | 301.5 |

Times are nanoseconds per operation.

## Where Zinc is not fastest

- **Not found and method mismatch.** Gin answers several misses faster, including `NotFound` at 60.03 ns against Zinc's 107.7 ns. The large-route-set method mismatch is 91.81 ns for Gin against 106.4 ns for Zinc.
- **Route registration.** Gin wins `RouteRegistrationParam` by 10%; Chi wins two scenario route-set builds. Registration happens at startup.
- **Query parameters.** Echo wins `QueryParams` by 3.2%.
- **Static files.** Gin answers a missing file at 1,022 ns against Zinc's 1,587.5 ns; Chi is narrowly faster on a file hit.

## Run it yourself

```bash
git clone https://github.com/0mjs/zinc
cd zinc/benchmarks
GOTOOLCHAIN=go1.27.1 go test -run='^$' -bench=. -benchmem -count=10 -benchtime=100ms | tee results.txt
```

Results depend on the machine. Compare runs with [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) rather than trusting a single sample.

The [full report](https://github.com/0mjs/zinc/blob/dev/BENCHMARKS.md) lists all 77 rows, including the percentage Zinc trails the winner in each loss. The separate [Gin routing-suite report](https://github.com/0mjs/zinc/blob/dev/GIN_BENCHMARK.md) covers 16 router-focused workloads measured together on 25 September; its scores are not part of the 77.
