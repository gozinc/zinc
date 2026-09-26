# Zinc Benchmarks

This report compares equivalent in-process Zinc, Gin, Echo, and Chi workloads through each framework's idiomatic API. Lower latency is better.

All four frameworks were measured together in one run on 2026-09-26, for the Zinc 0.4.0 release, with the code in `5d59449`, which is `v0.4.0` plus a CONTRIBUTING change.

## Run information

- Date: `2026-09-26`
- Zinc commit: `5d59449` (the v0.4.0 code)
- Rivals: Gin `v1.12.0`, Echo `v5.2.0`, Chi `v5.2.5`
- Go: `go1.27.1`
- Machine: `Apple M1 Pro`
- OS / architecture: `darwin / arm64`
- Samples: `10 × 100ms` per scenario, for every framework
- Measurement: median `ns/op` across the 10 samples

## Summary

| Framework | Lowest median latency |
| --- | ---: |
| Zinc | 61 / 77 |
| Gin | 11 / 77 |
| Echo | 1 / 77 |
| Chi | 4 / 77 |

Zinc has the lowest median in **61 of 77** scenarios, and is fastest or within 2% of the fastest in **65**. Common string-response routing paths measure 16 B and 1 allocation per request.

## Results

All values are median ns/op. The winner is the framework with the lowest median in that row.

