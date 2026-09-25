# Benchmark records

`make bench-dash` opens the local head-to-head run history. Every run has its own raw log and JSON record in `benchmarks/results/runs/`. The dashboard now groups runs by their intended Zinc release. These files are gitignored, so each checkout or worktree has a separate local results directory unless you copy or restore the records.

Record a complete four-framework run for a release:

```sh
make bench-record RELEASE=0.4.0 NOTE="baseline"
make bench-dash
```

When only Zinc was rerun, import its logs against a saved four-framework run. The new record keeps the rival source ID and the dashboard labels it as a mixed-date comparison:

```sh
cd benchmarks
go run ./cmd/zincbench import-zinc \
  -logs results/four-paths/json-happy-c-direct.log,results/four-paths/json-happy-c-nested.log \
  -rivals 20260924-210000-c6fd47f -commit 42e11d2 \
  -date 2026-09-25T00:11:00Z -release 0.3.0 \
  -note "Four-path final; Zinc Sep 25, rivals Sep 24" -go go1.27.1
```

The 0.3.0 history includes this 60/77 run. Its standalone visual report is `audits/2026-09-25/zinc-performance.html`; the independent Gin routing suite is in `benchmarks/results/gin-20260925/` and documented in `GIN_BENCHMARK.md`.

Bundle a release before removing a worktree or moving to another machine. The zip contains every run's JSON and raw log, with optional reports and independent suite data:

```sh
make bench-bundle RELEASE=0.3.0 ARGS='-include audits/2026-09-25/zinc-performance.html,benchmarks/results/four-paths,benchmarks/results/gin-20260925,BENCHMARKS.md,GIN_BENCHMARK.md'
```

The archive is written to `benchmarks/results/bundles/0.3.0.zip` by default. It is also gitignored; copy it to durable storage. To restore it in another checkout, extract the archive's `runs/` files into that checkout's `benchmarks/results/runs/`, then run `make bench-dash`. The `extras/` directory preserves included files under their original repository-relative paths.
