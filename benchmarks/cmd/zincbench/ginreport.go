package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// cmdGinReport writes GIN_BENCHMARK.md from a full run of the upstream Gin
// routing suite (every router, not only Zinc). With -previous, it adds how
// Zinc's rows changed since an earlier full run, so a slower release shows.
func cmdGinReport(s *Store, args []string) error {
	fs := newFlags("gin-report")
	logPath := fs.String("log", "", "full Gin-suite log (go test -bench . -count 5 -benchmem output)")
	prevPath := fs.String("previous", "", "an earlier full Gin-suite log, to show Zinc's change")
	prevLabel := fs.String("previous-label", "the previous release", "what the earlier log measured, e.g. v0.4.0")
	label := fs.String("label", "", "what this log measured, e.g. v0.5.0 (`8271438`)")
	date := fs.String("date", "", "date of the run, e.g. 27 September 2026")
	goVer := fs.String("go", "", "Go version of the run, e.g. go1.27.1")
	upstream := fs.String("upstream", "ff3cdf55eccd0aa6a272991db9611a86c734dc51", "upstream suite commit")
	out := fs.String("o", "", "write to this file instead of standard output")
	_ = fs.Parse(args)
	if *logPath == "" || *label == "" || *date == "" || *goVer == "" {
		return fmt.Errorf("gin-report needs -log, -label, -date and -go")
	}
	cur, err := readGinLog(*logPath)
	if err != nil {
		return err
	}
	var prev *ginRun
	if *prevPath != "" {
		if prev, err = readGinLog(*prevPath); err != nil {
			return err
		}
	}
	w := io.Writer(os.Stdout)
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return writeGinReport(w, cur, prev, ginReportInfo{Label: *label, Date: *date, Go: *goVer, Upstream: *upstream, PrevLabel: *prevLabel})
}

type ginReportInfo struct{ Label, Date, Go, Upstream, PrevLabel string }

type ginSample struct{ ns, bytes, allocs []float64 }

type ginRun struct {
	cpu, goos, goarch string
	// results[bench][router]
	results map[string]map[string]*ginSample
	// memory[routeSet][router] in bytes
	memory map[string]map[string]float64
}

var (
	ginBenchLine = regexp.MustCompile(`^Benchmark([A-Za-z0-9]+)_([A-Za-z0-9]+)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op`)
	ginMemHead   = regexp.MustCompile(`^#(\w+?)(?:API)? Routes: \d+`)
	ginMemLine   = regexp.MustCompile(`^\s+([\w.]+): (\d+) Bytes`)
	ginZincMem   = regexp.MustCompile(`^\s+Zinc_(\w+): (\d+) Bytes`)
)

// ginRouteSets maps the log's memory headings to the benchmark row names.
var ginRouteSets = map[string]string{"Github": "GitHub", "GPlus": "GPlus", "Parse": "Parse", "Static": "Static"}

func readGinLog(path string) (*ginRun, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	run := &ginRun{results: map[string]map[string]*ginSample{}, memory: map[string]map[string]float64{}}
	section, zincMem := "", false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "cpu: "):
			run.cpu = strings.TrimPrefix(line, "cpu: ")
		case strings.HasPrefix(line, "goos: "):
			run.goos = strings.TrimPrefix(line, "goos: ")
		case strings.HasPrefix(line, "goarch: "):
			run.goarch = strings.TrimPrefix(line, "goarch: ")
		case strings.HasPrefix(line, "#Zinc routing-structure memory"):
			zincMem, section = true, ""
		case ginMemHead.MatchString(line):
			section, zincMem = ginRouteSets[ginMemHead.FindStringSubmatch(line)[1]], false
		case zincMem && ginZincMem.MatchString(line):
			m := ginZincMem.FindStringSubmatch(line)
			set := m[1]
			if run.memory[set] == nil {
				run.memory[set] = map[string]float64{}
			}
			run.memory[set]["Zinc"], _ = strconv.ParseFloat(m[2], 64)
		case section != "" && ginMemLine.MatchString(line):
			m := ginMemLine.FindStringSubmatch(line)
			if run.memory[section] == nil {
				run.memory[section] = map[string]float64{}
			}
			run.memory[section][m[1]], _ = strconv.ParseFloat(m[2], 64)
		case ginBenchLine.MatchString(line):
			m := ginBenchLine.FindStringSubmatch(line)
			router, bench := m[1], m[2]
			if router == "ZincNoCache" {
				continue // 0.4 adapter only; 0.5 has no route cache
			}
			if run.results[bench] == nil {
				run.results[bench] = map[string]*ginSample{}
			}
			sm := run.results[bench][router]
			if sm == nil {
				sm = &ginSample{}
				run.results[bench][router] = sm
			}
			ns, _ := strconv.ParseFloat(m[3], 64)
			b, _ := strconv.ParseFloat(m[4], 64)
			a, _ := strconv.ParseFloat(m[5], 64)
			sm.ns, sm.bytes, sm.allocs = append(sm.ns, ns), append(sm.bytes, b), append(sm.allocs, a)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(run.results["GithubAll"]["Zinc"].nsOrNil()) == 0 {
		return nil, fmt.Errorf("%s: no Zinc GithubAll samples; is it a full Gin-suite log?", path)
	}
	return run, nil
}

