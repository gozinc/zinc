---
title: Benchmarks
description: How Zinc performs against Gin and Echo, and against the bare routers BunRouter and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Measured on 28 September 2026 for 0.5.2: Apple M1 Pro, `go1.27.1`, commit `78f5d3d`, with every rival measured in the same run. Lower is better.

Zinc is scored in two tables. **Frameworks** is the headline: Gin and Echo, on all 92 scenarios. **Routers** puts Zinc against two bare routers, BunRouter and Chi, on the 66 routing scenarios they can run. A bare router does less per request than a framework, so the second table is a stricter test of routing alone.

| Frameworks | Fastest in | Distance from the fastest |
|---|---|---|
| **Zinc** | **67 of 92** | **+5.0%** |
| Gin | 22 of 92 | +41.9% |
| Echo | 3 of 92 | +54.9% |

| Routers | Fastest in | Distance from the fastest |
|---|---|---|
| **Zinc** | **39 of 66** | **+6.2%** |
| BunRouter | 19 of 66 | +40.6% |
| Chi | 8 of 66 | +193.9% |

The distance is the geometric mean, over a table's scenarios, of how far each framework's median is above the fastest one in that scenario. 0% would mean fastest everywhere.

| Benchmark | Zinc | Gin | Echo |
|---|---:|---:|---:|
| Hello world | **79.16** | 135.1 | 150.8 |
| Route parameter, 10,000 distinct paths | **119.6** | 166.3 | 166.6 |
| GitHub API, Zipf-weighted traffic | **188.8** | 218.5 | 229.7 |
| API happy path | **1,095** | 2,941 | 1,670 |
| Parallel route parameter | **24.67** | 43.52 | 34.66 |
| Method not allowed, large route set | 122.2 | **78.92** | 185.2 |

Times are nanoseconds per operation.

## What changed since 0.4

The 0.4 suite scored Zinc at 61 of 77. The 0.5 suite is harder on Zinc on purpose, so the two scores can't be compared:

- **Every request pool holds 10,000 distinct paths.** The 0.4 suite repeated one URL per scenario, which is the best case for a per-path cache. Only Zinc had one, so it flattered Zinc. Scenarios that still repeat one URL are labelled `CacheBestCase`.
- **Before timing, each scenario checks that every framework returns the same response:** status, body, content type, `Allow` header and route parameters.
- **New scenarios:** Zipf-weighted traffic over real API route sets, a 1,024-service route set, and parallel traffic across route sets.
- **Two tables.** Chi moves from the headline to the routers table, beside BunRouter.

v0.4.0 scored 35 of 92 on this suite. Zinc 0.5.0 scored 62 of 92, after rebuilding the router around one route tree and removing the route cache, and 0.5.2 scores 67 of 92.

## Where Zinc is not fastest

- **Method not allowed (405).** Gin answers 405s up to 1.7 times as fast. Zinc walks the route tree once and collects every allowed method for the `Allow` header on the way.
- **Not found.** Gin answers a plain 404 in 63 ns against Zinc's 101 ns.
- **Route registration.** Gin and Chi build route tables faster. Zinc's tree holds static routes too, which costs about half a microsecond per route, once, at startup.
- **Plain hits against BunRouter.** BunRouter serves hello world and static routes 30–40% faster, because it does less per request than a framework.
- **Static files.** Gin answers a missing file in 947 ns against Zinc's 1,105 ns. About 110 ns of that is Zinc's confined file open, which refuses symlinks that point outside the served directory.

The [full report](https://github.com/0mjs/zinc/blob/dev/BENCHMARKS.md) lists every scenario in both tables, and every one Zinc loses.

## Run it yourself

```bash
git clone https://github.com/0mjs/zinc
cd zinc/benchmarks
go run ./cmd/zincbench record
go run ./cmd/zincbench report latest
```

`record` runs every scenario ten times for each framework and saves the run under `benchmarks/results/`. `report` prints the tables above for that run. `compare` shows what changed between two runs. Results depend on the machine: record on a quiet machine on mains power, and compare runs from the same machine.

The separate [Gin routing-suite report](https://github.com/0mjs/zinc/blob/dev/GIN_BENCHMARK.md) covers the upstream router-focused suite, where every request repeats one URL. Zinc 0.5 is slower there than 0.4 was, because 0.4's route cache answered those repeated URLs from memory: the 20-parameter row went from 125 to 634 ns. Its scores are not part of these tables.
