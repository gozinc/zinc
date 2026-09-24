# Gin-style routing benchmark tables with Zinc

These reproduce the categories and columns of [Gin’s benchmark page](https://gin-gonic.com/en/docs/benchmarks/) using **our single contemporaneous run** on 24 September 2026 (Go 1.27.1, Apple M1 Pro). Upstream suite `ff3cdf55`, local Zinc recovery production code `d0070d4`. All timings are ns/op; lower is better. Data: [full-results.csv](full-results.csv) and [full.log.gz](full.log.gz). The GitHub table serves as both summary and GitHub API result, as those tables have identical data on Gin’s page.

`Fiber†` uses a separate `fasthttp` harness. Its numeric position is shown for fidelity to the source page but should not be treated as a direct `net/http` comparison. The `*All` rows measure a complete fixture pass. Route-structure memory is a single post-GC estimate, not RSS.

## Memory: Static routes — 157

| Router | Bytes |
|---|---:|
| HttpRouter | 21,680 |
| Gin | 34,408 |
| Macaron | 37,024 |
| Zinc | 39,376 |
| BunRouter | 51,232 |
| Fiber† | 59,248 |
| http.ServeMux | 69,216 |
| HttpTreeMux | 73,448 |
| Chi | 83,160 |
| Echo | 92,104 |
| Beego | 98,824 |
| Goji v2 | 118,000 |
| GorillaMux | 599,496 |
| GoRestful | 819,688 |

## Memory: GitHub API — 203 routes

| Router | Bytes |
|---|---:|
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
| Fiber† | 163,832 |
| GoRestful | 1,270,864 |
| GorillaMux | 1,319,680 |

## Memory: Google+ API — 13 routes

| Router | Bytes |
|---|---:|
| HttpRouter | 2,776 |
| Gin | 4,576 |
| BunRouter | 7,360 |
| HttpTreeMux | 7,440 |
| Chi | 8,008 |
| Goji v2 | 8,096 |
| Zinc | 8,400 |
| Macaron | 8,672 |
| Beego | 10,256 |
| Fiber† | 10,840 |
| Echo | 11,096 |
| GorillaMux | 68,000 |
| GoRestful | 72,520 |

## Memory: Parse API — 26 routes

| Router | Bytes |
|---|---:|
| HttpRouter | 5,024 |
| HttpTreeMux | 7,848 |
| Gin | 7,896 |
| BunRouter | 9,336 |
| Chi | 9,656 |
| Zinc | 13,272 |
| Macaron | 13,704 |
| Echo | 13,944 |
| Fiber† | 15,352 |
| Goji v2 | 16,064 |
| Beego | 19,256 |
| GorillaMux | 105,384 |
| GoRestful | 121,184 |

## GitHub API — 203 routes

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | Gin | 13,463 | 0 | 0 |
| 2 | BunRouter | 14,880 | 0 | 0 |
| 3 | Echo | 15,937 | 0 | 0 |
| 4 | **Zinc** | **19,095** | 0 | 0 |
| 5 | HttpRouter | 21,639 | 13,792 | 167 |
| 6 | HttpTreeMux | 68,908 | 65,856 | 671 |
| 7 | Beego | 126,311 | 71,456 | 609 |
| 8 | Chi | 133,837 | 130,817 | 740 |
| 9 | Fiber† | 155,976 | 0 | 0 |
| 10 | Macaron | 164,178 | 147,784 | 1,624 |
| 11 | Goji v2 | 317,167 | 313,744 | 3,712 |
| 12 | GoRestful | 1,220,482 | 1,006,744 | 3,009 |
| 13 | GorillaMux | 1,815,907 | 225,667 | 1,588 |

## Google+ API — 13 routes

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | BunRouter | 476.8 | 0 | 0 |
| 2 | Gin | 602.4 | 0 | 0 |
| 3 | Echo | 645.6 | 0 | 0 |
| 4 | HttpRouter | 968.2 | 640 | 11 |
| 5 | **Zinc** | **1,244** | 0 | 0 |
| 6 | Fiber† | 3,522 | 0 | 0 |
| 7 | HttpTreeMux | 3,643 | 4,032 | 38 |
| 8 | Beego | 7,525 | 4,576 | 39 |
| 9 | Chi | 7,532 | 8,480 | 48 |
| 10 | Macaron | 10,273 | 9,464 | 104 |
| 11 | Goji v2 | 11,140 | 15,120 | 115 |
| 12 | GorillaMux | 21,673 | 14,448 | 102 |
| 13 | GoRestful | 38,469 | 60,720 | 193 |

## Parse API — 26 routes

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | BunRouter | 812.8 | 0 | 0 |
| 2 | Gin | 1,020 | 0 | 0 |
| 3 | Echo | 1,046 | 0 | 0 |
| 4 | HttpRouter | 1,433 | 640 | 16 |
| 5 | **Zinc** | **2,100** | 0 | 0 |
| 6 | HttpTreeMux | 5,899 | 5,728 | 51 |
| 7 | Fiber† | 6,095 | 0 | 0 |
| 8 | Chi | 13,316 | 14,944 | 84 |
| 9 | Beego | 13,756 | 9,152 | 78 |
| 10 | Macaron | 20,490 | 18,928 | 208 |
| 11 | Goji v2 | 20,690 | 29,456 | 199 |
| 12 | GorillaMux | 40,896 | 26,960 | 198 |
| 13 | GoRestful | 82,003 | 131,728 | 380 |

## Static routes — 157

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | HttpRouter | 5,666 | 0 | 0 |
| 2 | **Zinc** | **8,010** | 0 | 0 |
| 3 | HttpTreeMux | 8,371 | 0 | 0 |
| 4 | BunRouter | 8,579 | 0 | 0 |
| 5 | Gin | 9,736 | 0 | 0 |
| 6 | Echo | 9,982 | 0 | 0 |
| — | http.ServeMux (baseline) | 21,086 | 0 | 0 |
| 7 | Fiber† | 40,793 | 0 | 0 |
| 8 | Chi | 61,326 | 57,776 | 314 |
| 9 | Beego | 92,534 | 55,264 | 471 |
| 10 | Macaron | 125,441 | 114,296 | 1,256 |
| 11 | Goji v2 | 130,642 | 175,840 | 1,099 |
| 12 | GorillaMux | 449,340 | 133,138 | 1,099 |
| 13 | GoRestful | 673,493 | 677,824 | 2,193 |

## Single parameter

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | BunRouter | 17.7 | 0 | 0 |
| 2 | Echo | 26.35 | 0 | 0 |
| 3 | Gin | 30.87 | 0 | 0 |
| 4 | HttpRouter | 47.69 | 32 | 1 |
| 5 | **Zinc** | **55.29** | 0 | 0 |
| 6 | Fiber† | 157.8 | 0 | 0 |
| 7 | HttpTreeMux | 277.4 | 352 | 3 |
| 8 | Beego | 513.6 | 352 | 3 |
| 9 | Chi | 552.3 | 704 | 4 |
| 10 | Goji v2 | 749 | 1,136 | 8 |
| 11 | GorillaMux | 1,019 | 1,152 | 8 |
| 12 | Macaron | 1,052 | 1,064 | 10 |
| 13 | GoRestful | 2,399 | 4,600 | 15 |

## Five parameters

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | Gin | 59.33 | 0 | 0 |
| 2 | Echo | 69.11 | 0 | 0 |
| 3 | **Zinc** | **70.64** | 0 | 0 |
| 4 | BunRouter | 78.72 | 0 | 0 |
| 5 | HttpRouter | 154.1 | 160 | 1 |
| 6 | Fiber† | 347.8 | 0 | 0 |
| 7 | HttpTreeMux | 551.6 | 576 | 6 |
| 8 | Beego | 656.8 | 352 | 3 |
| 9 | Chi | 740.5 | 704 | 4 |
| 10 | Goji v2 | 848 | 1,200 | 8 |
| 11 | Macaron | 1,134 | 1,064 | 10 |
| 12 | GorillaMux | 1,626 | 1,216 | 8 |
| 13 | GoRestful | 2,715 | 4,712 | 15 |

## Twenty parameters

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | **Zinc** | **125** | 0 | 0 |
| 2 | Gin | 174.1 | 0 | 0 |
| 3 | Echo | 203.4 | 0 | 0 |
| 4 | BunRouter | 382.5 | 0 | 0 |
| 5 | HttpRouter | 495.4 | 704 | 1 |
| 6 | Fiber† | 702.2 | 0 | 0 |
| 7 | Goji v2 | 1,133 | 1,440 | 8 |
| 8 | Beego | 1,523 | 352 | 3 |
| 9 | Chi | 3,091 | 2,504 | 9 |
| 10 | Macaron | 3,093 | 2,864 | 15 |
| 11 | HttpTreeMux | 3,105 | 3,144 | 13 |
| 12 | GorillaMux | 3,511 | 3,272 | 13 |
| 13 | GoRestful | 5,250 | 7,008 | 20 |

## Parameter read and write

| Rank | Router | ns/op | B/op | allocs/op |
|---:|---|---:|---:|---:|
| 1 | BunRouter | 36.09 | 0 | 0 |
| 2 | Gin | 38.86 | 0 | 0 |
| 3 | HttpRouter | 52.25 | 32 | 1 |
| 4 | Echo | 68.94 | 8 | 1 |
| 5 | **Zinc** | **72.39** | 0 | 0 |
| 6 | Fiber† | 176.3 | 0 | 0 |
| 7 | HttpTreeMux | 294.4 | 352 | 3 |
| 8 | Beego | 539.6 | 360 | 4 |
| 9 | Chi | 560.6 | 704 | 4 |
| 10 | Goji v2 | 809.9 | 1,168 | 10 |
| 11 | GorillaMux | 1,043 | 1,152 | 8 |
| 12 | Macaron | 1,227 | 1,112 | 13 |
| 13 | GoRestful | 2,462 | 4,608 | 16 |
