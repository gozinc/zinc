package main

import (
	"strings"
	"testing"
)

const sampleOutput = `goos: darwin
goarch: arm64
pkg: github.com/0mjs/zinc/benchmarks
cpu: Apple M1 Pro
BenchmarkHelloWorld/Zinc-8        1240612    87.98 ns/op   16 B/op   1 allocs/op
BenchmarkHelloWorld/Zinc-8        1365966    88.40 ns/op   16 B/op   1 allocs/op
BenchmarkHelloWorld/Gin-8         1000000   136.9 ns/op    48 B/op   1 allocs/op
BenchmarkHelloWorld/Echo-8        1000000   152.8 ns/op    16 B/op   1 allocs/op
BenchmarkHelloWorld/Chi-8         1000000   180.8 ns/op   368 B/op   2 allocs/op
BenchmarkScenarioRouteSetAll/GitHubAPI203/Zinc-8   100000   138.8 ns/op   0 B/op   0 allocs/op
BenchmarkScenarioRouteSetAll/GitHubAPI203/Gin-8    100000   164.1 ns/op   0 B/op   0 allocs/op
BenchmarkScenarioRouteSetAll/GitHubAPI203/Echo-8   100000   208.3 ns/op   0 B/op   0 allocs/op
BenchmarkScenarioRouteSetAll/GitHubAPI203/Chi-8    100000   554.4 ns/op 704 B/op   4 allocs/op
BenchmarkNotFound/Zinc-8          1000000   107.4 ns/op    16 B/op   1 allocs/op
BenchmarkHTTPMiddleware/UseHTTP-8  1000000   112.1 ns/op     0 B/op   0 allocs/op
PASS
`

func TestParseOutput(t *testing.T) {
	scenarios, zincOnly, header, err := parseOutput(strings.NewReader(sampleOutput))
	if err != nil {
		t.Fatal(err)
	}
	if header["cpu"] != "Apple M1 Pro" || header["goarch"] != "arm64" {
		t.Fatalf("header = %v", header)
	}
	if len(scenarios) != 2 {
		t.Fatalf("scenarios = %d, want 2 (NotFound lacks rivals and must be dropped)", len(scenarios))
	}
	hello := scenarios["HelloWorld"]
	if got := hello["Zinc"].NS; len(got) != 2 || got[1] != 88.40 {
		t.Fatalf("HelloWorld/Zinc samples = %v", got)
	}
	if hello["Chi"].Allocs != 2 || hello["Gin"].Bytes != 48 {
		t.Fatalf("HelloWorld medians = %+v", hello)
	}
	if _, ok := scenarios["ScenarioRouteSetAll/GitHubAPI203"]; !ok {
		t.Fatal("nested scenario name not preserved")
	}
	if _, ok := zincOnly["HTTPMiddleware/UseHTTP"]; !ok {
		t.Fatalf("zincOnly = %v", zincOnly)
	}
}

func TestWins(t *testing.T) {
	scenarios, _, _, _ := parseOutput(strings.NewReader(sampleOutput))
	r := &Run{Scenarios: scenarios}
	if got := r.wins(); got != 2 {
		t.Fatalf("wins = %d, want 2", got)
	}
}

func TestSuitePatternCoversEveryFunction(t *testing.T) {
	if len(suite) != 48 {
		t.Fatalf("suite has %d benchmark functions, want 48", len(suite))
	}
	if !strings.HasPrefix(suitePattern(), "^Benchmark(HelloWorld|") {
		t.Fatalf("pattern = %s", suitePattern())
	}
}
