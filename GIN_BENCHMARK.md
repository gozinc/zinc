# Zinc in the Gin HTTP Routing Benchmark

- Machine: Apple M1 Pro
- OS / architecture: macOS, darwin/arm64
- Date: 25 September 2026
- Zinc revision: 0.3.0 development (`42e11d2`)
- Go: `go1.27.1`
- Samples: 5 × 100ms per benchmark; tables show medians
- Source: [gin-gonic/go-http-routing-benchmark](https://github.com/gin-gonic/go-http-routing-benchmark) at `ff3cdf55eccd0aa6a272991db9611a86c734dc51`

> The pinned upstream suite was run locally with a Zinc adapter. Only a filtered-run fixture-initialization fix was added to its timing helpers. This suite is available under the [BSD 3-Clause License](https://github.com/gin-gonic/go-http-routing-benchmark/blob/master/LICENSE); benchmark suite copyright © 2013 Julien Schmidt. Zinc is not affiliated with or endorsed by Gin or the suite authors.

## Summary

Zinc has the lowest median in **1/16** comparable rows against the other `net/http` routers, and **2/16** against Gin, Echo, and Chi. It records **0 B/op and 0 allocs/op** in all 16. This router-focused suite measures different work from Zinc's 77-scenario end-to-end suite; the win counts must not be combined.

The complete GitHub API pass (203 routes) measures Zinc at **19,249 ns/op**, ranking **4th of 12** `net/http` routers. Zinc ranks second on the 157-route static pass and first on the 20-parameter microbenchmark.

> **Fiber caveat:** Fiber uses a separate `fasthttp.RequestCtx` harness. Its times are shown for fidelity to the upstream suite, but it is excluded from `net/http` win counts and rankings.

## Zinc at a glance

| Workload | Zinc rank among `net/http` routers | Zinc ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| GitHub API (203 routes) | 4 / 12 | 19,249 | 0 | 0 |
| Google+ API (13 routes) | 5 / 12 | 1,170 | 0 | 0 |
| Parse API (26 routes) | 5 / 12 | 1,905 | 0 | 0 |
| Static routes (157 routes) | 2 / 13 | 8,086 | 0 | 0 |
| Single parameter | 5 / 12 | 55.63 | 0 | 0 |
| Five parameters | 2 / 12 | 70.91 | 0 | 0 |
| Twenty parameters | 1 / 12 | 125.5 | 0 | 0 |
| Parameter read and write | 5 / 12 | 73.1 | 0 | 0 |

## Memory consumption

Routing-structure bytes retained after registration, estimated once after GC. These are not process RSS or repeated timing samples.

### Static routes: 157

| Router | Bytes |
| --- | ---: |
| HttpRouter | 21,680 |
| Gin | 34,408 |
| Macaron | 36,976 |
| Zinc | 39,376 |
| BunRouter | 51,232 |
| Fiber | 59,248 |
| http.ServeMux | 69,216 |
| HttpTreeMux | 73,448 |
| Chi | 83,160 |
| Echo | 92,104 |
| Beego | 98,824 |
| Goji v2 | 117,952 |
| GorillaMux | 599,496 |
| GoRestful | 819,688 |

### GitHub API routes: 203

| Router | Bytes |
| --- | ---: |
| HttpRouter | 37,072 |
| Gin | 58,840 |
| HttpTreeMux | 78,800 |
| Macaron | 90,632 |
| BunRouter | 93,776 |
| Chi | 94,888 |
| Zinc | 102,440 |
| Echo | 117,912 |
| Goji v2 | 118,640 |
| Beego | 150,840 |
| Fiber | 163,832 |
| GoRestful | 1,270,704 |
| GorillaMux | 1,319,680 |

### Google+ API routes: 13

| Router | Bytes |
| --- | ---: |
| HttpRouter | 2,776 |
| Gin | 4,576 |
| BunRouter | 7,360 |
| HttpTreeMux | 7,440 |
| Chi | 8,008 |
| Goji v2 | 8,096 |
| Zinc | 8,400 |
| Macaron | 8,672 |
| Beego | 10,256 |
| Fiber | 10,840 |
| Echo | 11,096 |
| GorillaMux | 68,000 |
| GoRestful | 72,520 |

### Parse API routes: 26

| Router | Bytes |
| --- | ---: |
| HttpRouter | 5,024 |
| HttpTreeMux | 7,848 |
| Gin | 7,896 |
| BunRouter | 9,336 |
| Chi | 9,656 |
| Zinc | 13,272 |
| Macaron | 13,704 |
| Echo | 13,944 |
| Fiber | 15,352 |
| Goji v2 | 16,064 |
| Beego | 19,256 |
| GorillaMux | 105,384 |
| GoRestful | 121,184 |

## Benchmark results

The four `*All` rows time one complete pass over the route fixture, not one HTTP request. The remaining rows are single-request microbenchmarks. All values below are five-sample medians.

### GitHub API (203 routes)

One operation covers all 203 routes in the fixture.

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | Gin | 13,986 | 0 | 0 |
| 2 | BunRouter | 15,907 | 0 | 0 |
| 3 | Echo | 16,196 | 0 | 0 |
| 4 | **Zinc** | 19,249 | 0 | 0 |
| 5 | HttpRouter | 22,020 | 13,792 | 167 |
| 6 | HttpTreeMux | 71,181 | 65,856 | 671 |
| 7 | Chi | 135,847 | 130,817 | 740 |
| 8 | Beego | 136,871 | 71,457 | 609 |
| — | Fiber† | 161,916 | 0 | 0 |
| 9 | Macaron | 169,517 | 147,784 | 1,624 |
| 10 | Goji v2 | 322,153 | 313,744 | 3,712 |
| 11 | GoRestful | 1,223,138 | 1,006,744 | 3,009 |
| 12 | GorillaMux | 1,869,213 | 225,670 | 1,588 |

### Google+ API (13 routes)

One operation covers all 13 routes in the fixture.

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 488.2 | 0 | 0 |
| 2 | Gin | 613.2 | 0 | 0 |
| 3 | Echo | 652 | 0 | 0 |
| 4 | HttpRouter | 963.9 | 640 | 11 |
| 5 | **Zinc** | 1,170 | 0 | 0 |
| — | Fiber† | 3,555 | 0 | 0 |
| 6 | HttpTreeMux | 3,688 | 4,032 | 38 |
| 7 | Chi | 7,529 | 8,480 | 48 |
| 8 | Beego | 7,596 | 4,576 | 39 |
| 9 | Macaron | 10,599 | 9,464 | 104 |
| 10 | Goji v2 | 11,155 | 15,120 | 115 |
| 11 | GorillaMux | 21,998 | 14,448 | 102 |
| 12 | GoRestful | 35,982 | 60,720 | 193 |

### Parse API (26 routes)

One operation covers all 26 routes in the fixture.

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 815.2 | 0 | 0 |
| 2 | Gin | 1,057 | 0 | 0 |
| 3 | Echo | 1,071 | 0 | 0 |
| 4 | HttpRouter | 1,425 | 640 | 16 |
| 5 | **Zinc** | 1,905 | 0 | 0 |
| 6 | HttpTreeMux | 5,379 | 5,728 | 51 |
| — | Fiber† | 6,369 | 0 | 0 |
| 7 | Chi | 13,202 | 14,944 | 84 |
| 8 | Beego | 13,931 | 9,152 | 78 |
| 9 | Goji v2 | 20,368 | 29,456 | 199 |
| 10 | Macaron | 20,756 | 18,928 | 208 |
| 11 | GorillaMux | 40,871 | 26,960 | 198 |
| 12 | GoRestful | 79,250 | 131,728 | 380 |

### Static routes (157 routes)

One operation covers all 157 routes in the fixture.

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | HttpRouter | 5,828 | 0 | 0 |
| 2 | **Zinc** | 8,086 | 0 | 0 |
| 3 | HttpTreeMux | 8,343 | 0 | 0 |
| 4 | BunRouter | 8,599 | 0 | 0 |
| 5 | Gin | 9,659 | 0 | 0 |
| 6 | Echo | 10,009 | 0 | 0 |
| 7 | http.ServeMux | 21,171 | 0 | 0 |
| — | Fiber† | 40,900 | 0 | 0 |
| 8 | Chi | 59,325 | 57,776 | 314 |
| 9 | Beego | 91,645 | 55,264 | 471 |
| 10 | Goji v2 | 120,575 | 175,840 | 1,099 |
| 11 | Macaron | 121,298 | 114,296 | 1,256 |
| 12 | GorillaMux | 455,376 | 133,138 | 1,099 |
| 13 | GoRestful | 639,726 | 677,824 | 2,193 |

### Single parameter

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 17.75 | 0 | 0 |
| 2 | Echo | 26.45 | 0 | 0 |
| 3 | Gin | 31.89 | 0 | 0 |
| 4 | HttpRouter | 49.73 | 32 | 1 |
| 5 | **Zinc** | 55.63 | 0 | 0 |
| — | Fiber† | 175.6 | 0 | 0 |
| 6 | HttpTreeMux | 288.2 | 352 | 3 |
| 7 | Beego | 523.8 | 352 | 3 |
| 8 | Chi | 550.7 | 704 | 4 |
| 9 | Goji v2 | 751.2 | 1,136 | 8 |
| 10 | GorillaMux | 1,038 | 1,152 | 8 |
| 11 | Macaron | 1,090 | 1,064 | 10 |
| 12 | GoRestful | 2,413 | 4,600 | 15 |

### Five parameters

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | Gin | 60.51 | 0 | 0 |
| 2 | **Zinc** | 70.91 | 0 | 0 |
| 3 | Echo | 71.32 | 0 | 0 |
| 4 | BunRouter | 82.79 | 0 | 0 |
| 5 | HttpRouter | 163.5 | 160 | 1 |
| — | Fiber† | 362.1 | 0 | 0 |
| 6 | HttpTreeMux | 560.5 | 576 | 6 |
| 7 | Beego | 664.1 | 352 | 3 |
| 8 | Chi | 769.2 | 704 | 4 |
| 9 | Goji v2 | 890.4 | 1,200 | 8 |
| 10 | Macaron | 1,183 | 1,064 | 10 |
| 11 | GorillaMux | 1,673 | 1,216 | 8 |
| 12 | GoRestful | 2,873 | 4,712 | 15 |

### Twenty parameters

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | **Zinc** | 125.5 | 0 | 0 |
| 2 | Gin | 175.9 | 0 | 0 |
| 3 | Echo | 206 | 0 | 0 |
| 4 | HttpRouter | 505 | 704 | 1 |
| 5 | BunRouter | 617 | 0 | 0 |
| — | Fiber† | 723.2 | 0 | 0 |
| 6 | Goji v2 | 1,139 | 1,440 | 8 |
| 7 | Beego | 1,575 | 352 | 3 |
| 8 | Chi | 3,071 | 2,504 | 9 |
| 9 | Macaron | 3,130 | 2,864 | 15 |
| 10 | HttpTreeMux | 3,172 | 3,144 | 13 |
| 11 | GorillaMux | 3,550 | 3,272 | 13 |
| 12 | GoRestful | 5,330 | 7,008 | 20 |

### Parameter read and write

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 36.79 | 0 | 0 |
| 2 | Gin | 40.52 | 0 | 0 |
| 3 | HttpRouter | 52.9 | 32 | 1 |
| 4 | Echo | 62.94 | 8 | 1 |
| 5 | **Zinc** | 73.1 | 0 | 0 |
| — | Fiber† | 197.3 | 0 | 0 |
| 6 | HttpTreeMux | 302.6 | 352 | 3 |
| 7 | Beego | 560.8 | 360 | 4 |
| 8 | Chi | 571.9 | 704 | 4 |
| 9 | Goji v2 | 823.3 | 1,168 | 10 |
| 10 | GorillaMux | 1,053 | 1,152 | 8 |
| 11 | Macaron | 1,259 | 1,112 | 13 |
| 12 | GoRestful | 2,503 | 4,608 | 16 |

### GitHub static route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 29.06 | 0 | 0 |
| 2 | HttpRouter | 30.51 | 0 | 0 |
| 3 | HttpTreeMux | 36.99 | 0 | 0 |
| 4 | Gin | 41.5 | 0 | 0 |
| 5 | Echo | 44.07 | 0 | 0 |
| 6 | **Zinc** | 46.51 | 0 | 0 |
| — | Fiber† | 256.6 | 0 | 0 |
| 7 | Chi | 336.2 | 368 | 2 |
| 8 | Beego | 542.8 | 352 | 3 |
| 9 | Goji v2 | 753.8 | 1,120 | 7 |
| 10 | Macaron | 811.7 | 728 | 8 |
| 11 | GorillaMux | 2,124 | 848 | 7 |
| 12 | GoRestful | 5,270 | 4,792 | 14 |

### GitHub parameter route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | Gin | 70.12 | 0 | 0 |
| 2 | **Zinc** | 80.39 | 0 | 0 |
| 3 | Echo | 82.83 | 0 | 0 |
| 4 | BunRouter | 102.2 | 0 | 0 |
| 5 | HttpRouter | 130.4 | 96 | 1 |
| 6 | HttpTreeMux | 383.2 | 384 | 4 |
| — | Fiber† | 403.6 | 0 | 0 |
| 7 | Chi | 683 | 704 | 4 |
| 8 | Beego | 704 | 352 | 3 |
| 9 | Goji v2 | 974.8 | 1,216 | 10 |
| 10 | Macaron | 1,222 | 1,064 | 10 |
| 11 | GorillaMux | 4,015 | 1,168 | 8 |
| 12 | GoRestful | 6,268 | 4,696 | 15 |

### Google+ static route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 13.12 | 0 | 0 |
| 2 | HttpRouter | 15.78 | 0 | 0 |
| 3 | HttpTreeMux | 23.46 | 0 | 0 |
| 4 | Echo | 29.01 | 0 | 0 |
| 5 | Gin | 32.27 | 0 | 0 |
| 6 | **Zinc** | 39.72 | 0 | 0 |
| — | Fiber† | 152 | 0 | 0 |
| 7 | Chi | 319.3 | 368 | 2 |
| 8 | Beego | 497.5 | 352 | 3 |
| 9 | GorillaMux | 713.3 | 848 | 7 |
| 10 | Goji v2 | 758.5 | 1,120 | 7 |
| 11 | Macaron | 818.7 | 728 | 8 |
| 12 | GoRestful | 2,405 | 4,272 | 14 |

### Google+ parameter route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 25.8 | 0 | 0 |
| 2 | Gin | 43.99 | 0 | 0 |
| 3 | Echo | 45.71 | 0 | 0 |
| 4 | **Zinc** | 74.48 | 0 | 0 |
| 5 | HttpRouter | 77.16 | 64 | 1 |
| — | Fiber† | 208.5 | 0 | 0 |
| 6 | HttpTreeMux | 467.6 | 352 | 3 |
| 7 | Beego | 588.4 | 352 | 3 |
| 8 | Chi | 596.2 | 704 | 4 |
| 9 | Goji v2 | 813.6 | 1,136 | 8 |
| 10 | Macaron | 1,130 | 1,064 | 10 |
| 11 | GorillaMux | 1,404 | 1,152 | 8 |
| 12 | GoRestful | 2,753 | 4,616 | 15 |

### Google+ two-parameter route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 55.28 | 0 | 0 |
| 2 | Gin | 56.34 | 0 | 0 |
| 3 | Echo | 64.83 | 0 | 0 |
| 4 | **Zinc** | 84.56 | 0 | 0 |
| 5 | HttpRouter | 89.35 | 64 | 1 |
| 6 | HttpTreeMux | 368.9 | 384 | 4 |
| — | Fiber† | 409.9 | 0 | 0 |
| 7 | Chi | 642 | 704 | 4 |
| 8 | Beego | 718.4 | 352 | 3 |
| 9 | Goji v2 | 1,003 | 1,216 | 11 |
| 10 | Macaron | 1,155 | 1,064 | 10 |
| 11 | GoRestful | 2,839 | 4,712 | 15 |
| 12 | GorillaMux | 2,971 | 1,168 | 8 |

### Parse static route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | HttpRouter | 16.56 | 0 | 0 |
| 2 | BunRouter | 20.34 | 0 | 0 |
| 3 | Echo | 29.26 | 0 | 0 |
| 4 | Gin | 31.38 | 0 | 0 |
| 5 | HttpTreeMux | 35.2 | 0 | 0 |
| 6 | **Zinc** | 42.05 | 0 | 0 |
| — | Fiber† | 165.8 | 0 | 0 |
| 7 | Chi | 312 | 368 | 2 |
| 8 | Beego | 510.4 | 352 | 3 |
| 9 | Goji v2 | 727.6 | 1,120 | 7 |
| 10 | Macaron | 783.6 | 728 | 8 |
| 11 | GorillaMux | 809.7 | 848 | 7 |
| 12 | GoRestful | 2,811 | 4,792 | 14 |

### Parse parameter route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 37.36 | 0 | 0 |
| 2 | Gin | 38.89 | 0 | 0 |
| 3 | Echo | 63.49 | 0 | 0 |
| 4 | HttpRouter | 64.98 | 64 | 1 |
| 5 | **Zinc** | 86.11 | 0 | 0 |
| — | Fiber† | 219.1 | 0 | 0 |
| 6 | HttpTreeMux | 278.7 | 352 | 3 |
| 7 | Beego | 570.7 | 352 | 3 |
| 8 | Chi | 604.2 | 704 | 4 |
| 9 | Goji v2 | 828.2 | 1,168 | 9 |
| 10 | GorillaMux | 1,037 | 1,152 | 8 |
| 11 | Macaron | 1,052 | 1,064 | 10 |
| 12 | GoRestful | 3,055 | 5,112 | 15 |

### Parse two-parameter route

| Rank | Router | ns/op | B/op | allocs/op |
| ---: | --- | ---: | ---: | ---: |
| 1 | BunRouter | 40.99 | 0 | 0 |
| 2 | Gin | 42.94 | 0 | 0 |
| 3 | Echo | 44.6 | 0 | 0 |
| 4 | **Zinc** | 74.87 | 0 | 0 |
| 5 | HttpRouter | 75.21 | 64 | 1 |
| — | Fiber† | 261.4 | 0 | 0 |
| 6 | HttpTreeMux | 334.1 | 384 | 4 |
| 7 | Chi | 579.3 | 704 | 4 |
| 8 | Beego | 599.5 | 352 | 3 |
| 9 | Goji v2 | 719.3 | 1,152 | 8 |
| 10 | Macaron | 1,097 | 1,064 | 10 |
| 11 | GorillaMux | 1,287 | 1,168 | 8 |
| 12 | GoRestful | 3,376 | 5,536 | 15 |

## Reproduce this run

Use the pinned upstream commit, apply the local Zinc adapter, and point its `go.mod` replacement at the Zinc checkout under test. The raw 1,050 benchmark samples, memory estimates, and correctness output are saved locally in `benchmarks/results/gin-20260925/`.

```bash
GOTOOLCHAIN=go1.27.1 go test -count=1 ./...
GOTOOLCHAIN=go1.27.1 go test -run='^$' -bench=. -benchmem -benchtime=100ms -count=5 -timeout=20m ./...
```
