# 0.3.0 performance recovery plan

Status: active recovery goal, 24 September 2026. Hold the `0.3.0` tag while the measured regressions are investigated. The hardening fixes and merged dependency updates stay in `dev`.

## Starting point

The earlier release comparison measured pre-hardening `378674b3edd2fde8a2e095076463fa4760100b71` against the merged candidate `fb7e5be495db0e5c370735e83732e96713a3898c`. It used the same benchmark harness and Go 1.27.1 on an Apple M1 Pro, with ten alternating samples of all 342 workloads. Raw results, benchstat output, and CPU profiles were saved in the earlier local audit workspace; the combined candidate evidence is included in this branch.

| Workload | Before | Candidate | Change |
|---|---:|---:|---:|
| Hello world | 66.06 ns/op, 0 alloc | 178.45 ns/op, 1 alloc | +170% |
| Parameter route | 81.99 ns/op, 0 alloc | 200.90 ns/op, 1 alloc | +145% |
| API parameter/query/JSON | 1.019 µs/op, 7 allocs | 1.138 µs/op, 8 allocs | +12% |
| Static-file hit | 14.81 µs/op | 25.57 µs/op | +73% |
| Static-file miss | 1.532 µs/op | 12.313 µs/op | +704% |

These are in-process costs. They identify CPU and allocation regressions; they do not directly predict end-to-end latency. The unchanged Chi, Echo, and Gin hello-world controls were stable in the same run. The parallel route benchmarks also regressed sharply and need attention when changing the response path.

The controlled pre-hardening comparison won **59/77** four-framework workload rows; the current candidate won **13/77**, confirmed in a second complete head-to-head run. All 13 current wins were already baseline wins: **46 wins were lost and none gained**. The published **62/77** is an older single-sample result at `5f77c75` with Go 1.26.1. The release target is to regain at least that published **62/77** score in repeated, same-toolchain measurements while retaining every hardening guarantee. This requires recovering the 46 lost wins and finding at least three more, so merely returning to the controlled baseline is insufficient.

Of the 46 lost wins, 25 trail the current fastest peer by at most 50 ns and 42 by at most 100 ns. A shared per-request cost is the strongest candidate for regaining many rows at once. This is a prioritization clue, not proof that one change can recover them all.

## Recovery checkpoint (24 September 2026)

The isolated response branch (`codex/performance-recovery`, `1234fdc`) has passed root and benchmark-module tests, race checks, vet, and routing fuzzing. A complete ten-sample head-to-head run on Go 1.27.1 won **52/77**, up from the merged candidate's **13/77**. The wins and all 77 medians are recorded in that branch under `audits/2026-09-24/performance-recovery/`. Hello world measured 85.93 ns/op, parameter routing 111.05 ns/op, and default 404 104.30 ns/op. This is one complete run, not the two-run release gate.

A separate static branch (`codex/static-root-reuse`, `d1db287`) retains one lazy confined `os.Root` per static mount and closes it on `Shutdown` or `App.Close`. Ten alternating samples improved static hits **26.913 → 15.640 µs/op** and misses **12.631 → 1.601 µs/op**. It passes Go 1.25 and 1.27 tests, race tests, vet, documentation checks, and confinement/lifecycle regression tests. `StaticFS` remains caller-owned; the separate `middleware.Static` implementation is unchanged. The first complete combined run won **53/77**. Static-file hits were 14.799 µs/op and misses 1.549 µs/op in that run, close to the pre-hardening controlled baseline. The `0.3.0` tag remains on hold.

This combined branch (`codex/performance-combined`) includes those response and static changes, an ASCII static-length guard, a body-allowed status fast path, and cached canonical names in header-binding plans. Its latest complete 77-row head-to-head run wins **55/77**. Intermediate full runs scored 53/77 after static reuse and 56/77 after the routing guard; near ties fluctuate between runs, so these counts are not proof that the later changes caused a net loss. Focused alternating samples reduced the mixed-case Parse API parameter route from 204 to 136 ns/op, its method mismatch from 186 to 116 ns/op, a custom group 404 from 161 to 101 ns/op, and header/query/JSON binding from 2.228 to 1.935 µs/op (25 to 17 allocations). The [combined scorecard](audits/2026-09-24/performance-recovery/COMBINED.md) links the 77 medians and raw run outputs.

The combined branch passes root and benchmark-module race tests, root vet, and the CI parallel-race benchmark check on Go 1.25, 1.26, and 1.27; Go 1.27 routing fuzzing; staticcheck; govulncheck; and the website site, example, and npm audit checks. The release score remains below 62/77, and the broader Zinc-only suite and two-run final head-to-head gate remain undone. Continue profiling default misses, method mismatches, API JSON/query, and registration paths; do not change radix matching or cache precedence without behavior tests and a route-hit/miss comparison. Two speculative router edits were already reverted after controlled comparisons failed to show a reliable net gain. The `0.3.0` tag stays on hold.

The separate [gin-gonic routing benchmark](audits/2026-09-24/external-routing-benchmark/README.md) now passes with a local Zinc adapter and adds useful route-only evidence. Zinc has zero allocations in all 16 comparable workloads, but wins 1/16 against every `net/http` implementation and 2/16 against Gin, Echo, and Chi. Ten-sample aggregate results show Zinc ahead of Gin on 157 static routes but behind Gin on the GitHub, GPlus, and Parse mixed-route sets. These are different workloads from the 77-scenario HTTP suite, not a replacement score. Profile the mixed dynamic-route path as a candidate for further improvement without sacrificing the already strong static path or correctness.

## 1. Isolate the regressions