func (s *ginSample) nsOrNil() []float64 {
	if s == nil {
		return nil
	}
	return s.ns
}

// ginRows are the 16 comparable rows, in report order.
var ginRows = []struct{ key, title, note string }{
	{"GithubAll", "GitHub API (203 routes)", "One operation covers all 203 routes in the fixture."},
	{"GPlusAll", "Google+ API (13 routes)", "One operation covers all 13 routes in the fixture."},
	{"ParseAll", "Parse API (26 routes)", "One operation covers all 26 routes in the fixture."},
	{"StaticAll", "Static routes (157 routes)", "One operation covers all 157 routes in the fixture."},
	{"Param", "Single parameter", ""},
	{"Param5", "Five parameters", ""},
	{"Param20", "Twenty parameters", ""},
	{"ParamWrite", "Parameter read and write", ""},
	{"GithubStatic", "GitHub static route", ""},
	{"GithubParam", "GitHub parameter route", ""},
	{"GPlusStatic", "Google+ static route", ""},
	{"GPlusParam", "Google+ parameter route", ""},
	{"GPlus2Params", "Google+ two-parameter route", ""},
	{"ParseStatic", "Parse static route", ""},
	{"ParseParam", "Parse parameter route", ""},
	{"Parse2Params", "Parse two-parameter route", ""},
}

var ginDisplay = map[string]string{"Gojiv2": "Goji v2", "HttpServeMux": "http.ServeMux"}

func ginName(r string) string {
	if d, ok := ginDisplay[r]; ok {
		return d
	}
	return r
}

type ginEntry struct {
	router            string
	ns, bytes, allocs float64
	rank              int // 0 for Fiber, which isn't ranked
}

// ranked returns a row's routers fastest first. Fiber runs on fasthttp, not
// net/http, so it is listed but never ranked or counted.
func (r *ginRun) ranked(bench string) []ginEntry {
	var es []ginEntry
	for router, s := range r.results[bench] {
		es = append(es, ginEntry{router: router, ns: median(s.ns), bytes: median(s.bytes), allocs: median(s.allocs)})
	}
	sort.Slice(es, func(i, j int) bool { return es[i].ns < es[j].ns })
	rank := 0
	for i := range es {
		if es[i].router == "Fiber" {
			continue
		}
		rank++
		es[i].rank = rank
	}
	return es
}

func (r *ginRun) zinc(bench string) (ginEntry, int) {
	es := r.ranked(bench)
	n := 0
	var z ginEntry
	for _, e := range es {
		if e.rank > 0 {
			n++
		}
		if e.router == "Zinc" {
			z = e
		}
	}
	return z, n
}

// beats reports whether Zinc's median is strictly below every router in set.
func (r *ginRun) beats(bench string, set []string) bool {
	row := r.results[bench]
	z := median(row["Zinc"].nsOrNil())
	for _, other := range set {
		if s, ok := row[other]; !ok || median(s.ns) <= z {
			return false
		}
	}
	return true
}

