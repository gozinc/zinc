# Zinc Benchmarks

This report compares equivalent in-process Zinc, Gin, Echo, and Chi workloads through each framework's idiomatic API. Lower latency is better.

> **Mixed-date comparison.** Zinc was measured on 25 September 2026 with the code in `42e11d2`. Gin, Echo, and Chi samples are reused unchanged from the 24 September run `20260924-210000-c6fd47f`. The win count is an indicative snapshot, not a contemporaneous full-suite rerun or a release gate. Close results should be rerun together before drawing strong conclusions.

## Run information

- Date: `2026-09-25`
- Zinc commit: `42e11d2`
- Go: `go1.27.1`
- Machine: `Apple M1 Pro`
- OS / architecture: `darwin / arm64`
- Zinc samples: `10 × 100ms` per scenario
- Rival samples: `10 × 100ms` per scenario, saved 24 September 2026
- Measurement: median `ns/op` across the ten samples

## Summary

| Framework | Lowest median latency |
| --- | ---: |
| Zinc | 60 / 77 |
| Gin | 12 / 77 |
| Echo | 2 / 77 |
| Chi | 3 / 77 |

Zinc has the lowest median in **60 of 77** scenarios, and is fastest or within 2% of the fastest in **64**. Common string-response routing paths now measure 16 B and 1 allocation per request because response headers are request-owned.

## Results

All values are median ns/op. The winner is the framework with the lowest median in that row.

