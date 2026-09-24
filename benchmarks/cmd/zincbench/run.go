package main

import (
	"bufio"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Frameworks compared in the head-to-head suite. Zinc is always first.
var frameworks = []string{"Zinc", "Gin", "Echo", "Chi"}

// Run is one immutable benchmark record.
type Run struct {
	Schema    int                 `json:"schema"`
	ID        string              `json:"id"`
	CreatedAt string              `json:"createdAt"`
	Note      string              `json:"note,omitempty"`
	Imported  bool                `json:"imported,omitempty"`
	Git       GitInfo             `json:"git"`
	Env       Env                 `json:"env"`
	Scenarios map[string]Scenario `json:"scenarios"`
	ZincOnly  map[string]Samples  `json:"zincOnly,omitempty"`
}

// Scenario holds the samples for each framework in one head-to-head benchmark.
type Scenario map[string]Samples

// Samples are the per-iteration results of one benchmark.
type Samples struct {
	NS     []float64 `json:"ns"`
	Bytes  float64   `json:"bytes"`
	Allocs float64   `json:"allocs"`
}

// GitInfo identifies the code that was measured, including uncommitted work.
type GitInfo struct {
	Commit   string   `json:"commit"`
	Short    string   `json:"short"`
	Branch   string   `json:"branch,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	Dirty    bool     `json:"dirty"`
	DiffHash string   `json:"diffHash,omitempty"`
	Changed  []string `json:"changed,omitempty"`
}

// Env describes where and how the run was measured.
type Env struct {
	Go        string            `json:"go"`
	GOOS      string            `json:"goos"`
	GOARCH    string            `json:"goarch"`
	CPU       string            `json:"cpu"`
	Host      string            `json:"host"`
	Benchtime string            `json:"benchtime"`
	Count     int               `json:"count"`
	Rivals    map[string]string `json:"rivals"`
}

var benchLine = regexp.MustCompile(`^Benchmark(\S+?)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op(?:\s+([\d.]+) B/op)?(?:\s+([\d.]+) allocs/op)?`)

// parseOutput reads `go test -bench` output. Benchmarks whose last path
// segment names a framework become head-to-head scenarios; everything else is
// kept as a Zinc-only benchmark.
func parseOutput(r io.Reader) (scenarios map[string]Scenario, zincOnly map[string]Samples, header map[string]string, err error) {
	scenarios = map[string]Scenario{}
	zincOnly = map[string]Samples{}
	header = map[string]string{}
	type acc struct {
		ns            []float64
		bytes, allocs []float64
	}
	h2h := map[string]map[string]*acc{}
	solo := map[string]*acc{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		for _, key := range []string{"goos", "goarch", "cpu", "pkg"} {
			if strings.HasPrefix(line, key+": ") {
				header[key] = strings.TrimSpace(strings.TrimPrefix(line, key+": "))
			}
		}
		m := benchLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		ns, _ := strconv.ParseFloat(m[2], 64)
		b, _ := strconv.ParseFloat(m[3], 64)
		a, _ := strconv.ParseFloat(m[4], 64)

		var target *acc
		if i := strings.LastIndex(name, "/"); i > 0 && isFramework(name[i+1:]) {
			scenario, fw := name[:i], name[i+1:]
			if h2h[scenario] == nil {
				h2h[scenario] = map[string]*acc{}
			}
			if h2h[scenario][fw] == nil {
				h2h[scenario][fw] = &acc{}
			}
			target = h2h[scenario][fw]
		} else {
			if solo[name] == nil {
				solo[name] = &acc{}
			}
			target = solo[name]
		}
		target.ns = append(target.ns, ns)
		target.bytes = append(target.bytes, b)
		target.allocs = append(target.allocs, a)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, nil, err
	}

	toSamples := func(a *acc) Samples {
		return Samples{NS: a.ns, Bytes: median(a.bytes), Allocs: median(a.allocs)}
	}
	for scenario, fws := range h2h {
		if len(fws) != len(frameworks) {
			continue // a head-to-head scenario needs every framework
		}
		s := Scenario{}
		for fw, a := range fws {
			s[fw] = toSamples(a)
		}
		scenarios[scenario] = s
	}
	for name, a := range solo {
		zincOnly[name] = toSamples(a)
	}
	return scenarios, zincOnly, header, nil
}

func isFramework(s string) bool {
	for _, f := range frameworks {
		if s == f {
			return true
		}
	}
	return false
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// wins counts scenarios where Zinc has the strictly lowest median.
func (r *Run) wins() int {
	n := 0
	for _, s := range r.Scenarios {
		z := median(s["Zinc"].NS)
		best := true
		for _, f := range frameworks[1:] {
			if median(s[f].NS) <= z {
				best = false
			}
		}
		if best {
			n++
		}
	}
	return n
}
