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

var tiers = []Tier{
	{Key: "frameworks", Name: "Frameworks", Rivals: []string{"Gin", "Echo"}},
	{Key: "routers", Name: "Routers", Rivals: []string{"BunRouter", "Chi"}},
}

// headline is the tier behind the single win count shown by record, list and
// compare.
var headline = tiers[0]

// Score is Zinc's result against one tier over the scenarios where every
// rival in the tier was measured.
type Score struct {
	Wins int
	Of   int
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

// wins reports whether Zinc has the strictly lowest median against t.
func (t Tier) wins(sc Scenario) bool {
	z := median(sc["Zinc"].NS)
	for _, f := range t.Rivals {
		if median(sc[f].NS) <= z {
			return false
		}
	}
	return true
}

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
		if t.wins(sc) {
			s.Wins++
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

// wins counts headline scenarios where Zinc has the strictly lowest median.
func (r *Run) wins() int { return r.score(headline).Wins }

func (s Score) String() string {
	if s.Of == 0 {
		return "—"
	}
	return fmt.Sprintf("%d/%d +%.1f%%", s.Wins, s.Of, s.Geo)
}