| Benchmark | Zinc | Gin | Echo | Chi | Winner |
| --- | ---: | ---: | ---: | ---: | --- |
| `HelloWorld` | 86.75 | 136.9 | 152.9 | 180.8 | Zinc |
| `StaticRoute` | 85.48 | 137.3 | 155.9 | 181.7 | Zinc |
| `StaticRouteCold` | 88.06 | 163.8 | 184.5 | 241.9 | Zinc |
| `RouterParam` | 114.2 | 143.9 | 159.1 | 348.1 | Zinc |
| `RouterParamCold` | 136.4 | 147.1 | 161.6 | 372.6 | Zinc |
| `JSONResponse` | 568.2 | 596.3 | 577.2 | 665.8 | Zinc |
| `QueryParams` | 331 | 484.5 | 320.8 | 535 | Echo |
| `MiddlewareChain` | 242.8 | 522.5 | 326.2 | 893.2 | Zinc |
| `NotFound` | 107.7 | 60.03 | 762 | 306.6 | Gin |
| `LargeRouteSetStatic` | 85.19 | 152.6 | 173.4 | 216.8 | Zinc |
| `LargeRouteSetStaticMixed` | 89.04 | 170.9 | 189.4 | 265.9 | Zinc |
| `LargeRouteSetNotFound` | 105.8 | 62.53 | 783.9 | 322.3 | Gin |
| `LargeRouteSetMethodMismatch` | 106.4 | 91.81 | 915.7 | 304.2 | Gin |
| `LargeRouteSetParam` | 112.8 | 158.9 | 180.5 | 393.5 | Zinc |
| `LargeRouteSetParamMixed` | 136.8 | 178.5 | 198.9 | 437.1 | Zinc |
| `RouteRegistrationStatic` | 59,596 | 69,928 | 347,314 | 87,378 | Zinc |
| `RouteRegistrationParam` | 52,483 | 47,693 | 174,534 | 67,516 | Gin |
| `APIParamQueryJSON` | 837.2 | 2,865 | 1,341 | 1,119 | Zinc |
| `APIHappyPath` | 1,078.5 | 3,292 | 1,548.5 | 1,912.5 | Zinc |
| `APIBindJSONHappyPath` | 1,720.5 | 4,368.5 | 1,872 | 2,902.5 | Zinc |
| `APIBindHeaderQueryJSON` | 1,913.5 | 5,662.5 | 2,159 | 2,569.5 | Zinc |
| `APIBindInvalidJSON` | 940.5 | 974 | 1,129.5 | 1,118 | Zinc |
| `APIBindValidationFailure` | 524.2 | 748.5 | 733.4 | 820.6 | Zinc |
| `APIBindMultipartHappyPath` | 11,598 | 11,478 | 11,481 | 11,818 | Gin |
| `LargeJSONResponse` | 42,894 | 47,096 | 42,770 | 42,936 | Echo |
| `LargeJSONBind` | 135,896 | 174,716 | 171,817 | 172,838 | Zinc |
| `StaticFileHit` | 15,050 | 24,973 | 28,830 | 14,933 | Chi |
| `StaticFileNotFound` | 1,587.5 | 1,022 | 3,106 | 1,978.5 | Gin |
| `NestedGroupMiddlewareAPI` | 1,107.5 | 3,447.5 | 1,633 | 2,849 | Zinc |
| `APIUnauthorizedReject` | 51.25 | 50.7 | 57.83 | 187.6 | Gin |
| `ParallelStaticRoute` | 26.5 | 49.66 | 47.38 | 157.8 | Zinc |
| `ParallelRouterParam` | 40.05 | 55.91 | 49.04 | 301.5 | Zinc |
| `ParallelMiddlewareChain` | 66.2 | 197.9 | 90.09 | 896.6 | Zinc |
| `ParallelAPIHappyPath` | 173.6 | 841.7 | 390.1 | 1,298 | Zinc |
| `Param5` | 167.7 | 202.9 | 242.3 | 556.7 | Zinc |
| `Param10` | 293.1 | 371.4 | 406.4 | 1,237.5 | Zinc |
| `NestedGroupStatic` | 86.48 | 241.6 | 172.8 | 853.9 | Zinc |
| `NestedGroupParam` | 169.2 | 189.5 | 215 | 1,149.5 | Zinc |
| `NestedGroupNotFound` | 101.2 | 171.8 | 199.9 | 1,042.5 | Zinc |
| `NestedGroupMethodMismatch` | 176.9 | 203.8 | 315.6 | 1,088.5 | Zinc |
| `WildcardTail` | 115.9 | 154.1 | 162.2 | 380.1 | Zinc |
| `WildcardTailNotFound` | 96.56 | 153.6 | 158.6 | 189 | Zinc |
| `ScenarioRouteSetBuild/Static157` | 41,354 | 72,496 | 183,624 | 61,902 | Zinc |
| `ScenarioRouteSetBuild/GitHubAPI203` | 101,180 | 121,274 | 326,010 | 107,420 | Zinc |
| `ScenarioRouteSetBuild/GPlusAPI13` | 6,352 | 7,547.5 | 32,415 | 6,902 | Zinc |
| `ScenarioRouteSetBuild/ParseAPI26` | 10,622 | 12,643 | 39,426 | 10,304 | Chi |
| `ScenarioRouteSetBuild/NestedAPI36` | 16,100 | 20,160 | 51,460 | 14,093 | Chi |
| `ScenarioRouteSetBuild/ParamsAny24` | 15,567 | 18,909 | 56,116 | 18,120 | Zinc |
| `ScenarioRouteSetStatic/Static157` | 87.44 | 166.5 | 190.3 | 255.6 | Zinc |
| `ScenarioRouteSetStatic/GitHubAPI203` | 86.32 | 153.7 | 173.2 | 229.2 | Zinc |
| `ScenarioRouteSetStatic/GPlusAPI13` | 85.29 | 238.8 | 168.6 | 224.6 | Zinc |
| `ScenarioRouteSetStatic/ParseAPI26` | 85.44 | 150.2 | 172 | 225.1 | Zinc |
| `ScenarioRouteSetStatic/NestedAPI36` | 86.09 | 149.6 | 174.9 | 234 | Zinc |
| `ScenarioRouteSetStatic/ParamsAny24` | 84.96 | 153.4 | 164.9 | 223.9 | Zinc |
| `ScenarioRouteSetParam/GitHubAPI203` | 171.9 | 210.7 | 250.1 | 553.8 | Zinc |
| `ScenarioRouteSetParam/GPlusAPI13` | 149.9 | 171.9 | 198.1 | 439.3 | Zinc |
| `ScenarioRouteSetParam/ParseAPI26` | 141.2 | 173.7 | 189.8 | 442.1 | Zinc |
| `ScenarioRouteSetParam/NestedAPI36` | 159.5 | 191.1 | 218.7 | 490.4 | Zinc |
| `ScenarioRouteSetParam/ParamsAny24` | 176.8 | 213.6 | 242.7 | 552 | Zinc |
| `ScenarioRouteSetNotFound/Static157` | 111.6 | 69.44 | 801.8 | 351.2 | Gin |
| `ScenarioRouteSetNotFound/GitHubAPI203` | 100.2 | 123.3 | 799.8 | 342.5 | Zinc |
| `ScenarioRouteSetNotFound/GPlusAPI13` | 98.01 | 65.97 | 803.4 | 347.1 | Gin |
| `ScenarioRouteSetNotFound/ParseAPI26` | 100.8 | 113 | 803.8 | 346.2 | Zinc |
| `ScenarioRouteSetNotFound/NestedAPI36` | 100.4 | 122.5 | 795 | 345.3 | Zinc |
| `ScenarioRouteSetNotFound/ParamsAny24` | 100.5 | 79.44 | 805.7 | 354.2 | Gin |
| `ScenarioRouteSetMethodMismatch/Static157` | 109 | 101.8 | 979.5 | 357.9 | Gin |
| `ScenarioRouteSetMethodMismatch/GitHubAPI203` | 109.5 | 163.2 | 951.5 | 320.9 | Zinc |
| `ScenarioRouteSetMethodMismatch/GPlusAPI13` | 107.2 | 186.6 | 958.4 | 436.1 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParseAPI26` | 109.3 | 157.9 | 949.5 | 316.4 | Zinc |
| `ScenarioRouteSetMethodMismatch/NestedAPI36` | 115.7 | 178.6 | 954.4 | 327.6 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParamsAny24` | 118.9 | 112.2 | 945 | 315.1 | Gin |
| `ScenarioRouteSetAll/Static157` | 95.69 | 184.7 | 201.8 | 293.3 | Zinc |
| `ScenarioRouteSetAll/GitHubAPI203` | 172.2 | 209.4 | 232.6 | 517.5 | Zinc |
| `ScenarioRouteSetAll/GPlusAPI13` | 146.4 | 238.2 | 199.2 | 414.6 | Zinc |
| `ScenarioRouteSetAll/ParseAPI26` | 134.1 | 173.6 | 187.9 | 375.9 | Zinc |
| `ScenarioRouteSetAll/NestedAPI36` | 149.9 | 184.1 | 203.4 | 416.1 | Zinc |
| `ScenarioRouteSetAll/ParamsAny24` | 169.2 | 195.2 | 219.6 | 495.3 | Zinc |

