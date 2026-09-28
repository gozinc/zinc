package main

import (
	"math"
	"strings"
	"testing"
)

func samples(ns ...float64) Samples { return Samples{NS: ns} }

func TestScorePerTier(t *testing.T) {
	r := &Run{Scenarios: map[string]Scenario{
		// Zinc fastest among frameworks, 2× BunRouter among routers.
		"A": {"Zinc": samples(100), "Gin": samples(150), "Echo": samples(200), "Chi": samples(400), "BunRouter": samples(50)},
		// Zinc 1.5× Gin; no BunRouter, so frameworks only.
		"B": {"Zinc": samples(300), "Gin": samples(200), "Echo": samples(250), "Chi": samples(900)},
	}}
	fw := r.score(tiers[0])
	if fw.Wins != 1 || fw.Of != 2 {
		t.Fatalf("frameworks = %+v, want 1 of 2", fw)
	}
	// Geometric mean of 1.0 and 1.5 is sqrt(1.5).
	if want := (math.Sqrt(1.5) - 1) * 100; math.Abs(fw.Geo-want) > 1e-9 {
		t.Fatalf("frameworks geo = %v, want %v", fw.Geo, want)
	}
	rt := r.score(tiers[1])
	if rt.Wins != 0 || rt.Of != 1 || math.Abs(rt.Geo-100) > 1e-9 {
		t.Fatalf("routers = %+v, want 0 of 1 at +100%%", rt)
	}
	if r.wins() != 1 {
		t.Fatalf("headline wins = %d, want 1", r.wins())
	}
	if got := rt.String(); got != "0/1, 0 ties, +100.0%" {
		t.Fatalf("String = %q", got)
	}
	if got := (&Run{}).score(tiers[1]).String(); got != "—" {
		t.Fatalf("empty String = %q", got)
	}
}

func TestParseKeepsScenariosCoveredByAnyTier(t *testing.T) {
	out := `BenchmarkFramework/Zinc-8  1  10 ns/op
BenchmarkFramework/Gin-8   1  20 ns/op
BenchmarkFramework/Echo-8  1  30 ns/op
BenchmarkRouting/Zinc-8      1  10 ns/op
BenchmarkRouting/BunRouter-8 1   5 ns/op
BenchmarkRouting/Chi-8       1  50 ns/op
BenchmarkPartial/Zinc-8  1  10 ns/op
BenchmarkPartial/Gin-8   1  20 ns/op
`
	sc, _, _, err := parseOutput(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"Framework": true, "Routing": true, "Partial": false} {
		if _, ok := sc[name]; ok != want {
			t.Errorf("%s kept=%v, want %v", name, ok, want)
		}
	}
}

// Within TieBand of the fastest rival, a scenario is a tie whichever side is
// ahead; at the band or beyond, the faster one wins.
func TestScoreTieBand(t *testing.T) {
	r := &Run{Scenarios: map[string]Scenario{
		"ClearWin":  {"Zinc": samples(100), "Gin": samples(103), "Echo": samples(200)}, // Gin 3% slower: a win
		"TieAhead":  {"Zinc": samples(100), "Gin": samples(102.9), "Echo": samples(200)},
		"TieBehind": {"Zinc": samples(102.9), "Gin": samples(100), "Echo": samples(200)},
		"Equal":     {"Zinc": samples(100), "Gin": samples(100), "Echo": samples(100)},
		"ClearLoss": {"Zinc": samples(103), "Gin": samples(100), "Echo": samples(200)},
		"ZeroRival": {"Zinc": samples(5), "Gin": samples(0), "Echo": samples(0)},
	}}
	want := map[string]Outcome{"ClearWin": Win, "TieAhead": Tie, "TieBehind": Tie, "Equal": Tie, "ClearLoss": Loss, "ZeroRival": Loss}
	for name, sc := range r.Scenarios {
		if got := tiers[0].outcome(sc); got != want[name] {
			t.Errorf("%s: outcome %v, want %v", name, got, want[name])
		}
	}
	s := r.score(tiers[0])
	if s.Wins != 1 || s.Ties != 3 || s.Losses != 2 || s.Of != 6 {
		t.Fatalf("score = %+v, want 1 win, 3 ties, 2 losses of 6", s)
	}
	if r.wins() != 1 {
		t.Fatalf("headline wins = %d, want 1", r.wins())
	}
}
