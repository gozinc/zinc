---
title: Benchmarks
description: How Zinc performs against Gin and Echo, and against the bare routers BunRouter and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Measured on 27 September 2026 for 0.5.0: Apple M1 Pro, `go1.27.1`, commit `8271438`, with every rival measured in the same run. Lower is better.

Zinc is scored in two tables. **Frameworks** is the headline: Gin and Echo, on all 92 scenarios. **Routers** puts Zinc against two bare routers, BunRouter and Chi, on the 66 routing scenarios they can run. A bare router does less per request than a framework, so the second table is a stricter test of routing alone.

| Frameworks | Fastest in | Distance from the fastest |
|---|---|---|
| **Zinc** | **64 of 92** | **+6.5%** |
| Gin | 24 of 92 | +40.4% |
| Echo | 4 of 92 | +53.7% |

| Routers | Fastest in | Distance from the fastest |
|---|---|---|
| **Zinc** | **36 of 66** | **+7.0%** |
| BunRouter | 22 of 66 | +40.3% |
| Chi | 8 of 66 | +197.5% |

The distance is the geometric mean, over a table's scenarios, of how far each framework's median is above the fastest one in that scenario. 0% would mean fastest everywhere.

| Benchmark | Zinc | Gin | Echo |
|---|---:|---:|---:|
| Hello world | **83.93** | 136.6 | 149.4 |
| Route parameter, 10,000 distinct paths | **122.3** | 166.1 | 167.0 |
| GitHub API, Zipf-weighted traffic | **201.9** | 216.5 | 233.9 |
| API happy path | **1,098** | 2,972 | 1,738 |
| Parallel route parameter | **25.09** | 41.23 | 32.70 |
| Method not allowed, large route set | 168.7 | **79.11** | 187.0 |

Times are nanoseconds per operation.

## What changed since 0.4

The 0.4 suite scored Zinc at 61 of 77. The 0.5 suite is harder on Zinc on purpose, so the two scores can't be compared:

- **Every request pool holds 10,000 distinct paths.** The 0.4 suite repeated one URL per scenario, which is the best case for a per-path cache. Only Zinc had one, so it flattered Zinc. Scenarios that still repeat one URL are labelled `CacheBestCase`.
- **Before timing, each scenario checks that every framework returns the same response:** status, body, content type, `Allow` header and route parameters.
- **New scenarios:** Zipf-weighted traffic over real API route sets, a 1,024-service route set, and parallel traffic across route sets.
- **Two tables.** Chi moves from the headline to the routers table, beside BunRouter.

v0.4.0 scored 35 of 92 on this suite. Zinc 0.5 scores 64 of 92, after rebuilding the router around one route tree and removing the route cache.

## Where Zinc is not fastest

- **Method not allowed (405).** Gin answers 405s up to twice as fast. Zinc walks the route tree once and collects every allowed method for the `Allow` header on the way.
- **Not found.** Gin answers a plain 404 in 62 ns against Zinc's 100 ns.
- **Route registration.** Gin and Chi build route tables faster. Zinc's tree holds static routes too, which costs about half a microsecond per route, once, at startup.
- **Plain hits against BunRouter.** BunRouter serves hello world and static routes 30–45% faster, because it does less per request than a framework.
- **Static files.** Gin answers a missing file in 955 ns against Zinc's 1,156 ns. About 110 ns of that is Zinc's confined file open, which refuses symlinks that point outside the served directory.

The [full report](https://github.com/0mjs/zinc/blob/dev/BENCHMARKS.md) lists every scenario in both tables, and every one Zinc loses.

## Run it yourself

```bash
git clone https://github.com/0mjs/zinc
cd zinc/benchmarks
go run ./cmd/zincbench record
go run ./cmd/zincbench report latest
```

`record` runs every scenario ten times for each framework and saves the run under `benchmarks/results/`. `report` prints the tables above for that run. `compare` shows what changed between two runs. Results depend on the machine: record on a quiet machine on mains power, and compare runs from the same machine.

The separate [Gin routing-suite report](https://github.com/0mjs/zinc/blob/dev/GIN_BENCHMARK.md) covers the upstream router-focused suite; its scores are not part of these tables.
