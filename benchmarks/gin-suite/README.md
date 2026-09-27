# Zinc in the upstream Gin routing suite

`zinc_test.go.txt` is Zinc's adapter for [gin-gonic/go-http-routing-benchmark](https://github.com/gin-gonic/go-http-routing-benchmark). It's stored as `.txt` because it belongs to the suite's package, not this module. The report it produces is [GIN_BENCHMARK.md](../../GIN_BENCHMARK.md).

```bash
git clone https://github.com/gin-gonic/go-http-routing-benchmark gin-suite
cd gin-suite
git checkout ff3cdf55eccd0aa6a272991db9611a86c734dc51
cp <zinc>/benchmarks/gin-suite/zinc_test.go.txt zinc_test.go
go mod edit -require github.com/0mjs/zinc@v0.0.0 -replace github.com/0mjs/zinc=<zinc>
go mod tidy
go test -count=1 ./...
go test -run='^$' -bench=. -benchmem -benchtime=100ms -count=5 -timeout=60m . > full.log
```

Then, from Zinc's `benchmarks` directory:

```bash
go run ./cmd/zincbench gin-report -log full.log -label 'v0.5.0 (`<commit>`)' -date '<date>' -go <go version> \
  -previous <an earlier full.log> -previous-label v0.4.0 -o ../GIN_BENCHMARK.md
```

Run it on a quiet machine on mains power: the suite takes about 15 minutes and every router's rows go in the report.
