# Zinc Benchmarks

This report compares Zinc with Gin, Echo, and Chi using equivalent in-process workloads through each framework's idiomatic API. Lower values are better.

## Run information

- Date: `2026-07-31`
- Zinc commit: `5f77c75`
- Go: `go1.26.1`
- Machine: `Apple M1 Pro`
- OS / architecture: `darwin / arm64`
- Command: `cd benchmarks && go test -run=^$ -bench=. -benchmem -count=1`
- Measurement: `ns/op`

## Summary

| Framework | Lowest latency |
| --- | ---: |
| Zinc | 62 / 77 |
| Gin | 10 / 77 |
| Chi | 5 / 77 |
| Echo | 0 / 77 |

Zinc records the lowest latency in 62 of 77 comparable rows. It is fastest or within 2% of the fastest result in 63 rows. Zinc's primary static, parameter, and not-found dispatch benchmarks perform zero request-time heap allocations.

Results vary by workload and machine. Small differences from a single run should be confirmed with repeated samples and `benchstat`.

## Results

| Benchmark | Zinc | Gin | Echo | Chi | Winner |
| --- | ---: | ---: | ---: | ---: | --- |
| `HelloWorld` | 65.27 | 89.27 | 138.2 | 198.9 | Zinc |
| `StaticRoute` | 68.61 | 88.00 | 126.9 | 188.6 | Zinc |
| `StaticRouteCold` | 74.61 | 113.3 | 153.0 | 247.8 | Zinc |
| `RouterParam` | 80.65 | 94.61 | 132.4 | 345.9 | Zinc |
| `RouterParamCold` | 108.7 | 98.25 | 145.4 | 386.7 | Gin |
| `JSONResponse` | 322.6 | 394.5 | 387.9 | 508.2 | Zinc |
| `QueryParams` | 414.6 | 457.2 | 494.8 | 581.8 | Zinc |
| `MiddlewareChain` | 365.1 | 424.2 | 540.5 | 950.0 | Zinc |
| `NotFound` | 72.18 | 58.46 | 598.0 | 349.5 | Gin |
| `LargeRouteSetStatic` | 74.36 | 101.9 | 142.9 | 217.4 | Zinc |
| `LargeRouteSetStaticMixed` | 80.00 | 120.7 | 156.6 | 281.0 | Zinc |
| `LargeRouteSetNotFound` | 72.73 | 64.90 | 616.8 | 346.7 | Gin |
| `LargeRouteSetMethodMismatch` | 111.1 | 98.81 | 891.7 | 313.5 | Gin |
| `LargeRouteSetParam` | 79.56 | 107.5 | 150.8 | 392.3 | Zinc |
| `LargeRouteSetParamMixed` | 109.0 | 127.6 | 167.6 | 442.8 | Zinc |
| `RouteRegistrationStatic` | 60642 | 73543 | 338036 | 88416 | Zinc |
| `RouteRegistrationParam` | 57919 | 51874 | 160023 | 70344 | Gin |
| `APIParamQueryJSON` | 843.5 | 2663 | 1748 | 976.3 | Zinc |
| `APIHappyPath` | 1186 | 3024 | 2287 | 1737 | Zinc |
| `APIBindJSONHappyPath` | 2151 | 4408 | 2529 | 2976 | Zinc |
| `APIBindHeaderQueryJSON` | 2280 | 5740 | 3018 | 2685 | Zinc |
| `APIBindInvalidJSON` | 761.0 | 694.1 | 838.5 | 849.1 | Gin |
| `APIBindValidationFailure` | 693.9 | 924.0 | 888.2 | 1004 | Zinc |
| `APIBindMultipartHappyPath` | 11621 | 11527 | 11695 | 11963 | Gin |
| `LargeJSONResponse` | 29107 | 32187 | 29114 | 29329 | Zinc |
| `LargeJSONBind` | 227622 | 246603 | 241525 | 240938 | Zinc |
| `StaticFileHit` | 15086 | 25991 | 29138 | 15109 | Zinc |
| `StaticFileNotFound` | 1536 | 1031 | 2688 | 1998 | Gin |
| `NestedGroupMiddlewareAPI` | 1249 | 3184 | 2145 | 2779 | Zinc |
| `APIUnauthorizedReject` | 40.28 | 49.04 | 75.01 | 198.3 | Zinc |
| `ParallelStaticRoute` | 10.57 | 44.96 | 54.00 | 166.3 | Zinc |
| `ParallelRouterParam` | 13.68 | 53.86 | 56.11 | 315.4 | Zinc |
| `ParallelMiddlewareChain` | 60.68 | 196.0 | 267.6 | 931.6 | Zinc |
| `ParallelAPIHappyPath` | 371.3 | 886.4 | 658.6 | 1349 | Zinc |
| `Param5` | 133.5 | 151.5 | 211.7 | 575.8 | Zinc |
| `Param10` | 241.6 | 341.4 | 372.0 | 1272 | Zinc |
| `NestedGroupStatic` | 64.58 | 188.4 | 142.7 | 874.0 | Zinc |
| `NestedGroupParam` | 129.9 | 144.2 | 187.9 | 1118 | Zinc |
| `NestedGroupNotFound` | 69.45 | 117.0 | 174.8 | 1048 | Zinc |
| `NestedGroupMethodMismatch` | 121.1 | 155.1 | 445.4 | 1121 | Zinc |
| `WildcardTail` | 80.51 | 103.4 | 134.0 | 438.1 | Zinc |
| `WildcardTailNotFound` | 71.15 | 99.82 | 131.9 | 202.8 | Zinc |
| `ScenarioRouteSetBuild/Static157` | 49037 | 77647 | 180401 | 69326 | Zinc |
| `ScenarioRouteSetBuild/GitHubAPI203` | 119350 | 127958 | 333379 | 112836 | Chi |
| `ScenarioRouteSetBuild/GPlusAPI13` | 7895 | 8029 | 16619 | 7344 | Chi |
| `ScenarioRouteSetBuild/ParseAPI26` | 13043 | 13851 | 24767 | 10951 | Chi |
| `ScenarioRouteSetBuild/NestedAPI36` | 20930 | 21431 | 36934 | 15312 | Chi |
| `ScenarioRouteSetBuild/ParamsAny24` | 20589 | 20164 | 41356 | 19190 | Chi |
| `ScenarioRouteSetStatic/Static157` | 70.04 | 116.3 | 160.6 | 264.4 | Zinc |
| `ScenarioRouteSetStatic/GitHubAPI203` | 73.57 | 104.0 | 144.0 | 239.7 | Zinc |
| `ScenarioRouteSetStatic/GPlusAPI13` | 68.21 | 186.4 | 140.5 | 235.5 | Zinc |
| `ScenarioRouteSetStatic/ParseAPI26` | 68.14 | 100.8 | 142.3 | 238.7 | Zinc |
| `ScenarioRouteSetStatic/NestedAPI36` | 66.91 | 100.2 | 147.7 | 244.7 | Zinc |
| `ScenarioRouteSetStatic/ParamsAny24` | 68.15 | 105.0 | 134.3 | 242.3 | Zinc |
| `ScenarioRouteSetParam/GitHubAPI203` | 130.2 | 160.8 | 219.4 | 574.8 | Zinc |
| `ScenarioRouteSetParam/GPlusAPI13` | 108.4 | 126.6 | 176.6 | 487.6 | Zinc |
| `ScenarioRouteSetParam/ParseAPI26` | 106.8 | 126.1 | 160.2 | 470.3 | Zinc |
| `ScenarioRouteSetParam/NestedAPI36` | 118.2 | 140.0 | 188.4 | 514.7 | Zinc |
| `ScenarioRouteSetParam/ParamsAny24` | 136.2 | 165.0 | 210.8 | 574.2 | Zinc |
| `ScenarioRouteSetNotFound/Static157` | 72.83 | 67.90 | 646.4 | 388.6 | Gin |
| `ScenarioRouteSetNotFound/GitHubAPI203` | 66.04 | 129.1 | 646.5 | 384.2 | Zinc |
| `ScenarioRouteSetNotFound/GPlusAPI13` | 69.49 | 75.09 | 646.0 | 383.7 | Zinc |
| `ScenarioRouteSetNotFound/ParseAPI26` | 66.32 | 114.6 | 644.0 | 383.3 | Zinc |
| `ScenarioRouteSetNotFound/NestedAPI36` | 66.53 | 128.1 | 642.6 | 381.1 | Zinc |
| `ScenarioRouteSetNotFound/ParamsAny24` | 68.07 | 83.84 | 647.5 | 398.0 | Zinc |
| `ScenarioRouteSetMethodMismatch/Static157` | 117.3 | 113.4 | 1015 | 442.8 | Gin |
| `ScenarioRouteSetMethodMismatch/GitHubAPI203` | 109.4 | 175.8 | 906.3 | 350.5 | Zinc |
| `ScenarioRouteSetMethodMismatch/GPlusAPI13` | 113.8 | 194.9 | 929.1 | 461.8 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParseAPI26` | 106.0 | 175.1 | 901.9 | 334.5 | Zinc |
| `ScenarioRouteSetMethodMismatch/NestedAPI36` | 114.2 | 193.0 | 919.9 | 345.6 | Zinc |
| `ScenarioRouteSetMethodMismatch/ParamsAny24` | 114.4 | 119.3 | 919.0 | 330.6 | Zinc |
| `ScenarioRouteSetAll/Static157` | 75.89 | 150.2 | 190.4 | 304.2 | Zinc |
| `ScenarioRouteSetAll/GitHubAPI203` | 138.8 | 164.1 | 208.3 | 554.4 | Zinc |
| `ScenarioRouteSetAll/GPlusAPI13` | 127.5 | 187.6 | 180.2 | 431.3 | Zinc |
| `ScenarioRouteSetAll/ParseAPI26` | 108.0 | 123.3 | 157.1 | 391.2 | Zinc |
| `ScenarioRouteSetAll/NestedAPI36` | 119.0 | 133.5 | 168.7 | 431.1 | Zinc |
| `ScenarioRouteSetAll/ParamsAny24` | 144.2 | 165.5 | 195.4 | 511.8 | Zinc |

## Scope

The comparison covers core routing, parameter and wildcard routes, route misses, method mismatch, route registration, realistic route trees, binding, response helpers, static files, nested groups, middleware, and parallel in-process dispatch.
