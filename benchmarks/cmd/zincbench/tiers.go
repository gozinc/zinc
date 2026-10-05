package main

import (
	"fmt"
	"math"
)

// Tier is a group of rivals that Zinc is scored against separately.
// Frameworks is the headline; Routers measures Zinc's routing against bare
// routers on the scenarios they can run.
type Tier struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Rivals []string `json:"rivals"`
}

// TieBand is how close, in percent, Zinc's median must be to the fastest
// rival's for a scenario to count as a tie instead of a win or a loss. A gap
// that small is within one run's noise and too small to matter to an app.
const TieBand = 3.0

// Outcome is Zinc's result in one scenario against a tier.
type Outcome int

const (
	Loss Outcome = iota
	Tie
	Win
)

// gapPct is how far a is above b, in percent of the smaller: positive when a
// is slower. Measuring from the smaller keeps the band symmetric.
func gapPct(a, b float64) float64 {
	lo := math.Min(a, b)
	if lo <= 0 {
		switch {
		case a == b:
			return 0
		case a < b:
			return math.Inf(-1)
		default:
			return math.Inf(1)
		}
	}
	return (a - b) / lo * 100
}

var tiers = []Tier{
	{Key: "frameworks", Name: "Frameworks", Rivals: []string{"Gin", "Echo"}},
	{Key: "routers", Name: "Routers", Rivals: []string{"BunRouter", "Chi"}},
}

// headline is the tier behind the single win count shown by record, list and
// compare.
var headline = tiers[0]

// Score is Zinc's result against one tier over the scenarios where every
// rival in the tier was measured. Wins, Ties and Losses add up to Of.
type Score struct {
	Wins   int
	Ties   int
	Losses int
	Of     int
	// Geo is the geometric mean, in percent, of how far Zinc's median is
	// above the fastest median in each scenario. 0 means fastest everywhere.
	Geo float64
}

// covers reports whether sc has samples for Zinc and every rival in t.
func (t Tier) covers(sc Scenario) bool {
	if _, ok := sc["Zinc"]; !ok {
		return false
	}
	for _, f := range t.Rivals {
		if _, ok := sc[f]; !ok {
			return false
		}
	}
	return true
}

// outcome compares Zinc's median with the fastest rival's in t: within
// TieBand is a tie, otherwise the faster one wins.
func (t Tier) outcome(sc Scenario) Outcome {
	z := median(sc["Zinc"].NS)
	best := math.Inf(1)
	for _, f := range t.Rivals {
		best = math.Min(best, median(sc[f].NS))
	}
	gap := gapPct(z, best)
	switch {
	case math.Abs(gap) < TieBand:
		return Tie
	case gap < 0:
		return Win
	default:
		return Loss
	}
}

// wins reports whether Zinc beats every rival in t by more than TieBand.
func (t Tier) wins(sc Scenario) bool { return t.outcome(sc) == Win }

func (r *Run) score(t Tier) Score {
	var s Score
	var logs float64
	for _, sc := range r.Scenarios {
		if !t.covers(sc) {
			continue
		}
		z := median(sc["Zinc"].NS)
		best := z
		for _, f := range t.Rivals {
			best = math.Min(best, median(sc[f].NS))
		}
		s.Of++
		switch t.outcome(sc) {
		case Win:
			s.Wins++
		case Tie:
			s.Ties++
		default:
			s.Losses++
		}
		if best > 0 {
			logs += math.Log(z / best)
		}
	}
	if s.Of > 0 {
		s.Geo = (math.Exp(logs/float64(s.Of)) - 1) * 100
	}
	return s
}

func (s Score) String() string {
	if s.Of == 0 {
		return "—"
	}
	return fmt.Sprintf("%d/%d, %d ties, +%.1f%%", s.Wins, s.Of, s.Ties, s.Geo)
}