| Benchmark | Zinc | Gin | Echo | Chi | Winner |
| --- | ---: | ---: | ---: | ---: | --- |
| `HelloWorld` | 80.06 | 135.2 | 148.5 | 175.1 | Zinc |
| `StaticRoute` | 78.72 | 134.2 | 150.2 | 170.7 | Zinc |
| `StaticRouteCold` | 83.01 | 158.9 | 180.7 | 231 | Zinc |
| `RouterParam` | 105.8 | 139.1 | 155.3 | 304 | Zinc |
| `RouterParamCold` | 128.9 | 143.2 | 157.1 | 339.5 | Zinc |
| `JSONResponse` | 544.9 | 565 | 556 | 617.6 | Zinc |
| `QueryParams` | 318.8 | 446.1 | 313.8 | 482.5 | Echo |
| `MiddlewareChain` | 236.5 | 492.9 | 316.3 | 864.5 | Zinc |
| `NotFound` | 100.2 | 52.64 | 700 | 288.1 | Gin |
| `LargeRouteSetStatic` | 79.53 | 147.2 | 166.3 | 198.6 | Zinc |
| `LargeRouteSetStaticMixed` | 83.76 | 166.1 | 180.9 | 245.2 | Zinc |
| `LargeRouteSetNotFound` | 98.17 | 55.76 | 711.2 | 296.5 | Gin |
| `LargeRouteSetMethodMismatch` | 101.2 | 86.28 | 845.2 | 281.4 | Gin |
| `LargeRouteSetParam` | 106.1 | 153.4 | 173.4 | 362.6 | Zinc |
| `LargeRouteSetParamMixed` | 130.5 | 171.9 | 191.8 | 410 | Zinc |
| `RouteRegistrationStatic` | 57,247 | 66,337.5 | 306,513 | 81,150 | Zinc |
| `RouteRegistrationParam` | 50,046 | 45,202 | 142,740.5 | 62,823.5 | Gin |
| `APIParamQueryJSON` | 741.6 | 2,664 | 1,238 | 1,014 | Zinc |
| `APIHappyPath` | 985.9 | 3,093 | 1,428.5 | 1,718 | Zinc |
| `APIBindJSONHappyPath` | 1,566.5 | 4,083 | 1,734 | 2,682.5 | Zinc |
| `APIBindHeaderQueryJSON` | 1,601 | 5,368 | 2,038.5 | 2,362 | Zinc |
| `APIBindInvalidJSON` | 906.8 | 897.7 | 1,058 | 1,039.5 | Gin |
| `APIBindValidationFailure` | 497.9 | 704.7 | 685 | 761.8 | Zinc |
| `APIBindMultipartHappyPath` | 10,457.5 | 10,601.5 | 10,583 | 10,754.5 | Zinc |
| `LargeJSONResponse` | 41,818.5 | 45,220 | 41,869 | 42,021 | Zinc |
| `LargeJSONBind` | 130,681.5 | 165,499.5 | 162,818.5 | 162,730.5 | Zinc |
| `StaticFileHit` | 14,696.5 | 24,496 | 27,950.5 | 14,545 | Chi |
| `StaticFileNotFound` | 1,503.5 | 970.4 | 2,845 | 1,824 | Gin |
| `NestedGroupMiddlewareAPI` | 997.5 | 3,242.5 | 1,517.5 | 2,642.5 | Zinc |
| `APIUnauthorizedReject` | 50.5 | 48.53 | 56.34 | 172.9 | Gin |
| `ParallelStaticRoute` | 25.7 | 47.4 | 44.37 | 152.9 | Zinc |
| `ParallelRouterParam` | 28.84 | 53.63 | 44.09 | 297.5 | Zinc |
| `ParallelMiddlewareChain` | 55.09 | 188.1 | 81.51 | 897 | Zinc |
| `ParallelAPIHappyPath` | 142.4 | 818.7 | 380.4 | 1,281.5 | Zinc |
| `Param5` | 161.2 | 196.9 | 232.2 | 527.1 | Zinc |
| `Param10` | 264.5 | 362.3 | 395.2 | 1,188 | Zinc |
| `NestedGroupStatic` | 82 | 229.2 | 167.9 | 814.8 | Zinc |
| `NestedGroupParam` | 166.6 | 183.6 | 208.2 | 1,096 | Zinc |
| `NestedGroupNotFound` | 97.06 | 166.2 | 192.3 | 989 | Zinc |
| `NestedGroupMethodMismatch` | 173.6 | 195.1 | 303 | 1,033.5 | Zinc |
| `WildcardTail` | 108.8 | 149.1 | 157.7 | 354.4 | Zinc |
| `WildcardTailNotFound` | 92.19 | 151.9 | 152.9 | 178.6 | Zinc |
| `ScenarioRouteSetBuild/Static157` | 41,370.5 | 70,058.5 | 153,920.5 | 57,628.5 | Zinc |
| `ScenarioRouteSetBuild/GitHubAPI203` | 100,661 | 118,632.5 | 291,341 | 100,062.5 | Chi |
| `ScenarioRouteSetBuild/GPlusAPI13` | 6,753.5 | 7,389.5 | 16,013 | 6,806 | Zinc |
| `ScenarioRouteSetBuild/ParseAPI26` | 10,969 | 12,245 | 21,258.5 | 9,680.5 | Chi |
| `ScenarioRouteSetBuild/NestedAPI36` | 17,069 | 19,562.5 | 32,272 | 13,313.5 | Chi |
| `ScenarioRouteSetBuild/ParamsAny24` | 16,673.5 | 18,099 | 37,477 | 17,211.5 | Zinc |
| `ScenarioRouteSetStatic/Static157` | 86.11 | 162.9 | 185.6 | 241.3 | Zinc |
| `ScenarioRouteSetStatic/GitHubAPI203` | 85.6 | 151.4 | 168.8 | 217.4 | Zinc |
| `ScenarioRouteSetStatic/GPlusAPI13` | 83.94 | 222.7 | 163.5 | 217.2 | Zinc |
| `ScenarioRouteSetStatic/ParseAPI26` | 83.2 | 146.4 | 167.3 | 214.2 | Zinc |
| `ScenarioRouteSetStatic/NestedAPI36` | 83.2 | 146.1 | 170.2 | 220.2 | Zinc |
| `ScenarioRouteSetStatic/ParamsAny24` | 83.14 | 149 | 159.6 | 213.2 | Zinc |
| `ScenarioRouteSetParam/GitHubAPI203` | 172.3 | 206.4 | 240.9 | 525.5 | Zinc |
| `ScenarioRouteSetParam/GPlusAPI13` | 148.4 | 166.8 | 190.3 | 414 | Zinc |
| `ScenarioRouteSetParam/ParseAPI26` | 138.1 | 168.3 | 181.8 | 417.1 | Zinc |
| `ScenarioRouteSetParam/NestedAPI36` | 158.4 | 185.2 | 211.1 | 466.5 | Zinc |
| `ScenarioRouteSetParam/ParamsAny24` | 175.3 | 208.1 | 233.1 | 520 | Zinc |
| `ScenarioRouteSetNotFound/Static157` | 107.2 | 98.75 | 754.5 | 330.8 | Gin |
| `ScenarioRouteSetNotFound/GitHubAPI203` | 96.1 | 110.2 | 750.3 | 324.4 | Zinc |
| `ScenarioRouteSetNotFound/GPlusAPI13` | 95.5 | 84 | 749.8 | 325.1 | Gin |
| `ScenarioRouteSetNotFound/ParseAPI26` | 95.98 | 100.6 | 748.1 | 325.5 | Zinc |
| `ScenarioRouteSetNotFound/NestedAPI36` | 96.28 | 108.1 | 747.9 | 325.1 | Zinc |
| `ScenarioRouteSetNotFound/ParamsAny24` | 96.37 | 86.14 | 752.9 | 340 | Gin |
| `ScenarioRouteSetMethodMismatch/Static157` | 105.6 | 98.25 | 911.2 | 332.1 | Gin |
| `ScenarioRouteSetMethodMismatch/GitHubAPI203` | 106.7 | 153.1 | 891.2 | 300.8 | Zinc |
| `ScenarioRouteSetMethodMismatch/GPlusAPI13` | 106 | 175.2 | 900.9 | 410.9 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParseAPI26` | 107.2 | 147.4 | 893.4 | 299 | Zinc |
| `ScenarioRouteSetMethodMismatch/NestedAPI36` | 106.3 | 164.1 | 895.6 | 308.1 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParamsAny24` | 105.7 | 106.2 | 880.6 | 297.6 | Zinc |
| `ScenarioRouteSetAll/Static157` | 89.93 | 180.1 | 193.8 | 275.6 | Zinc |
| `ScenarioRouteSetAll/GitHubAPI203` | 168.9 | 204.2 | 220.3 | 486.1 | Zinc |
| `ScenarioRouteSetAll/GPlusAPI13` | 142.3 | 226.5 | 190.7 | 394.2 | Zinc |
| `ScenarioRouteSetAll/ParseAPI26` | 131.3 | 168.4 | 181.4 | 352.8 | Zinc |
| `ScenarioRouteSetAll/NestedAPI36` | 143 | 178.1 | 195.2 | 390.6 | Zinc |
| `ScenarioRouteSetAll/ParamsAny24` | 166.6 | 191.1 | 210.9 | 462.9 | Zinc |