## Where Zinc is slower

The percentages below compare Zinc's median with the fastest framework in the same row. Differences of a few percent are within the range where a fresh paired run matters most.

| Benchmark | Winner | Zinc slower by |
| --- | --- | ---: |
| `NotFound` | Gin | 79.3% |
| `LargeRouteSetNotFound` | Gin | 69.2% |
| `ScenarioRouteSetNotFound/Static157` | Gin | 60.7% |
| `StaticFileNotFound` | Gin | 55.3% |
| `ScenarioRouteSetNotFound/GPlusAPI13` | Gin | 48.6% |
| `ScenarioRouteSetNotFound/ParamsAny24` | Gin | 26.4% |
| `LargeRouteSetMethodMismatch` | Gin | 15.9% |
| `ScenarioRouteSetBuild/NestedAPI36` | Chi | 14.2% |
| `RouteRegistrationParam` | Gin | 10.0% |
| `ScenarioRouteSetMethodMismatch/Static157` | Gin | 7.2% |
| `ScenarioRouteSetMethodMismatch/ParamsAny24` | Gin | 6.0% |
| `QueryParams` | Echo | 3.2% |
| `ScenarioRouteSetBuild/ParseAPI26` | Chi | 3.1% |
| `APIUnauthorizedReject` | Gin | 1.1% |
| `APIBindMultipartHappyPath` | Gin | 1.0% |
| `StaticFileHit` | Chi | 0.8% |
| `LargeJSONResponse` | Echo | 0.3% |

## Scope and reproducibility

The 77 scenarios cover routing, misses, registration, route sets, binding, response helpers, static files, middleware, and parallel in-process dispatch. They are distinct from the upstream Gin routing suite in [GIN_BENCHMARK.md](GIN_BENCHMARK.md); its counts must not be added to these.

To collect a new head-to-head run from the benchmark module:

```bash
cd benchmarks
GOTOOLCHAIN=go1.27.1 go test -run='^$' -bench=. -benchmem -count=10 -benchtime=100ms
```

The September 25 pass filtered to Zinc's 77 comparable scenarios and reused the saved rival samples. Its direct and nested raw logs, plus the comparison CSV, are in the local ignored `benchmarks/results/four-paths/` directory. Performance varies with CPU load; the retained code paths were also checked with alternating Zinc before/after runs.
