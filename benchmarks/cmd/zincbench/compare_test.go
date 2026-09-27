package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestZincOnlyPatternsSelectOnlyZincAtEachDepth(t *testing.T) {
	full, _, _, err := parseOutput(strings.NewReader(sampleOutput))
	if err != nil {
		t.Fatal(err)
	}
	patterns := zincOnlyPatterns(&Run{Scenarios: full})
	want := []string{
		"^Benchmark(HelloWorld)$/^Zinc$",
		"^Benchmark(ScenarioRouteSetAll)$/./^Zinc$",
	}
	if strings.Join(patterns, "\n") != strings.Join(want, "\n") {
		t.Fatalf("patterns = %q, want %q", patterns, want)
	}

	// go test applies one regexp per sub-benchmark level.
	matches := func(pattern, name string) bool {
		levels, parts := strings.Split(pattern, "/"), strings.Split(name, "/")
		for i, level := range levels {
			if i >= len(parts) || !regexp.MustCompile(level).MatchString(parts[i]) {
				return false
			}
		}
		return true
	}
	cases := map[string]bool{
		"BenchmarkHelloWorld/Zinc":                       true,
		"BenchmarkHelloWorld/Gin":                        false,
		"BenchmarkScenarioRouteSetAll/GitHubAPI203/Zinc": true,
		"BenchmarkScenarioRouteSetAll/GitHubAPI203/Chi":  false,
	}
	for name, want := range cases {
		got := false
		for _, p := range patterns {
			got = got || matches(p, name)
		}
		if got != want {
			t.Errorf("%s selected=%v, want %v", name, got, want)
		}
	}
}

func testRun(id string, zincNS, allocs float64, rivalNS float64) *Run {
	sc := Scenario{"Zinc": {NS: []float64{zincNS}, Bytes: 16, Allocs: allocs}}
	for _, f := range frameworks[1:] {
		sc[f] = Samples{NS: []float64{rivalNS}}
	}
	return &Run{ID: id, Scenarios: map[string]Scenario{"HelloWorld": sc},
		ZincOnly: map[string]Samples{"API04ParamInt": {NS: []float64{zincNS}, Allocs: allocs}}}
}

func TestCompareFlagsAllocationsTimeAndWins(t *testing.T) {
	th := Thresholds{SlowPct: 5, SlowNS: 15, NoisyPct: 12, Noisy: defaultNoisy}

	c := compareRuns(testRun("base", 100, 1, 110), testRun("run", 102, 1, 110))
	if d := c.Scenarios[0]; d.Flagged(th) || d.Fast(th) {
		t.Fatalf("2 ns of noise was flagged: %+v", d)
	}
	if c.allocRegressions() != 0 {
		t.Fatal("equal allocations counted as a regression")
	}

	// +5% but only +5 ns: under the absolute floor.
	if d := compareRuns(testRun("base", 100, 1, 200), testRun("run", 105.5, 1, 200)).Scenarios[0]; d.Slow(th) {
		t.Fatal("slowdown under the absolute floor was flagged")
	}

	c = compareRuns(testRun("base", 100, 1, 110), testRun("run", 130, 2, 110))
	d := c.Scenarios[0]
	if !d.Slow(th) || !d.AllocsUp() || d.BaseWin == d.Win {
		t.Fatalf("regression not flagged: %+v", d)
	}
	if c.allocRegressions() != 2 {
		t.Fatalf("allocRegressions = %d, want 2 (scenario and micro)", c.allocRegressions())
	}

	// Noisy classes need the wider margin; B/op median drift is ignored.
	if (Delta{Name: "RouteRegistrationParam", BaseNS: 1000, NS: 1100}).Slow(th) {
		t.Fatal("10% on a registration benchmark was flagged")
	}
	if !(Delta{Name: "APIHappyPath", BaseNS: 1000, NS: 1100}).Slow(th) {
		t.Fatal("10% on an ordinary benchmark was not flagged")
	}
	if (Delta{BaseBytes: 82744, Bytes: 82746.5}).BytesUp() {
		t.Fatal("median B/op drift was flagged")
	}
	if !(Delta{BaseBytes: 16, Bytes: 48}).BytesUp() {
		t.Fatal("real byte growth was not flagged")
	}

	var out bytes.Buffer
	c.print(&out, th, false)
	for _, want := range []string{"Frameworks 1/1 +0.0% → 0/1 +18.2%  (+0, -1)", "Routers    1/1 +0.0% → 0/1 +18.2%", "ALLOCS", "SLOW", "Lost      1: HelloWorld", "API04ParamInt"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestABPatternsAndVerdict(t *testing.T) {
	got := abPatterns([]string{"LargeRouteSetParam", "ScenarioRouteSetBuild/GitHubAPI203", "API04ParamInt"})
	want := []string{
		"^BenchmarkLargeRouteSetParam$/^Zinc$",
		"^BenchmarkScenarioRouteSetBuild$/^GitHubAPI203$/^Zinc$",
		"^BenchmarkAPI04ParamInt$",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("patterns = %q, want %q", got, want)
	}
	base := []float64{100, 101, 102, 103, 104, 105}
	if v := abVerdict(base, []float64{101, 102, 103, 104, 105, 106}); v != "same" {
		t.Fatalf("overlapping samples: verdict %q", v)
	}
	if v := abVerdict(base, []float64{120, 121, 122, 123, 124, 125}); v != "slower" {
		t.Fatalf("separated samples: verdict %q", v)
	}
}
