# Response-path recovery checkpoint

The merged 0.3 hardening candidate (`fb7e5be`) won **13/77** head-to-head benchmark rows against Gin, Echo, and Chi. This response-only branch at `1234fdc` wins **52/77**. Both complete runs use the same 77 four-framework scenarios, Go 1.27.1, Apple M1 Pro, ten 100 ms samples per framework and scenario, and the lowest median ns/op as an outright win. The compressed raw output is saved as `merged-head-to-head.log.gz` and `response-head-to-head.log.gz`; `response-head-to-head.json` records all 77 response-branch medians and winners.

| Scenario | Merged candidate | Response branch | Change |
|---|---:|---:|---:|
| Hello world | 169.30 ns/op | 85.93 ns/op | -49% |
| Parameter route | 191.75 ns/op | 111.05 ns/op | -42% |
| Middleware chain | 357.20 ns/op | 242.45 ns/op | -32% |
| Default 404 | 158.25 ns/op | 104.30 ns/op | -34% |
| Large-set method mismatch | 181.00 ns/op | 121.55 ns/op | -33% |

The response branch sets canonical constant headers directly while keeping their value slices request-owned. Ordinary `String` and default 404/405 responses use the already-owned instrumented writer; custom writers, HEAD, and uncommon statuses retain the general response path. No hardening behavior was removed. Root and benchmark-module tests, race tests, `go vet`, and a ten-second routing-equivalence fuzz run passed on this code. Focused ten-sample comparisons and the full-run data were used to reject two router experiments that did not offer a reliable gain.

This is **an intermediate checkpoint, not a release result**. The pre-hardening controlled baseline won 59/77; the published 62/77 came from a different historical toolchain and single-sample run. The current branch is still ten wins short of the 62/77 repeated-run release target. Static-file hit and miss remain materially slower than the pre-hardening baseline because the confined filesystem opens a fresh root on each request. A separate static-filesystem change and final combined reruns are required before a `0.3.0` tag decision.
