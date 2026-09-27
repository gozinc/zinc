package main

import (
	"strings"
	"testing"
)

func TestReportScoresBothTablesAndListsLosses(t *testing.T) {
	s := func(ns float64) Samples { return Samples{NS: []float64{ns, ns, ns}} }
	run := &Run{
		ID: "20260101-000000-abc1234", Release: "9.9.9", CreatedAt: "2026-01-01T00:00:00Z",
		Git: GitInfo{Short: "abc1234"},
		Env: Env{Go: "go1.27.1", GOOS: "darwin", GOARCH: "arm64", CPU: "Test CPU", Count: 3, Benchtime: "100ms",
			Rivals: map[string]string{"Gin": "v1", "Echo": "v2", "Chi": "v3", "BunRouter": "v4"}},
		Scenarios: map[string]Scenario{
			"Hello":   {"Zinc": s(80), "Gin": s(130), "Echo": s(150), "Chi": s(170), "BunRouter": s(60)},
			"Miss":    {"Zinc": s(100), "Gin": s(50), "Echo": s(700), "Chi": s(290), "BunRouter": s(90)},
			"APIBind": {"Zinc": s(1500), "Gin": s(4000), "Echo": s(1700)},
		},
	}
	var b strings.Builder
	if err := writeReport(&b, run); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"### Frameworks (3 scenarios)",
		"| **Zinc** | 2 / 3 |",
		"### Routers (2 scenarios)",
		"| BunRouter | 2 / 2 |",
		"| `Miss` | 100.0 | 50.00 | 700.0 | Gin |",
		"| `APIBind` | 1,500 | 4,000 | 1,700 | Zinc |",
		"### Frameworks: Zinc slower in 1",
		"| `Miss` | Gin | 100.0% |",
		"| `Hello` | BunRouter | 33.3% |",
		"Chi `v3`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q", want)
		}
	}
}
