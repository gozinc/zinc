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

The hardening behavior remains in place. Root and benchmark-module tests and race tests pass on Go 1.27; root tests pass on Go 1.25. Vet, routing-equivalence fuzzing, and the documentation API/link check pass. Static tests cover root reuse, delayed directory creation, symlinks and path replacement, concurrent reads and close, and cleanup by `Close` and `Shutdown`. The retained root keeps serving its original directory after that path is renamed; the static-files guide documents this choice.

The release decision remains open. Run the Go 1.26 matrix, staticcheck, vulnerability and full website checks, the broader Zinc-only suite, and two final head-to-head runs on an idle host before reconsidering the tag. Investigate the remaining default-miss/method-mismatch and API gaps without removing response isolation or filesystem confinement. `middleware.Static` still uses per-request `os.OpenInRoot`; it needs separate lifecycle design if its throughput is part of the 0.3 release bar. No `0.3.0` tag should be cut from these results.
