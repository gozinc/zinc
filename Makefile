.PHONY: dev
dev:
	air

.PHONY: docs
docs:
	npm --prefix website run dev -- --port 3000

.PHONY: tag
tag:
ifndef version
	$(error version is not set. Usage: make tag version=<version_number>)
endif
	git tag v$(version) && git push origin v$(version)

.PHONY: test
test:
	gotestsum --format testname

.PHONY: benchmark
benchmark:
	go test -bench=. -benchmem

# Head-to-head benchmark history. Records and the dashboard live in
# benchmarks/results/, which git ignores.
#   make bench-record RELEASE=0.4.0 NOTE="router: cache promotion at 8"
#   make bench-baseline RUN=<run-id>   (or RUN=latest)
#   make bench-bundle RELEASE=0.4.0
#   make bench-zinc RELEASE=0.4.0 NOTE="P1: footguns"   (Zinc only, rivals from the baseline)
#   make bench-compare                                 (latest run against the baseline)
#   make bench-ab SCENARIOS=NotFound,API04ParamInt     (alternating rounds against the baseline commit)
.PHONY: bench-record bench-zinc bench-compare bench-ab bench-dash bench-list bench-baseline bench-bundle
bench-record:
	cd benchmarks && go run ./cmd/zincbench record -release "$(RELEASE)" -note "$(NOTE)" $(ARGS)

bench-zinc:
	cd benchmarks && go run ./cmd/zincbench record -zinc-only -release "$(RELEASE)" -note "$(NOTE)" $(ARGS)

bench-compare:
	cd benchmarks && go run ./cmd/zincbench compare $(ARGS)

bench-ab:
	cd benchmarks && go run ./cmd/zincbench ab -scenarios "$(SCENARIOS)" $(ARGS)

bench-dash:
	cd benchmarks && go run ./cmd/zincbench dash -open

bench-list:
	cd benchmarks && go run ./cmd/zincbench list

bench-baseline:
	cd benchmarks && go run ./cmd/zincbench baseline $(RUN)

bench-bundle:
	cd benchmarks && go run ./cmd/zincbench bundle -release "$(RELEASE)" $(ARGS)
