# Next 0.3 performance pass

Open the [interactive benchmark dashboard](benchmark-dashboard.html) for the complete 77-row Zinc/Gin/Chi/Echo leaderboard, loss percentages, and the saved Gin-style routing rankings. The HTML is self-contained and separates the original external snapshot from current-code paired checks.

This pass starts from the combined recovery branch in PR #72 (`2ea63f3`). On 24 September 2026, a repeat of the 77-scenario Zinc/Gin/Echo/Chi suite gave Zinc **57/77** wins, versus 55/77 in the previous run of essentially the same code. The two-win shift is benchmark noise near ties, not a code improvement. Both runs use Go 1.27.1 on an Apple M1 Pro, ten 100 ms samples for each framework and scenario, and the lowest median ns/op as the winner.

The code changes in this pass are intentionally small:

1. Default 405 responses place the request-owned `Allow` and default `Content-Type` values in one backing allocation. Each header slice has its own capacity, so appending to one cannot alter the other. Ten alternating baseline/variant comparisons show **9.6–9.9% lower ns/op** on the four measured method-mismatch scenarios, with **2 to 1 allocations/op**. Successful static and parameter routes were statistically unchanged. Two 404 controls moved +1.2–1.4% in the first focused comparison, so the branches were rearranged. A second ten-pair comparison of the combined candidate found the final layout **0.4–1.0% faster on 404** and **1.1–2.1% faster on 405**, with successful-route controls unchanged.
2. A stable route cache now promotes a snapshot at eight entries, while the 64-dynamic-route threshold for `Router.Find` caching remains unchanged. Ten alternating comparisons show **12.7% lower ns/op** for the 32-path working set, **8.4%** for external GPlusAll, and **13.3%** for external ParseAll. Hot, cold, unique-path, high-cardinality, phase-shift, GitHub aggregate, and static aggregate controls had no material change. Allocations were unchanged. A regression test covers promotion and overlay replacement at eight entries.

Four alternative router cache lookup orderings were rejected. They improved repeated 404 or hot parameter paths but slowed cold parameter traffic by about 5–7%, or ordinary static traffic by 3–6%. The accepted cache change leaves routing precedence and lookup order intact.

The combined branch passes root tests, root race tests, benchmark-module correctness tests, `go vet`, both staticcheck modes, and an 11-second routing-cache equivalence fuzz run (182,282 executions) on Go 1.27.1. The first combined candidate, before the final 404 branch layout, won **58/77**. The complete **final-code result is 57/77**, with 77 rows and ten samples per framework in every row. Compared with the fresh 57/77 baseline, the ParamsAny24 method-mismatch row became a Zinc win, while a near-tied large JSON response row moved to Echo. The winner of the multipart binding row also changed between Echo and Gin without changing Zinc's score. These close rows illustrate why the focused alternating comparisons matter more than the single-run win-count delta.

| 77-row scenario | Baseline Zinc | Final Zinc | Final result |
|---|---:|---:|---|
| Large route-set 405 | 119.95 ns, 2 allocs | 106.00 ns, 1 alloc | Gin still wins by 15.5% |
| Static157 405 | 124.80 ns, 2 allocs | 111.90 ns, 1 alloc | Gin still wins by 10.0% |
| ParamsAny24 405 | 124.10 ns, 2 allocs | 111.15 ns, 1 alloc | Zinc wins |
| Default 404 | 107.10 ns, 1 alloc | 107.40 ns, 1 alloc | Gin wins |
| Large route-set 404 | 104.80 ns, 1 alloc | 104.75 ns, 1 alloc | Gin wins |

The **62/77 two-run release gate remains open**; no 0.3.0 tag follows from this pass. Query parsing and JSON binding remain separate candidates for profiling, with the 77-row baseline showing QueryParams 32.4% behind Echo, APIBindJSONHappyPath 9.0% behind Echo, and APIBindInvalidJSON 31.1% behind Gin. Query parsing semantics and input limits should be preserved in any change. Default 404 is still 78.9% slower than Gin in the final run, but cache lookup reorderings traded that gain for regressions on successful routes. That path needs a different design rather than another broad ordering change.

Raw data in this directory:

- `next-pass-baseline-77.log.gz` and `next-pass-candidate-77.log.gz`: full 77-row runs.
- `next-pass-intermediate-77.log.gz`: the 58/77 complete run before the final 404 branch layout.
- `next-pass-baseline-77.json` and `next-pass-candidate-77.json`: all medians, winners, allocations, and sample counts.
- `next-pass-focused-405-before.log.gz` and `next-pass-focused-405-after.log.gz`: ten alternating comparisons.
- `next-pass-focused-cache-before.log.gz` and `next-pass-focused-cache-after.log.gz`: ten alternating internal cache comparisons.
- `next-pass-focused-external-before.log.gz` and `next-pass-focused-external-after.log.gz`: ten alternating external aggregate comparisons.
- `next-pass-focused-layout-before.log.gz` and `next-pass-focused-layout-after.log.gz`: ten alternating 404/405 layout comparisons.
