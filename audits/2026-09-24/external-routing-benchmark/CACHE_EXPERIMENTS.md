# Route-cache experiments after the external routing run

These are scratch experiments, not changes to the recovery branch. On 24 September 2026, ten alternating baseline/variant pairs ran four Zinc aggregate workloads on Go 1.27.1 and an Apple M1 Pro. Each benchmark used 100 ms samples. The saved CPU profile is `parse-cpu.pprof.gz`; the raw alternating outputs are `cache-bypass-alternating.log.gz` and `low-freeze-alternating.log.gz`.

In the Parse aggregate CPU profile, `Router.dispatchInto` accounts for 53.6% cumulative sampled time and `RouteCache.getWithMask` for 21.5%. Context reset/release and response-writer setup also show up. Cumulative profile percentages overlap and should not be added together.

| Scratch change | Workload | Baseline median | Variant median | Change | Allocations per fixture pass |
|---|---|---:|---:|---:|---|
| Bypass dispatch cache below 64 dynamic routes | GPlusAll | 1,247.5 ns | 2,678.0 ns | +114.7% | 0 → 10 |
| Bypass dispatch cache below 64 dynamic routes | ParseAll | 2,115.0 ns | 4,349.5 ns | +105.7% | 0 → 16 |
| Lower `routeCacheMinRoutes` from 64 to 8 | GPlusAll | 1,249.5 ns | 1,148.0 ns | −8.1% | 0 → 0 |
| Lower `routeCacheMinRoutes` from 64 to 8 | ParseAll | 2,098.5 ns | 1,824.5 ns | −13.1% | 0 → 0 |
| Lower `routeCacheMinRoutes` from 64 to 8 | GithubAll | 19,527.5 ns | 19,603.5 ns | +0.4% | 0 → 0 |
| Lower `routeCacheMinRoutes` from 64 to 8 | StaticAll | 8,041.0 ns | 8,086.5 ns | +0.6% | 0 → 0 |

The bypass is rejected: it doubles the small mixed-route workloads and adds allocations. Lowering the minimum is a candidate for deeper work, not a ready fix. The constant also affects cache promotion and dynamic `Find` behavior; verify hot/cold/working-set/unique-path, method mismatch, concurrency/race, routing equivalence, and the full 77-scenario suite before considering it for production. Preserve the static and 20-parameter wins while investigating the larger mixed-route gap.
