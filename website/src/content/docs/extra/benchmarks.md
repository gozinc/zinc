---
title: Benchmarks
description: How Zinc performs against Gin, Echo, and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Measured for the 0.4.0 release on 26 September 2026: Apple M1 Pro, `go1.27.1`, commit `5d59449`, with Gin, Echo, and Chi measured in the same run. Lower is better.

| Framework | Fastest in |
|---|---|
| **Zinc** | **61 of 77** |
| Gin | 11 of 77 |
| Chi | 4 of 77 |
| Echo | 1 of 77 |

Zinc was fastest, or within 2% of the fastest, in 65 rows. The 77-scenario score is a useful comparison, not a release threshold. Common string-response routing paths allocate 16 B and one object per request.

| Benchmark | Zinc | Gin | Echo | Chi |
|---|---:|---:|---:|---:|
| Hello world | **80.06** | 135.2 | 148.5 | 175.1 |
| Route parameter | **105.8** | 139.1 | 155.3 | 304 |
| Middleware chain | **236.5** | 492.9 | 316.3 | 864.5 |
| JSON response | **544.9** | 565 | 556 | 617.6 |
| API happy path | **985.9** | 3,093 | 1,428.5 | 1,718 |
| Parallel route parameter | **28.84** | 53.63 | 44.09 | 297.5 |

Times are nanoseconds per operation.

## Where Zinc is not fastest

- **Not found and method mismatch.** Gin answers several misses faster, including `NotFound` at 52.64 ns against Zinc's 100.2 ns. The large-route-set method mismatch is 86.28 ns for Gin against 101.2 ns for Zinc.
- **Route registration.** Gin wins `RouteRegistrationParam` by 11%; Chi wins 3 scenario route-set builds. Registration happens at startup.
- **Query parameters.** Echo wins `QueryParams` by 1.6%.
- **Static files.** Gin answers a missing file at 970.4 ns against Zinc's 1,503.5 ns; Chi is narrowly faster on a file hit.

## Run it yourself

```bash
git clone https://github.com/0mjs/zinc
cd zinc/benchmarks
GOTOOLCHAIN=go1.27.1 go test -run='^$' -bench=. -benchmem -count=10 -benchtime=100ms | tee results.txt
```

Results depend on the machine. Compare runs with [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) rather than trusting a single sample.

The [full report](https://github.com/0mjs/zinc/blob/dev/BENCHMARKS.md) lists all 77 rows, including the percentage Zinc trails the winner in each loss. The separate [Gin routing-suite report](https://github.com/0mjs/zinc/blob/dev/GIN_BENCHMARK.md) covers 16 router-focused workloads measured together on 25 September; its scores are not part of the 77.
