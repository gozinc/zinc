# Combined 0.3 performance recovery checkpoint

The merged hardening candidate at `fb7e5be` won **13/77** four-framework rows. The response fix alone won **52/77**; adding retained static roots won **53/77**; the ASCII static-length routing guard won **56/77**. The current combined code through `d0070d4` won **55/77** in one complete run. These scores count the outright lowest median ns/op of ten 100 ms samples for each of Zinc, Gin, Echo, and Chi on Go 1.27.1 and an Apple M1 Pro. Scores near a tie move between runs; the current revision has not met the plan's two-run **62/77** gate. The historical website 62/77 figure came from a different commit, Go toolchain, and a single sample.

The [final medians](combined-head-to-head.json) and compressed raw outputs (`combined-final-head-to-head.log.gz`, `status-fast-head-to-head.log.gz`, and `router-guard-head-to-head.log.gz`) are saved here. The merged-candidate and response-only raw runs are in this directory too. Focused alternating comparisons show the large effects more reliably than a change in outright win count:

| Change | Zinc before | Zinc after | Result |
|---|---:|---:|---|
| Common plain-text response | 149 ns/op | 87 ns/op | -42% in the focused response comparison |
| Default 404 | 164 ns/op | 103 ns/op | -37% in the focused response comparison |
| Static file hit | 26.913 µs/op | 15.640 µs/op | -41.9%, 18 to 15 allocs |
| Static file miss | 12.631 µs/op | 1.601 µs/op | -87.3%, 11 to 8 allocs |
| Mixed-case Parse API parameter route | 204 ns/op | 136 ns/op | -33.4%, 2 to 1 allocs |
| Header/query/JSON binding | 2.228 µs/op | 1.935 µs/op | -13.1%, 25 to 17 allocs |

The hardening behavior remains in place. Root and benchmark-module race tests, root vet, and the CI parallel-race benchmark check pass on Go 1.25, 1.26, and 1.27. Routing-equivalence fuzzing passed on Go 1.27. Staticcheck and govulncheck pass; govulncheck found no vulnerabilities. The website dependency install, site/API/link check, example check, and npm audit pass. Static tests cover root reuse, delayed directory creation, symlinks and path replacement, concurrent reads and close, and cleanup by `Close` and `Shutdown`. The retained root keeps serving its original directory after that path is renamed; the static-files guide documents this choice.

The separate [gin-gonic routing suite](../external-routing-benchmark/README.md) passed with the local Zinc adapter. It exposes a different tradeoff: Zinc is allocation-free on all 16 comparable route-focused rows, but wins 1/16 against every `net/http` implementation and 2/16 against Gin, Echo, and Chi. Repeated aggregate runs show strong static dispatch and a remaining mixed dynamic-route gap. Those 16 rows do not replace or combine with the 77 end-to-end framework scenarios above.

The release decision remains open. The current score is below the 62/77 gate. Investigate the remaining default-miss/method-mismatch and API gaps without removing response isolation or filesystem confinement. `middleware.Static` still uses per-request `os.OpenInRoot`; it needs separate lifecycle design if its throughput is part of the 0.3 release bar. Once a candidate reaches the target, run the broader Zinc-only suite and two final head-to-head runs on an idle host, plus hosted Linux CI, before reconsidering the tag. No `0.3.0` tag should be cut from these results.
