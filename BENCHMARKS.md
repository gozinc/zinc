# Zinc Benchmarks

This report compares equivalent in-process Zinc, Gin, Echo, and Chi workloads through each framework's idiomatic API. Lower latency is better.

All four frameworks were measured together in one run on 2026-09-26, for the Zinc 0.4.0 release, with the code in `d836005`.

## Run information

- Date: `2026-09-26`
- Zinc commit: `d836005` (0.4.0)
- Rivals: Gin `v1.12.0`, Echo `v5.2.0`, Chi `v5.2.5`
- Go: `go1.27.1`
- Machine: `Apple M1 Pro`
- OS / architecture: `darwin / arm64`
- Samples: `10 × 100ms` per scenario, for every framework
- Measurement: median `ns/op` across the 10 samples

## Summary

| Framework | Lowest median latency |
| --- | ---: |
| Zinc | 58 / 77 |
| Gin | 12 / 77 |
| Echo | 2 / 77 |
| Chi | 5 / 77 |

Zinc has the lowest median in **58 of 77** scenarios, and is fastest or within 2% of the fastest in **63**. Common string-response routing paths measure 16 B and 1 allocation per request.

## Results

All values are median ns/op. The winner is the framework with the lowest median in that row.

| Benchmark | Zinc | Gin | Echo | Chi | Winner |
| --- | ---: | ---: | ---: | ---: | --- |
| `HelloWorld` | 98.76 | 139.9 | 155.3 | 203.6 | Zinc |
| `StaticRoute` | 85.26 | 143.6 | 169.6 | 195.2 | Zinc |
| `StaticRouteCold` | 93.01 | 169.9 | 191.2 | 256.9 | Zinc |
| `RouterParam` | 115.1 | 144.8 | 171.2 | 379.7 | Zinc |
| `RouterParamCold` | 136.4 | 156.2 | 166.8 | 384.4 | Zinc |
| `JSONResponse` | 588 | 619.4 | 643.1 | 745.8 | Zinc |
| `QueryParams` | 340 | 496.1 | 325 | 530 | Echo |
| `MiddlewareChain` | 300.1 | 558.6 | 338.8 | 1,036.5 | Zinc |
| `NotFound` | 109.2 | 73.62 | 798.5 | 326.9 | Gin |
| `LargeRouteSetStatic` | 89.52 | 152.9 | 174.3 | 224.1 | Zinc |
| `LargeRouteSetStaticMixed` | 90.59 | 173.5 | 192.2 | 272.1 | Zinc |
| `LargeRouteSetNotFound` | 104.3 | 69.93 | 784.9 | 324.9 | Gin |
| `LargeRouteSetMethodMismatch` | 110.8 | 91.94 | 920.1 | 312.4 | Gin |
| `LargeRouteSetParam` | 114.8 | 162.1 | 183.6 | 421.4 | Zinc |
| `LargeRouteSetParamMixed` | 138.9 | 179.6 | 202.3 | 452.8 | Zinc |
| `RouteRegistrationStatic` | 61,728 | 71,277 | 335,690.5 | 88,169 | Zinc |
| `RouteRegistrationParam` | 56,110.5 | 48,330 | 163,049 | 71,351 | Gin |
| `APIParamQueryJSON` | 831 | 3,016.5 | 1,355 | 1,169 | Zinc |
| `APIHappyPath` | 1,122 | 3,589.5 | 1,671.5 | 2,239.5 | Zinc |
| `APIBindJSONHappyPath` | 1,739 | 4,581.5 | 2,013 | 3,125.5 | Zinc |
| `APIBindHeaderQueryJSON` | 1,834 | 5,807.5 | 2,212.5 | 2,873.5 | Zinc |
| `APIBindInvalidJSON` | 1,029 | 1,047.5 | 1,228 | 1,218.5 | Zinc |
| `APIBindValidationFailure` | 552.3 | 805.9 | 771.2 | 862.7 | Zinc |
| `APIBindMultipartHappyPath` | 13,358.5 | 13,151 | 13,661.5 | 14,212 | Gin |
| `LargeJSONResponse` | 47,231.5 | 51,169.5 | 47,154.5 | 48,433.5 | Echo |
| `LargeJSONBind` | 140,693.5 | 180,790 | 184,100 | 183,950 | Zinc |
| `StaticFileHit` | 16,024 | 26,545 | 29,935.5 | 15,682.5 | Chi |
| `StaticFileNotFound` | 1,610.5 | 1,008 | 2,944 | 1,970.5 | Gin |
| `NestedGroupMiddlewareAPI` | 1,058 | 3,496.5 | 1,669 | 2,868.5 | Zinc |
| `APIUnauthorizedReject` | 52.36 | 52.06 | 58.82 | 188.8 | Gin |
| `ParallelStaticRoute` | 30.55 | 49.37 | 46.17 | 155.8 | Zinc |
| `ParallelRouterParam` | 39.74 | 55.5 | 50.3 | 295.4 | Zinc |
| `ParallelMiddlewareChain` | 71.78 | 258.7 | 93.75 | 859.9 | Zinc |
| `ParallelAPIHappyPath` | 210.4 | 885.6 | 394.4 | 1,573.5 | Zinc |
| `Param5` | 168.7 | 203.2 | 256.4 | 611.5 | Zinc |
| `Param10` | 295.7 | 371.2 | 406.9 | 1,318 | Zinc |
| `NestedGroupStatic` | 85.5 | 246.9 | 181.7 | 850.5 | Zinc |
| `NestedGroupParam` | 172.9 | 190.8 | 222.7 | 1,157.5 | Zinc |
| `NestedGroupNotFound` | 102.8 | 175 | 199.8 | 1,053 | Zinc |
| `NestedGroupMethodMismatch` | 179.6 | 203.1 | 313.1 | 1,080.5 | Zinc |
| `WildcardTail` | 113 | 152.9 | 165.6 | 377 | Zinc |
| `WildcardTailNotFound` | 95.92 | 154.9 | 160.3 | 195.9 | Zinc |
| `ScenarioRouteSetBuild/Static157` | 44,327 | 73,377 | 164,309.5 | 62,091 | Zinc |
| `ScenarioRouteSetBuild/GitHubAPI203` | 110,873 | 123,017.5 | 306,592.5 | 109,923 | Chi |
| `ScenarioRouteSetBuild/GPlusAPI13` | 7,447 | 7,776.5 | 16,129.5 | 7,057 | Chi |
| `ScenarioRouteSetBuild/ParseAPI26` | 12,087 | 12,974 | 23,130 | 10,680.5 | Chi |
| `ScenarioRouteSetBuild/NestedAPI36` | 18,679.5 | 20,906.5 | 35,604.5 | 14,521.5 | Chi |
| `ScenarioRouteSetBuild/ParamsAny24` | 18,600 | 19,945.5 | 40,020.5 | 18,672 | Zinc |
| `ScenarioRouteSetStatic/Static157` | 93.34 | 177.2 | 192 | 262.3 | Zinc |
| `ScenarioRouteSetStatic/GitHubAPI203` | 93 | 153.6 | 174 | 223.2 | Zinc |
| `ScenarioRouteSetStatic/GPlusAPI13` | 92.16 | 247.5 | 171.8 | 224.9 | Zinc |
| `ScenarioRouteSetStatic/ParseAPI26` | 93.94 | 153.1 | 178.1 | 231.8 | Zinc |
| `ScenarioRouteSetStatic/NestedAPI36` | 91.16 | 152.3 | 179.8 | 246.2 | Zinc |
| `ScenarioRouteSetStatic/ParamsAny24` | 90.79 | 156.2 | 167.7 | 226.6 | Zinc |
| `ScenarioRouteSetParam/GitHubAPI203` | 177.2 | 211.9 | 252.6 | 554.2 | Zinc |
| `ScenarioRouteSetParam/GPlusAPI13` | 153.9 | 173.9 | 200.9 | 439.6 | Zinc |
| `ScenarioRouteSetParam/ParseAPI26` | 142.7 | 174.4 | 189.8 | 447.1 | Zinc |
| `ScenarioRouteSetParam/NestedAPI36` | 163.6 | 191.7 | 219.4 | 498.2 | Zinc |
| `ScenarioRouteSetParam/ParamsAny24` | 181.5 | 214.8 | 243.6 | 567.6 | Zinc |
| `ScenarioRouteSetNotFound/Static157` | 115 | 67.14 | 811.1 | 366.2 | Gin |
| `ScenarioRouteSetNotFound/GitHubAPI203` | 103.3 | 125.7 | 829.4 | 366.2 | Zinc |
| `ScenarioRouteSetNotFound/GPlusAPI13` | 103.6 | 65.36 | 831.2 | 380.4 | Gin |
| `ScenarioRouteSetNotFound/ParseAPI26` | 102.2 | 113.8 | 820.5 | 359.9 | Zinc |
| `ScenarioRouteSetNotFound/NestedAPI36` | 102.2 | 122.7 | 801.9 | 362.4 | Zinc |
| `ScenarioRouteSetNotFound/ParamsAny24` | 102.6 | 78.94 | 811.5 | 363.1 | Gin |
| `ScenarioRouteSetMethodMismatch/Static157` | 114.3 | 112.2 | 1,017 | 398.9 | Gin |
| `ScenarioRouteSetMethodMismatch/GitHubAPI203` | 118.8 | 170.4 | 979 | 326.9 | Zinc |
| `ScenarioRouteSetMethodMismatch/GPlusAPI13` | 119.5 | 202.4 | 994.2 | 442.2 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParseAPI26` | 121.8 | 168.2 | 1,020 | 353.5 | Zinc |
| `ScenarioRouteSetMethodMismatch/NestedAPI36` | 117.5 | 183.7 | 1,025.5 | 354.2 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParamsAny24` | 116.5 | 111.9 | 948.2 | 325.1 | Gin |
| `ScenarioRouteSetAll/Static157` | 100.9 | 190.6 | 206.5 | 303.5 | Zinc |
| `ScenarioRouteSetAll/GitHubAPI203` | 188.1 | 212.4 | 241.6 | 567.7 | Zinc |
| `ScenarioRouteSetAll/GPlusAPI13` | 153.9 | 259.5 | 205 | 440.1 | Zinc |
| `ScenarioRouteSetAll/ParseAPI26` | 146.6 | 185.6 | 200.3 | 477.6 | Zinc |
| `ScenarioRouteSetAll/NestedAPI36` | 151.3 | 186.6 | 206.1 | 423.2 | Zinc |
| `ScenarioRouteSetAll/ParamsAny24` | 173.8 | 199.2 | 221.1 | 524.6 | Zinc |

