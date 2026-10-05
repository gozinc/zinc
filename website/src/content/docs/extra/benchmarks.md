---
title: Benchmarks
description: How Zinc performs against Gin and Echo, and against the bare routers BunRouter and Chi, where it wins and loses, and how to run the suite yourself.
---

Zinc keeps its benchmark suite in the repository, so every performance claim can be checked against the code that produced it. Each framework does the same work through its own idiomatic API, in process, with no network in the way.

## Latest results

Measured on 2 October 2026 for 0.7.0: Apple M1 Pro, `go1.27.1`, commit `62512f0`, with every rival measured in the same run. Lower is better.

Zinc is scored in two tables. **Frameworks** is the headline: Gin and Echo, on all 93 scenarios. **Routers** puts Zinc against two bare routers, BunRouter and Chi, on the 66 routing scenarios they can run. A bare router does less per request than a framework, so the second table is a stricter test of routing alone.

A framework is fastest in a scenario when it beats every other by 3% or more. Closer than that is a tie: within one run's noise, and too small to matter to an application.

| Frameworks | Fastest in | Tied for fastest | Distance from the fastest |
|---|---|---|---|
| **Zinc** | **63 of 93** | **6** | **+5.7%** |
| Gin | 21 of 93 | 4 | +41.7% |
| Echo | 2 of 93 | 5 | +52.2% |

| Routers | Fastest in | Tied for fastest | Distance from the fastest |
|---|---|---|---|
| **Zinc** | **32 of 66** | **12** | **+7.7%** |
| BunRouter | 14 of 66 | 12 | +39.4% |
| Chi | 8 of 66 | 0 | +194.6% |

The distance is the geometric mean, over a table's scenarios, of how far each framework's median is above the fastest one in that scenario. 0% would mean fastest everywhere.

| Benchmark | Zinc | Gin | Echo |
|---|---:|---:|---:|
| Hello world | **80.62** | 135.1 | 151.3 |
| Route parameter, 10,000 distinct paths | **123.8** | 167.2 | 169.7 |
| GitHub API, Zipf-weighted traffic | **190.8** | 222.4 | 239.7 |
| API happy path | **1,099** | 2,978 | 1,681 |
| API behind production middleware | 3,230 | 4,375 | 3,321 |
| Parallel route parameter | **24.34** | 41.41 | 32.49 |
| Method not allowed, large route set | 124.4 | **80.94** | 186.1 |

Times are nanoseconds per operation. "API behind production middleware" runs each framework's own request ID, access log, panic recovery and CORS in front of a JSON route.

## What changed since 0.6.0

Zinc wins the same 63 of 93 against Gin and Echo, and its distance from the fastest moved from +5.2% to +5.7%. Two things cost time on purpose, and one scenario moved into the tie band:

- **Binding costs 3–5% more.** Since 0.6.2, a field tagged for a header, query or path parameter is never filled from the body, and `Bind().All` reads headers and cookies. A typed create request takes about 40 ns longer, a hand-written one about 50 ns: measured by interleaved A/B against 0.6.1.
- **API behind production middleware is now a tie.** Zinc's time is unchanged, 3,230 ns against 3,234 ns. Echo measured 2.8% behind it this run, just inside the 3% tie band; in 0.6.0 it was 3.0% behind, just outside.
- **Validation is built in since 0.7.0**, and costs nothing on a request whose struct has no `validate` tags: the plan is compiled once per type.

Every release from 0.6.1 to 0.7.0 was measured on its final commit and checked by A/B against the one before. Route-set build scenarios found one slowdown on the way, an app construction cost from the built-in docs page, which was fixed before release.

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
- **Not found.** Gin answers a plain 404 in 65 ns against Zinc's 104 ns.
- **Route registration.** Gin and Chi build route tables faster. Zinc's tree holds static routes too, which costs about half a microsecond per route, once, at startup.
- **Plain hits against BunRouter.** BunRouter serves hello world and static routes 30–40% faster, because it does less per request than a framework.
- **Static files.** Gin answers a missing file in 950 ns against Zinc's 1,117 ns. About 110 ns of that is Zinc's confined file open, which refuses symlinks that point outside the served directory.

The [full report](https://github.com/gozinc/zinc/blob/dev/BENCHMARKS.md) lists every scenario in both tables, every one Zinc loses, and every tie.

## Run it yourself

```bash
git clone https://github.com/gozinc/zinc
cd zinc/benchmarks
go run ./cmd/zincbench record
go run ./cmd/zincbench report latest
```

`record` runs every scenario ten times for each framework and saves the run under `benchmarks/results/`. `report` prints the tables above for that run. `compare` shows what changed between two runs. Results depend on the machine: record on a quiet machine on mains power, and compare runs from the same machine.

The separate [Gin routing-suite report](https://github.com/gozinc/zinc/blob/dev/GIN_BENCHMARK.md) covers the upstream router-focused suite, where every request repeats one URL. Zinc 0.5 is slower there than 0.4 was, because 0.4's route cache answered those repeated URLs from memory: the 20-parameter row went from 125 to 634 ns. Its scores are not part of these tables.