Use the saved benchmark binary pair and profiles as the reference. Repeat only the affected workloads at each relevant hardening commit, with the final benchmark harness and one Go toolchain throughout. This distinguishes the response-writer change in #65, any added dispatch work, and static confinement in #63. Check that each comparison exercises equivalent behavior; do not treat an old insecure or incorrect behavior as a feature to restore. Track which of the 46 lost head-to-head rows each change affects, as well as absolute time and allocations.

**First attribution completed:** ten alternating 100 ms samples on Go 1.27.1, with the identical final benchmark harness, compared #64 head `2076198` immediately before response hardening with #65 head `f5ba8a0` immediately after it. Hello world changed **67.06 → 173.85 ns/op (+159%)**, parameter routing **86.09 → 197.85 ns/op (+130%)**, mixed large parameter routes **114.6 → 228.4 ns/op (+99%)**, and parallel static routing **12.34 → 46.40 ns/op (+276%)**. The corresponding Chi, Echo, and Gin hello-world and parallel controls did not show significant changes. API parameter/query/JSON rose 10%; middleware chain timing was effectively unchanged. The common response path introduced by #65 is therefore a confirmed dominant source of the tiny-handler regression. This does not prove every route loss is caused by that PR. Raw outputs and benchstat were saved in the earlier local audit workspace.

For a fast feedback set, include hello world, static and parameter routes, middleware chain, API binding, parallel static and parameter routes, static-file hit and miss, and the cache hot/working-set/unique cases. Capture ns/op, B/op, allocs/op, and a CPU profile for any case whose cost is still unexplained. Record intermediate results in the audit evidence directory.

## 2. Reduce common response-path cost

Start with `Context.prepareResponse`, header access, the instrumented writer, and context setup/release. These ordinary request costs affect far more of the 77 comparison rows than filesystem serving does. The candidate hello-world profile spends noticeable CPU in response preparation, header canonicalization, writer setup, and allocation. Measure one change at a time:

- Avoid redundant `Header()` lookups and normalization for known, correctly canonicalized constant header names, while preserving caller-supplied header behavior.
- Check whether writer capability detection, interface dispatch, and commitment bookkeeping can be done once per request or shortened for ordinary writes.
- Review prefix-middleware scanning when no prefix middleware is registered; prove its cost separately before changing it.
- Keep response headers owned by each request. Do not regain zero allocations by reusing mutable header value slices across requests. Treat any remaining allocation as an explicit, measured correctness cost.

Preserve tests for first-write commitment, informational statuses, flushing, hijacking, middleware writers, one terminal error, header isolation, and race safety. Benchmark ordinary static and parameter routes, misses, API paths, parallel runs, and peer wins after each substantive edit. Open a focused performance PR with a before/after table rather than mixing it with static-file work. If that PR cannot restore most route wins, inspect route dispatch and middleware sequencing separately before changing the radix representation.

## 3. Reduce static-file cost without weakening confinement

The macOS static-miss profile spends most of its time opening a new filesystem root under `os.OpenInRoot` on every request. Prototype a way to retain a confined `os.Root` for repeated opens. First decide who owns and closes it, how that lifecycle fits an `http.Handler`-style app, what happens on registration failure and shutdown, and how concurrent requests behave while closing. If no sound lifecycle is available, keep the current secure implementation and document the cost rather than adding an unbounded or implicit descriptor cache.

Validate files and directories, misses, in-root symlinks, escaping symlinks, path encodings, races during path replacement, descriptor cleanup, and repeated/concurrent requests. Benchmark hits and misses separately, including an ordinary `StaticFS` supplied by callers. Keep this as a separate PR so the syscall tradeoff is reviewable. This work matters even if it changes few head-to-head wins: the measured 7× static miss cost is a release concern on its own.

## 4. Release gate

For each candidate fix, run at least ten alternating samples and use `benchstat` for the affected workloads, as required by `CONTRIBUTING.md`. Compare the final combined revision with **both** the merged hardening candidate and the pre-hardening baseline using the same Go toolchain, harness, host, and response assertions. After focused results stabilize, run the full 77-row peer suite twice and the broader Zinc-only suite once. Recheck allocations and parallel throughput. Run root and benchmark module tests with `-race`, `go vet`, the routing fuzz target, and the repository's CI matrix before tagging.

The performance gate is **at least 62 outright lowest-median wins out of the same 77 peer rows** in both final runs. Keep the benchmark fixtures and peers unchanged while pursuing that number; count a tie as no outright win. Also aim to bring tiny-handler and static-file costs close to the pre-hardening baseline. As a working review threshold, investigate any remaining **greater than 25%** slowdown in common handlers or static-file hits, the static-miss regression, and any material throughput or allocation regression. This is a trigger for investigation, not permission to weaken security. Record unavoidable costs and make an explicit release decision from the final numbers. Do not claim a performance improvement from normal sample noise.

The correctness gate is every existing hardening regression test passing, including auth inheritance, body limits, response commitment, session expiry/rotation, bounded middleware state, trusted-proxy handling, routing contracts, and binding errors. Do not introduce benchmark-only branches or shared mutable response state. The Go support matrix and vulnerability checks must remain green. Update the public historical benchmark page with a clearly dated current result only after the final candidate and all checks are accepted.

Linux performance measurement is **optional**. The existing validation workflow already runs functional and race checks on `ubuntu-latest` through GitHub Actions. A Linux benchmark would require a comparable before/after run on the same Linux runner or machine; it does not require Cloudflare or a paid Cloudflare plan. It can help check whether the macOS filesystem syscall cost generalizes, but it is not a prerequisite for starting or completing the local optimization work. Check runner availability and billing before scheduling any extra hosted benchmark job.

Once the fixes and evidence are reviewed, tag the exact validated `dev` commit as `0.3.0` only after the release decision. No tag should point to the currently measured candidate by default.