func writeGinReport(w io.Writer, cur, prev *ginRun, info ginReportInfo) error {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	p("# Zinc in the Gin HTTP Routing Benchmark\n\n")
	p("- Machine: %s\n- OS / architecture: %s/%s\n- Date: %s\n- Zinc: %s\n- Go: `%s`\n", cur.cpu, cur.goos, cur.goarch, info.Date, info.Label, info.Go)
	p("- Samples: 5 × 100ms per benchmark; tables show medians\n")
	p("- Source: [gin-gonic/go-http-routing-benchmark](https://github.com/gin-gonic/go-http-routing-benchmark) at `%s`\n\n", info.Upstream)
	p("> The pinned upstream suite was run unchanged, with Zinc's adapter added as one file and Zinc added to its `go.mod`. This suite is available under the [BSD 3-Clause License](https://github.com/gin-gonic/go-http-routing-benchmark/blob/master/LICENSE); benchmark suite copyright © 2013 Julien Schmidt. Zinc is not affiliated with or endorsed by Gin or the suite authors.\n\n")
	p("This report is generated by `zincbench gin-report` from the run's log. Nothing in it is edited by hand.\n\n")

	wins, fw, rt := 0, 0, 0
	for _, row := range ginRows {
		if z, _ := cur.zinc(row.key); z.rank == 1 {
			wins++
		}
		if cur.beats(row.key, []string{"Gin", "Echo"}) {
			fw++
		}
		if cur.beats(row.key, []string{"BunRouter", "Chi"}) {
			rt++
		}
	}
	gh, ghN := cur.zinc("GithubAll")
	p("## Summary\n\n")
	p("Zinc has the lowest median in **%d/%d** rows against every other `net/http` router in the suite. Against the frameworks Gin and Echo it is fastest in **%d/%d**, and against the bare routers BunRouter and Chi in **%d/%d**. ", wins, len(ginRows), fw, len(ginRows), rt, len(ginRows))
	p("This router-focused suite measures different work from Zinc's own suite in [BENCHMARKS.md](BENCHMARKS.md); the counts must not be combined.\n\n")
	p("The complete GitHub API pass (203 routes) measures Zinc at **%s ns/op**, ranking **%s of %d** `net/http` routers.\n\n", formatNS(gh.ns), ordinalWord(gh.rank), ghN)
	p("> **This suite repeats the same URLs.** Each row sends one fixed path, or one fixed pass over a fixture, on every iteration. Zinc 0.4's route cache answered those from memory, which flattered it here. Zinc 0.5 has no route cache, so rows that 0.4 served from the cache are slower in 0.5. Zinc's own suite uses 10,000 distinct paths per scenario for this reason.\n\n")
	p("> **Fiber caveat:** Fiber uses a separate `fasthttp.RequestCtx` harness. Its times are shown for fidelity to the upstream suite, but it is excluded from `net/http` win counts and rankings.\n\n")

	p("## Zinc at a glance\n\n")
	if prev != nil {
		p("| Workload | Rank among `net/http` routers | Zinc ns/op | Zinc %s ns/op | Change | B/op | allocs/op |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n", info.PrevLabel)
	} else {
		p("| Workload | Rank among `net/http` routers | Zinc ns/op | B/op | allocs/op |\n| --- | ---: | ---: | ---: | ---: |\n")
	}
	for _, row := range ginRows {
		z, n := cur.zinc(row.key)
		if prev != nil {
			pz, _ := prev.zinc(row.key)
			change := "—"
			if pz.ns > 0 {
				change = fmt.Sprintf("%+.0f%%", (z.ns/pz.ns-1)*100)
			}
			p("| %s | %d / %d | %s | %s | %s | %s | %s |\n", row.title, z.rank, n, formatNS(z.ns), formatNS(pz.ns), change, formatCount(z.bytes), formatCount(z.allocs))
		} else {
			p("| %s | %d / %d | %s | %s | %s |\n", row.title, z.rank, n, formatNS(z.ns), formatCount(z.bytes), formatCount(z.allocs))
		}
	}
	p("\n")

	p("## Memory consumption\n\nRouting-structure bytes retained after registration, estimated once after GC. These are not process RSS or repeated timing samples.\n\n")
	for _, set := range []struct{ key, title string }{{"Static", "Static routes: 157"}, {"GitHub", "GitHub API routes: 203"}, {"GPlus", "Google+ API routes: 13"}, {"Parse", "Parse API routes: 26"}} {
		mem := cur.memory[set.key]
		if len(mem) == 0 {
			continue
		}
		type m struct {
			router string
			bytes  float64
		}
		var ms []m
		for router, v := range mem {
			ms = append(ms, m{router, v})
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i].bytes < ms[j].bytes })
		p("### %s\n\n| Router | Bytes |\n| --- | ---: |\n", set.title)
		for _, e := range ms {
			name := ginName(e.router)
			if e.router == "Zinc" {
				name = "**Zinc**"
			}
			p("| %s | %s |\n", name, formatCount(e.bytes))
		}
		p("\n")
	}

	p("## Benchmark results\n\nThe four `*All` rows time one complete pass over the route fixture, not one HTTP request. The remaining rows are single-request microbenchmarks. All values below are five-sample medians.\n\n")
	for _, row := range ginRows {
		p("### %s\n\n", row.title)
		if row.note != "" {
			p("%s\n\n", row.note)
		}
		p("| Rank | Router | ns/op | B/op | allocs/op |\n| ---: | --- | ---: | ---: | ---: |\n")
		for _, e := range cur.ranked(row.key) {
			rank, name := strconv.Itoa(e.rank), ginName(e.router)
			if e.rank == 0 {
				rank, name = "—", name+"†"
			}
			if e.router == "Zinc" {
				name = "**Zinc**"
			}
			p("| %s | %s | %s | %s | %s |\n", rank, name, formatNS(e.ns), formatCount(e.bytes), formatCount(e.allocs))
		}
		p("\n")
	}

	p("## Reproduce this run\n\n")
	p("[`benchmarks/gin-suite/`](benchmarks/gin-suite/) has Zinc's adapter and the steps: clone the upstream suite at the pinned commit, copy the adapter in as `zinc_test.go`, and point its `go.mod` at the Zinc checkout under test. Then, in the suite:\n\n")
	p("```bash\ngo test -count=1 ./...\ngo test -run='^$' -bench=. -benchmem -benchtime=100ms -count=5 -timeout=60m . > full.log\n```\n\n")
	p("And from Zinc's `benchmarks` directory:\n\n```bash\ngo run ./cmd/zincbench gin-report -log full.log -label <version> -date <date> -go <go version> -o ../GIN_BENCHMARK.md\n```\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func formatCount(v float64) string {
	return groupThousands(strconv.FormatFloat(math.Round(v), 'f', 0, 64))
}

func ordinalWord(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}
