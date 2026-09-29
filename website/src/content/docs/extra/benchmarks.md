---
title: Benchmarks
description: How Zinc performs against Gin and Echo, and against the bare routers BunRouter and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Measured on 29 September 2026 for 0.6.0: Apple M1 Pro, `go1.27.1`, commit `93d9975`, with every rival measured in the same run. Lower is better.

Zinc is scored in two tables. **Frameworks** is the headline: Gin and Echo, on all 93 scenarios. **Routers** puts Zinc against two bare routers, BunRouter and Chi, on the 66 routing scenarios they can run. A bare router does less per request than a framework, so the second table is a stricter test of routing alone.

A framework is fastest in a scenario when it beats every other by 3% or more. Closer than that is a tie: within one run's noise, and too small to matter to an application.

| Frameworks | Fastest in | Tied for fastest | Distance from the fastest |
|---|---|---|---|
| **Zinc** | **63 of 93** | **9** | **+5.2%** |
| Gin | 19 of 93 | 7 | +43.5% |
| Echo | 2 of 93 | 4 | +55.7% |

| Routers | Fastest in | Tied for fastest | Distance from the fastest |
|---|---|---|---|
| **Zinc** | **32 of 66** | **11** | **+6.8%** |
| BunRouter | 15 of 66 | 11 | +41.4% |
| Chi | 8 of 66 | 0 | +204.4% |

The distance is the geometric mean, over a table's scenarios, of how far each framework's median is above the fastest one in that scenario. 0% would mean fastest everywhere.

| Benchmark | Zinc | Gin | Echo |
|---|---:|---:|---:|
| Hello world | **80.62** | 138.8 | 154.6 |
| Route parameter, 10,000 distinct paths | **121.0** | 167.2 | 167.2 |
| GitHub API, Zipf-weighted traffic | **191.1** | 218.4 | 231.7 |
| API happy path | **1,094** | 2,942 | 1,701 |
| API behind production middleware | **3,234** | 4,322 | 3,332 |
| Parallel route parameter | **22.88** | 41.76 | 34.21 |
| Method not allowed, large route set | 121.2 | **80.51** | 186.4 |

Times are nanoseconds per operation. "API behind production middleware" runs each framework's own request ID, access log, panic recovery and CORS in front of a JSON route.

## What changed since 0.5

0.5.2 scored 67 of 92. The two scores can't be compared directly:

- **Ties.** Before 0.6, the lower median won however small the gap, so a 0.2% lead counted as much as a 50% one. Now a gap under 3% is a tie. On 0.5.2's own run, that turns 67 wins into 63 wins and 8 ties.
- **A new scenario.** The API route behind a production middleware stack. Zinc wins it by 3%, just outside the tie band.

Zinc's own speed didn't change between 0.5.2 and 0.6.0: the release runs agree within noise.

## What changed since 0.4

The 0.4 suite scored Zinc at 61 of 77. The 0.5 suite is harder on Zinc on purpose, so the two scores can't be compared:

- **Every request pool holds 10,000 distinct paths.** The 0.4 suite repeated one URL per scenario, which is the best case for a per-path cache. Only Zinc had one, so it flattered Zinc. Scenarios that still repeat one URL are labelled `CacheBestCase`.
- **Before timing, each scenario checks that every framework returns the same response:** status, body, content type, `Allow` header and route parameters.
- **New scenarios:** Zipf-weighted traffic over real API route sets, a 1,024-service route set, and parallel traffic across route sets.
- **Two tables.** Chi moves from the headline to the routers table, beside BunRouter.

v0.4.0 scored 35 of 92 on this suite. Zinc 0.5.0 scored 62 of 92, after rebuilding the router around one route tree and removing the route cache.

## Where Zinc is not fastest

- **Method not allowed (405).** Gin answers 405s up to 1.6 times as fast. Zinc walks the route tree once and collects every allowed method for the `Allow` header on the way.
- **Not found.** Gin answers a plain 404 in 58 ns against Zinc's 102 ns.
- **Route registration.** Gin and Chi build route tables faster. Zinc's tree holds static routes too, which costs about half a microsecond per route, once, at startup.
- **Plain hits against BunRouter.** BunRouter serves hello world and static routes 30–40% faster, because it does less per request than a framework.
- **Static files.** Gin answers a missing file in 960 ns against Zinc's 1,100 ns. About 110 ns of that is Zinc's confined file open, which refuses symlinks that point outside the served directory.

The [full report](https://github.com/0mjs/zinc/blob/dev/BENCHMARKS.md) lists every scenario in both tables, every one Zinc loses, and every tie.

## Run it yourself

```bash
git clone https://github.com/0mjs/zinc
cd zinc/benchmarks
go run ./cmd/zincbench record
go run ./cmd/zincbench report latest
```

`record` runs every scenario ten times for each framework and saves the run under `benchmarks/results/`. `report` prints the tables above for that run. `compare` shows what changed between two runs. Results depend on the machine: record on a quiet machine on mains power, and compare runs from the same machine.

The separate [Gin routing-suite report](https://github.com/0mjs/zinc/blob/dev/GIN_BENCHMARK.md) covers the upstream router-focused suite, where every request repeats one URL. Zinc 0.5 is slower there than 0.4 was, because 0.4's route cache answered those repeated URLs from memory: the 20-parameter row went from 125 to 634 ns. Its scores are not part of these tables.
