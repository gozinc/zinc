package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
)

// Thresholds is the regression check applied by compare. Allocation counts
// are deterministic, so any increase fails; time is noisy, so a scenario is
// flagged only when it is slower by both a relative and an absolute margin.
// Two Zinc-only runs of the same code (P0, 25 Sep 2026) put most scenarios
// within 3%, while parallel, registration, and route-set build benchmarks
// moved by up to 12%. Those get the NoisyPct margin instead.
type Thresholds struct {
	SlowPct  float64
	SlowNS   float64
	NoisyPct float64
	Noisy    *regexp.Regexp
}

var defaultNoisy = regexp.MustCompile(`^Parallel|Registration|RouteSetBuild`)

func (t Thresholds) pct(name string) float64 {
	if t.Noisy != nil && t.Noisy.MatchString(name) {
		return t.NoisyPct
	}
	return t.SlowPct
}

// Delta compares Zinc's samples for one benchmark between two runs.
type Delta struct {
	Name                   string
	BaseNS, NS             float64
	BaseBytes, Bytes       float64
	BaseAllocs, Allocs     float64
	BaseWin, Win, HasRival bool
}

func (d Delta) DeltaNS() float64 { return d.NS - d.BaseNS }

func (d Delta) DeltaPct() float64 {
	if d.BaseNS == 0 {
		return 0
	}
	return (d.NS - d.BaseNS) / d.BaseNS * 100
}

func (d Delta) AllocsUp() bool { return d.Allocs > d.BaseAllocs }

// BytesUp ignores the few bytes that the median of B/op drifts by on
// benchmarks with large or amortised allocations.
func (d Delta) BytesUp() bool {
	growth := d.Bytes - d.BaseBytes
	return growth > 8 && growth > d.BaseBytes*0.01
}

func (d Delta) Slow(t Thresholds) bool {
	return d.DeltaPct() > t.pct(d.Name) && d.DeltaNS() > t.SlowNS
}

func (d Delta) Fast(t Thresholds) bool {
	return d.DeltaPct() < -t.pct(d.Name) && -d.DeltaNS() > t.SlowNS
}

func (d Delta) Flagged(t Thresholds) bool {
	return d.AllocsUp() || d.BytesUp() || d.Slow(t) || d.BaseWin != d.Win
}

// Comparison is the result of comparing run against base.
type Comparison struct {
	Base, Run        *Run
	Scenarios, Micro []Delta
	Missing, Added   []string
}

func compareRuns(base, run *Run) Comparison {
	c := Comparison{Base: base, Run: run}
	for name, sc := range base.Scenarios {
		now, ok := run.Scenarios[name]
		if !ok {
			c.Missing = append(c.Missing, name)
			continue
		}
		d := sampleDelta(name, sc["Zinc"], now["Zinc"])
		d.HasRival = true
		d.BaseWin = zincWins(sc)
		d.Win = zincWins(now)
		c.Scenarios = append(c.Scenarios, d)
	}
	for name := range run.Scenarios {
		if _, ok := base.Scenarios[name]; !ok {
			c.Added = append(c.Added, name)
		}
	}
	for name, samples := range base.ZincOnly {
		if now, ok := run.ZincOnly[name]; ok {
			c.Micro = append(c.Micro, sampleDelta(name, samples, now))
		}
	}
	sort.Strings(c.Missing)
	sort.Strings(c.Added)
	byPct := func(ds []Delta) {
		sort.Slice(ds, func(i, j int) bool { return ds[i].DeltaPct() > ds[j].DeltaPct() })
	}
	byPct(c.Scenarios)
	byPct(c.Micro)
	return c
}

func sampleDelta(name string, base, now Samples) Delta {
	return Delta{
		Name:   name,
		BaseNS: median(base.NS), NS: median(now.NS),
		BaseBytes: base.Bytes, Bytes: now.Bytes,
		BaseAllocs: base.Allocs, Allocs: now.Allocs,
	}
}

// zincWins reports whether Zinc has the strictly lowest median in sc.
func zincWins(sc Scenario) bool {
	z := median(sc["Zinc"].NS)
	for _, f := range frameworks[1:] {
		if median(sc[f].NS) <= z {
			return false
		}
	}
	return true
}

func cmdCompare(s *Store, args []string) error {
	fs := newFlags("compare")
	baseID := fs.String("base", "", "run to compare against (default: the pinned baseline)")
	slowPct := fs.Float64("slow-pct", 5, "flag a scenario slower by more than this percentage...")
	slowNS := fs.Float64("slow-ns", 15, "...and by more than this many nanoseconds")
	noisyPct := fs.Float64("noisy-pct", 12, "percentage margin for parallel, registration, and route-set build benchmarks")
	all := fs.Bool("all", false, "list every scenario, not only flagged ones")
	_ = fs.Parse(args)

	runID := "latest"
	if fs.NArg() > 0 {
		runID = fs.Arg(0)
	}
	base, err := s.resolveRun(*baseID)
	if err != nil {
		return err
	}
	run, err := s.resolveRun(runID)
	if err != nil {
		return err
	}
	if base.ID == run.ID {
		return fmt.Errorf("base and run are both %s", run.ID)
	}
	t := Thresholds{SlowPct: *slowPct, SlowNS: *slowNS, NoisyPct: *noisyPct, Noisy: defaultNoisy}
	c := compareRuns(base, run)
	c.print(os.Stdout, t, *all)
	if n := c.allocRegressions(); n > 0 {
		return fmt.Errorf("check failed: %d benchmark(s) allocate more than the base", n)
	}
	return nil
}