## Where Zinc is slower

The percentages below compare Zinc's median with the fastest framework in the same row. Differences of a few percent are within the range where a fresh paired run matters most.

| Benchmark | Winner | Zinc slower by |
| --- | --- | ---: |
| `ScenarioRouteSetNotFound/Static157` | Gin | 71.3% |
| `StaticFileNotFound` | Gin | 59.8% |
| `ScenarioRouteSetNotFound/GPlusAPI13` | Gin | 58.5% |
| `LargeRouteSetNotFound` | Gin | 49.1% |
| `NotFound` | Gin | 48.4% |
| `ScenarioRouteSetNotFound/ParamsAny24` | Gin | 30.0% |
| `ScenarioRouteSetBuild/NestedAPI36` | Chi | 28.6% |
| `LargeRouteSetMethodMismatch` | Gin | 20.5% |
| `RouteRegistrationParam` | Gin | 16.1% |
| `ScenarioRouteSetBuild/ParseAPI26` | Chi | 13.2% |
| `ScenarioRouteSetBuild/GPlusAPI13` | Chi | 5.5% |
| `QueryParams` | Echo | 4.6% |
| `ScenarioRouteSetMethodMismatch/ParamsAny24` | Gin | 4.1% |
| `StaticFileHit` | Chi | 2.2% |
| `ScenarioRouteSetMethodMismatch/Static157` | Gin | 1.8% |
| `APIBindMultipartHappyPath` | Gin | 1.6% |
| `ScenarioRouteSetBuild/GitHubAPI203` | Chi | 0.9% |
| `APIUnauthorizedReject` | Gin | 0.6% |
| `LargeJSONResponse` | Echo | 0.2% |

## Scope and reproducibility

The 77 scenarios cover routing, misses, registration, route sets, binding, response helpers, static files, middleware, and parallel in-process dispatch. They are distinct from the upstream Gin routing suite in [GIN_BENCHMARK.md](GIN_BENCHMARK.md); its counts must not be added to these.

To collect a new head-to-head run from the benchmark module:

```bash
cd benchmarks
GOTOOLCHAIN=go1.27.1 go test -run='^$' -bench=. -benchmem -count=10 -benchtime=100ms
```

This report is `zincbench` run `20260926-005419-57422b6-dirty`. The run recorded commit `57422b6`, which became `d836005` when test and tooling fixes were moved into earlier commits; the library code is identical. Its raw log and every run recorded during the 0.4 work are in the release bundle, `benchmarks/results/bundles/0.4.0.zip`, which is ignored by git. Performance varies with CPU load. The eight scenarios where this run measured Zinc slower than the pre-0.4 baseline run were rechecked with interleaved before-and-after runs (`zincbench ab`) against the pre-0.4 code, commit `f52f657`, and none differed.