## Where Zinc is slower

The percentages below compare Zinc's median with the fastest framework in the same row. Differences of a few percent are within the range where a fresh paired run matters most.

| Benchmark | Winner | Zinc slower by |
| --- | --- | ---: |
| `NotFound` | Gin | 90.4% |
| `LargeRouteSetNotFound` | Gin | 76.1% |
| `StaticFileNotFound` | Gin | 54.9% |
| `ScenarioRouteSetBuild/NestedAPI36` | Chi | 28.2% |
| `LargeRouteSetMethodMismatch` | Gin | 17.3% |
| `ScenarioRouteSetNotFound/GPlusAPI13` | Gin | 13.7% |
| `ScenarioRouteSetBuild/ParseAPI26` | Chi | 13.3% |
| `ScenarioRouteSetNotFound/ParamsAny24` | Gin | 11.9% |
| `RouteRegistrationParam` | Gin | 10.7% |
| `ScenarioRouteSetNotFound/Static157` | Gin | 8.6% |
| `ScenarioRouteSetMethodMismatch/Static157` | Gin | 7.5% |
| `APIUnauthorizedReject` | Gin | 4.0% |
| `QueryParams` | Echo | 1.6% |
| `StaticFileHit` | Chi | 1.0% |
| `APIBindInvalidJSON` | Gin | 1.0% |
| `ScenarioRouteSetBuild/GitHubAPI203` | Chi | 0.6% |

## Scope and reproducibility

The 77 scenarios cover routing, misses, registration, route sets, binding, response helpers, static files, middleware, and parallel in-process dispatch. They are distinct from the upstream Gin routing suite in [GIN_BENCHMARK.md](GIN_BENCHMARK.md); its counts must not be added to these.

To collect a new head-to-head run from the benchmark module:

```bash
cd benchmarks
GOTOOLCHAIN=go1.27.1 go test -run='^$' -bench=. -benchmem -count=10 -benchtime=100ms
```

This report is `zincbench` run `20260926-103557-5d59449`, recorded on a quiet machine on AC power. Its raw log is in the local benchmark results and the release bundle, `benchmarks/results/bundles/0.4.0.zip`, which git ignores.
