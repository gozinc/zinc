# External routing benchmark: Zinc 0.3 recovery candidate

Run on 24 September 2026. Upstream suite: [`gin-gonic/go-http-routing-benchmark`](https://github.com/gin-gonic/go-http-routing-benchmark) at `ff3cdf55eccd0aa6a272991db9611a86c734dc51`. Zinc: local `codex/performance-combined` at `3d9109a`; the benchmarked production code was last changed at `d0070d4`. Go 1.27.1, darwin/arm64, Apple M1 Pro. The upstream fixtures and peer implementations were unchanged, except for the small filtered-run initialization fix in `adapter.patch.gz`.

The adapter in `zinc_test.go.sample` registers the upstream paths with `:parameter` converted to Zinc's `{parameter}` syntax. It uses `zinc.New()` defaults. The normal routing handlers intentionally write no body, as in the upstream suite. `ParamWrite` uses `io.WriteString(c.Writer(), c.Param("name"))`, matching the peers' direct-write approach. The extra `ParamWriteString` row uses Zinc's `Context.String` for a separate diagnostic and is excluded from peer win counts. The suite's mock writer creates a new header map on each `Header()` call, which particularly affects that extra row; it is not a realistic end-to-end HTTP measurement.

The full correctness test, including Zinc across every upstream API route, passed. The full upstream command with Zinc added passed all **210** benchmark functions (193 upstream plus 16 comparable Zinc rows and the one extra diagnostic):

```sh
GOTOOLCHAIN=go1.27.1 go test -count=1 ./...
GOTOOLCHAIN=go1.27.1 go test -run '^$' -bench=. -benchmem -timeout=20m -count=1 ./...
```

The complete run is in `full.log.gz`; `full-results.csv` contains all 210 measured rows. [Gin-style ranking tables](GIN_STYLE_TABLES.md) preserve the full page-style results, with Zinc added using this same-machine run. Zinc had **0 B/op and 0 allocs/op in every one of its 16 comparable rows**. It had the lowest time in **1/16** rows against all `net/http` implementations, or **2/16** against Gin, Echo, and Chi. Fiber is excluded from those rankings because the upstream suite runs it through a separate `fasthttp` harness. The 16-row count is specific to this external route-focused suite and must not be mixed with Zinc's separate 77-scenario result.

## Complete single-run comparison

All times are ns/op. “Fastest” considers every `net/http` implementation in this suite, including dedicated routers. `*All` measures one pass over the entire route fixture, not one HTTP request. The single-request rows and the aggregate rows therefore have different units of work.

| Workload | Zinc | Gin | Echo | Chi | Fastest `net/http` |
|---|---:|---:|---:|---:|---|
| Param | 55.29 | 30.87 | 26.35 | 552.30 | BunRouter (17.70) |
| Param5 | 70.64 | 59.33 | 69.11 | 740.50 | Gin (59.33) |
| Param20 | 125.00 | 174.10 | 203.40 | 3,091.00 | Zinc (125.00) |
| ParamWrite | 72.39 | 38.86 | 68.94 | 560.60 | BunRouter (36.09) |
| GithubStatic | 46.47 | 39.96 | 40.59 | 348.80 | HttpRouter (26.63) |
| GithubParam | 80.65 | 66.98 | 78.15 | 661.90 | Gin (66.98) |
| GithubAll | 19,095.00 | 13,463.00 | 15,937.00 | 133,837.00 | Gin (13,463.00) |
| GPlusStatic | 39.89 | 30.72 | 27.23 | 306.00 | BunRouter (12.66) |
| GPlusParam | 73.35 | 42.43 | 43.38 | 564.40 | BunRouter (25.37) |
| GPlus2Params | 83.79 | 55.85 | 61.24 | 621.80 | BunRouter (52.28) |
| GPlusAll | 1,244.00 | 602.40 | 645.60 | 7,532.00 | BunRouter (476.80) |
| ParseStatic | 40.16 | 30.89 | 27.87 | 308.80 | HttpRouter (15.54) |
| ParseParam | 67.20 | 37.16 | 35.16 | 556.40 | BunRouter (24.44) |
| Parse2Params | 73.68 | 43.10 | 44.34 | 602.30 | BunRouter (41.17) |
| ParseAll | 2,100.00 | 1,020.00 | 1,046.00 | 13,316.00 | BunRouter (812.80) |
| StaticAll | 8,010.00 | 9,736.00 | 9,982.00 | 61,326.00 | HttpRouter (5,666.00) |

## Repeated aggregate comparison

The four `*All` workloads were repeated ten times per framework at 100 ms per sample. Medians below are ns per complete fixture pass; all six displayed frameworks had ten samples in `aggregate-ten.log.gz` and `aggregate-medians.csv`.

| Fixture | Routes | Zinc | Gin | Echo | Chi | BunRouter | HttpRouter |
|---|---:|---:|---:|---:|---:|---:|---:|
| GitHubAll | 203 | 19,869.0 | 13,376.5 | 15,810.0 | 131,769.5 | 14,704.5 | 21,614.5 |
| GPlusAll | 13 | 1,265.0 | 597.7 | 645.05 | 7,508.5 | 477.95 | 962.6 |
| ParseAll | 26 | 2,142.5 | 1,015.5 | 1,052.0 | 13,160.5 | 811.2 | 1,417.5 |
| StaticAll | 157 | 8,157.0 | 9,721.0 | 9,953.5 | 60,932.5 | 8,544.5 | 5,696.5 |

Zinc was **48.5% slower than Gin** on the repeated GitHub aggregate, **111.6% slower than Gin** on GPlus, and **111.0% slower than Gin** on Parse. It was **16.1% faster than Gin** on StaticAll. The external suite is a useful routing diagnostic: Zinc's static dispatch and deep parameter matching are strong, while small mixed dynamic route sets remain slower than Gin/Echo. It does not replace the 77-scenario HTTP benchmark, where response writing and API behavior are part of the measured work.

Follow-up [route-cache experiments](CACHE_EXPERIMENTS.md) record a rejected cache bypass and the isolated promotion-threshold change now included in draft PR #72. The [interactive dashboard](../performance-recovery/benchmark-dashboard.html) reproduces this saved Gin-style snapshot and separately shows current-code paired Zinc checks. The [paired medians](dashboard-current-paired.json) and compressed raw outputs (`dashboard-current-paired-before.log.gz`, `dashboard-current-paired-after.log.gz`) are separate from the full-suite numbers above.

## Routing-structure memory

The upstream suite estimates heap retained after loading each fixture by forcing GC before and after construction. These single estimates are not RSS and were not repeated.

| Fixture | Zinc | Gin | Echo | Chi |
|---|---:|---:|---:|---:|
| GitHub | 102,440 B | 58,840 B | 117,912 B | 94,888 B |
| GPlus | 8,400 B | 4,576 B | 11,096 B | 8,008 B |
| Parse | 13,272 B | 7,896 B | 13,944 B | 9,656 B |
| Static | 39,376 B | 34,408 B | 92,104 B | 83,160 B |

To reproduce, clone the pinned upstream commit, apply `adapter.patch.gz` with `gzip -dc adapter.patch.gz | git apply`, copy `zinc_test.go.sample` into the benchmark checkout as `zinc_test.go`, and update the `replace github.com/0mjs/zinc` path in `go.mod` to the candidate worktree. For the filtered ten-sample run, use:

```sh
GOTOOLCHAIN=go1.27.1 go test -run '^$' -bench '^Benchmark(Gin|Echo|Chi|Zinc|BunRouter|HttpRouter)_(GithubAll|GPlusAll|ParseAll|StaticAll)$' -benchmem -benchtime=100ms -count=10 -timeout=20m ./...
```
