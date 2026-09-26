---
title: Benchmarks
description: How Zinc performs against Gin, Echo, and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Measured for the 0.4.0 release on 26 September 2026: Apple M1 Pro, `go1.27.1`, commit `d836005`, with Gin, Echo, and Chi measured in the same run. Lower is better.

| Framework | Fastest in |
|---|---|
| **Zinc** | **58 of 77** |
| Gin | 12 of 77 |
| Chi | 5 of 77 |
| Echo | 2 of 77 |

Zinc was fastest, or within 2% of the fastest, in 63 rows. The 77-scenario score is a useful comparison, not a release threshold. Common string-response routing paths allocate 16 B and one object per request.

| Benchmark | Zinc | Gin | Echo | Chi |
|---|---:|---:|---:|---:|
| Hello world | **98.76** | 139.9 | 155.3 | 203.6 |
| Route parameter | **115.1** | 144.8 | 171.2 | 379.7 |
| Middleware chain | **300.1** | 558.6 | 338.8 | 1,036.5 |
| JSON response | **588** | 619.4 | 643.1 | 745.8 |
| API happy path | **1,122** | 3,589.5 | 1,671.5 | 2,239.5 |
| Parallel route parameter | **39.74** | 55.5 | 50.3 | 295.4 |

Times are nanoseconds per operation.

## Where Zinc is not fastest

- **Not found and method mismatch.** Gin answers several misses faster, including `NotFound` at 73.62 ns against Zinc's 109.2 ns. The large-route-set method mismatch is 91.94 ns for Gin against 110.8 ns for Zinc.
- **Route registration.** Gin wins `RouteRegistrationParam` by 16%; Chi wins 4 scenario route-set builds. Registration happens at startup.
- **Query parameters.** Echo wins `QueryParams` by 4.6%.
- **Static files.** Gin answers a missing file at 1,008 ns against Zinc's 1,610.5 ns; Chi is narrowly faster on a file hit.

## Run it yourself

```bash
git clone https://github.com/0mjs/zinc
cd zinc/benchmarks
GOTOOLCHAIN=go1.27.1 go test -run='^$' -bench=. -benchmem -count=10 -benchtime=100ms | tee results.txt
```

Results depend on the machine. Compare runs with [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) rather than trusting a single sample.

The [full report](https://github.com/0mjs/zinc/blob/dev/BENCHMARKS.md) lists all 77 rows, including the percentage Zinc trails the winner in each loss. The separate [Gin routing-suite report](https://github.com/0mjs/zinc/blob/dev/GIN_BENCHMARK.md) covers 16 router-focused workloads measured together on 25 September; its scores are not part of the 77.