func (c Comparison) allocRegressions() int {
	n := 0
	for _, d := range append(append([]Delta(nil), c.Scenarios...), c.Micro...) {
		if d.AllocsUp() {
			n++
		}
	}
	return n
}

func (c Comparison) print(w io.Writer, t Thresholds, all bool) {
	fmt.Fprintf(w, "base  %s  %s  %s\n", c.Base.ID, c.Base.Git.Short, c.Base.Note)
	fmt.Fprintf(w, "run   %s  %s  %s\n", c.Run.ID, c.Run.Git.Short, c.Run.Note)
	if c.Run.RivalSource != "" {
		fmt.Fprintf(w, "      rivals reused from %s (mixed-date; wins are descriptive)\n", c.Run.RivalSource)
	}
	fmt.Fprintln(w)

	var allocs, bytesUp, slow, fast, gained, lost []Delta
	for _, d := range c.Scenarios {
		switch {
		case d.AllocsUp():
			allocs = append(allocs, d)
		case d.BytesUp():
			bytesUp = append(bytesUp, d)
		}
		if d.Slow(t) {
			slow = append(slow, d)
		}
		if d.Fast(t) {
			fast = append(fast, d)
		}
		if d.Win && !d.BaseWin {
			gained = append(gained, d)
		}
		if d.BaseWin && !d.Win {
			lost = append(lost, d)
		}
	}
	fmt.Fprintf(w, "Wins      %d/%d → %d/%d  (+%d, -%d)\n", c.Base.wins(), len(c.Base.Scenarios), c.Run.wins(), len(c.Run.Scenarios), len(gained), len(lost))
	fmt.Fprintf(w, "Allocs ↑  %s\n", names(allocs))
	fmt.Fprintf(w, "Bytes ↑   %s\n", names(bytesUp))
	fmt.Fprintf(w, "Slower    %s  (>%.0f%%, noisy >%.0f%%, and >%.0f ns)\n", names(slow), t.SlowPct, t.NoisyPct, t.SlowNS)
	fmt.Fprintf(w, "Faster    %s\n", names(fast))
	if len(lost) > 0 {
		fmt.Fprintf(w, "Lost      %s\n", names(lost))
	}
	if len(gained) > 0 {
		fmt.Fprintf(w, "Gained    %s\n", names(gained))
	}
	if len(c.Missing) > 0 {
		fmt.Fprintf(w, "Missing   %s\n", strings.Join(c.Missing, ", "))
	}
	if len(slow) > 0 {
		fmt.Fprintf(w, "Confirm   zincbench ab -scenarios %s\n", strings.Join(nameList(slow), ","))
	}

	rows := c.Scenarios
	if !all {
		rows = nil
		for _, d := range c.Scenarios {
			if d.Flagged(t) || d.Fast(t) {
				rows = append(rows, d)
			}
		}
	}
	if len(rows) > 0 {
		fmt.Fprintln(w)
		printDeltas(w, rows, t)
	}
	if len(c.Micro) > 0 {
		fmt.Fprintln(w, "\nZinc-only benchmarks")
		printDeltas(w, c.Micro, t)
	}
}

func printDeltas(w io.Writer, ds []Delta, t Thresholds) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "benchmark\tbase ns\tns\tΔ ns\tΔ %\tB/op\tallocs\twin\tflag\t")
	for _, d := range ds {
		win := ""
		if d.HasRival {
			win = mark(d.BaseWin) + "→" + mark(d.Win)
		}
		fmt.Fprintf(tw, "%s\t%.1f\t%.1f\t%+.1f\t%+.1f%%\t%s\t%s\t%s\t%s\t\n",
			d.Name, d.BaseNS, d.NS, d.DeltaNS(), d.DeltaPct(),
			change(d.BaseBytes, d.Bytes), change(d.BaseAllocs, d.Allocs), win, flags(d, t))
	}
	tw.Flush()
}

func flags(d Delta, t Thresholds) string {
	var f []string
	if d.AllocsUp() {
		f = append(f, "ALLOCS")
	}
	if d.BytesUp() {
		f = append(f, "BYTES")
	}
	if d.Slow(t) {
		f = append(f, "SLOW")
	}
	if d.Fast(t) {
		f = append(f, "fast")
	}
	return strings.Join(f, " ")
}

func change(base, now float64) string {
	if base == now {
		return fmt.Sprintf("%g", now)
	}
	return fmt.Sprintf("%g→%g", base, now)
}

func mark(win bool) string {
	if win {
		return "W"
	}
	return "·"
}

func nameList(ds []Delta) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return out
}

func names(ds []Delta) string {
	if len(ds) == 0 {
		return "none"
	}
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return fmt.Sprintf("%d: %s", len(ds), strings.Join(out, ", "))
}
