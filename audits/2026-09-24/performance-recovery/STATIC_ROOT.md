# Confined static-root reuse

`Static` and `Group.Static` now retain one lazy `os.Root` per mount. The root is safe for concurrent file opens and keeps symlinks confined; a mutex prevents an open from racing with close. `App.Shutdown` releases the handles after requests drain, and `App.Close` releases them when the app is served by an external server. Registration still succeeds before a directory exists, and later requests can open it once it appears.

Ten alternating 100 ms runs with Go 1.27.1 on an Apple M1 Pro used the unchanged four-framework static-file benchmarks. The [JSON medians](static-root-benchmarks.json) and compressed raw logs (`merged-static.log.gz`, `retained-root-static.log.gz`) are included.

| Zinc workload | Merged hardening candidate | Retained root | Change |
|---|---:|---:|---:|
| Static file hit | 26.913 µs/op, 18 allocs | 15.640 µs/op, 15 allocs | -41.9% |
| Static file miss | 12.631 µs/op, 11 allocs | 1.601 µs/op, 8 allocs | -87.3% |

The pre-hardening controlled baseline was 14.81 µs for a hit and 1.532 µs for a miss. Root and benchmark-module tests pass on Go 1.25 and 1.27; the Go 1.27 race suite, vet, and documentation check pass. Security tests cover repeated and concurrent opens, in-root symlinks, escape attempts, symlink replacement during reads, delayed directory creation, and cleanup through `Close` and `Shutdown`.

This branch does not alter `StaticFS` or the separate `middleware.Static` implementation. The final release gate requires combining this change with response recovery and rerunning the complete 77-scenario comparison twice on the combined revision.
