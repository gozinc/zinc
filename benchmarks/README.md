# Benchmark records

`make bench-dash` opens the local head-to-head run history. Every run has its own raw log and JSON record in `benchmarks/results/runs/`. The dashboard now groups runs by their intended Zinc release. These files are gitignored, so each checkout or worktree has a separate local results directory unless you copy or restore the records.

Record a complete run of every framework for a release:

```sh
make bench-record RELEASE=0.5.0 NOTE="baseline"
make bench-dash
```

## Suite v2

The 0.5 suite has 92 head-to-head scenarios. It was rebuilt so no framework's result is flattered, Zinc's included. The audit behind it is `audits/2026-09-26/suite-v2-audit.md` (local).

- **Request pools.** Parameter, 404, 405 and wildcard scenarios cycle through 10,000 distinct paths, ten times Zinc's route cache. In the 0.4 suite almost every scenario repeated one URL, so every one of Zinc's lookups was a cache hit. The pools are built before timing, are deterministic, and are identical for every framework. `CacheBestCase/*` keeps three one-URL requests, labelled as the cache's best case.
- **Proofs.** Before timing, every scenario checks that its frameworks agree on status, body (decoded if JSON), media type, `Allow` and the parameters they read. A framework can't look faster by doing less.
- **Equal work.** Gin's binding validator is off, since Zinc and Echo validate only when asked. Echo binds the query on POST explicitly. Chi and BunRouter set `Content-Type` as the others do.
- **Normalised misses.** Routing scenarios answer a 404 with the same text and a 405 with the status and `Allow` for every framework. `NotFound` and `StaticFileNotFound` keep each framework's default response.
- **BunRouter** runs the routing scenarios. It answers a wrong method on a route with a parameter followed by more segments with 404 rather than 405, so it sits out those 405 scenarios.
- **New scenarios:** `ScenarioRouteSetTraffic` (Zipf-weighted routes with distinct parameters), a 1,024-route `Services1024` corpus, and `ParallelRouteSetTraffic`.

## Two scoreboards

Zinc is scored against two groups separately:

- **Frameworks** (Gin and Echo) on every scenario. This is the headline figure.
- **Routers** (BunRouter and Chi) on the routing scenarios they run.

Each scoreboard shows two numbers: how many scenarios Zinc is fastest in, and the geometric mean of how far Zinc's median is above the fastest median in each scenario. The second number doesn't treat a 0.1% win and a 60% loss as equal, so it shows the size of the gaps as well as the count. Records made before 0.5 are scored the same way, so their headline no longer counts Chi. Fiber is not measured: it runs on fasthttp, not net/http.

## Checking a change

Rivals don't change between Zinc commits, so a change only needs Zinc remeasured. `bench-zinc` runs the Zinc case of all 77 scenarios, plus the Zinc-only `BenchmarkAPI04*` benchmarks, in about three minutes, and reuses the rival samples from the pinned baseline. The record keeps the source run's ID and is labelled as a mixed-date comparison.

```sh
make bench-zinc RELEASE=0.4.0 NOTE="router: cache promotion at 8"
make bench-compare
```

`bench-compare` compares the latest run with the baseline. It lists both scoreboards, then changes in allocations, bytes, time and headline wins, and exits non-zero if any benchmark allocates more. A scenario is flagged as slower when it loses more than 5% and more than 15 ns. Parallel, registration and route-set build benchmarks, which vary by up to 12% between runs of identical code, get a 12% margin. Use `ARGS='-all'` to list every scenario.

Pin a Zinc-only run as the baseline when checking Zinc-only runs. Identical code measures faster in Zinc-only mode than in a four-framework run, so mixing the two modes shows differences that aren't real.

Confirm a flagged scenario before acting on it:

```sh
make bench-ab SCENARIOS=LargeRouteSetParam,API04ParamInt
```

`bench-ab` checks out the baseline commit in the user cache directory (`zincbench/ab/` under `os.UserCacheDir`, outside the repository), then measures that tree and the working tree in alternating rounds. It reports "slower" or "faster" only when the interquartile ranges of the two sample sets don't overlap. The baseline commit must contain any Zinc-only benchmark you name.

## Profiling a scenario

```sh
make bench-profile SCENARIO=ScenarioRouteSetAll/GitHubAPI203
```

`bench-profile` runs Zinc's case of one scenario with the CPU and memory profilers, prints the top of each, and keeps the profiles and test binary in `benchmarks/results/profiles/` for `go tool pprof -http=:`.

## Importing Zinc-only logs

When only Zinc was rerun by hand, import its logs against a saved four-framework run. The new record keeps the rival source ID and the dashboard labels it as a mixed-date comparison:

```sh
cd benchmarks
go run ./cmd/zincbench import-zinc \
  -logs results/four-paths/json-happy-c-direct.log,results/four-paths/json-happy-c-nested.log \
  -rivals 20260924-210000-c6fd47f -commit 42e11d2 \
  -date 2026-09-25T00:11:00Z -release 0.3.0 \
  -note "Four-path final; Zinc Sep 25, rivals Sep 24" -go go1.27.1
```

The 0.3.0 history includes this 60/77 run. Its standalone visual report is `audits/2026-09-25/zinc-performance.html`; the independent Gin routing suite is in `benchmarks/results/gin-20260925/` and documented in `GIN_BENCHMARK.md`.

Bundle a release before removing a worktree or moving to another machine. The zip contains every run's JSON and raw log, with optional reports and independent suite data:

```sh
make bench-bundle RELEASE=0.3.0 ARGS='-include audits/2026-09-25/zinc-performance.html,benchmarks/results/four-paths,benchmarks/results/gin-20260925,BENCHMARKS.md,GIN_BENCHMARK.md'
```

The archive is written to `benchmarks/results/bundles/0.3.0.zip` by default. It is also gitignored; copy it to durable storage. To restore it in another checkout, extract the archive's `runs/` files into that checkout's `benchmarks/results/runs/`, then run `make bench-dash`. The `extras/` directory preserves included files under their original repository-relative paths.
