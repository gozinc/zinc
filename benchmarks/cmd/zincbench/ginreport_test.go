package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ginTestLog = `#GithubAPI Routes: 203
   Gin: 58840 Bytes
   Chi: 94888 Bytes

#Zinc routing-structure memory:
   Zinc_GitHub: 70000 Bytes
goos: darwin
goarch: arm64
cpu: Test CPU
BenchmarkGin_GithubAll         	   10	     14000 ns/op	       0 B/op	       0 allocs/op
BenchmarkGin_GithubAll         	   10	     14200 ns/op	       0 B/op	       0 allocs/op
BenchmarkZinc_GithubAll        	   10	     25000 ns/op	       0 B/op	       0 allocs/op
BenchmarkZinc_GithubAll        	   10	     25400 ns/op	       0 B/op	       0 allocs/op
BenchmarkZincNoCache_GithubAll 	   10	     25000 ns/op	       0 B/op	       0 allocs/op
BenchmarkChi_GithubAll         	   10	    130000 ns/op	  130817 B/op	     740 allocs/op
BenchmarkFiber_GithubAll       	   10	     10000 ns/op	       0 B/op	       0 allocs/op
BenchmarkGin_Param20-8         	  100	       172 ns/op	       0 B/op	       0 allocs/op
BenchmarkZinc_Param20-8        	  100	       615 ns/op	       0 B/op	       0 allocs/op
`

func TestGinReportRanksCountsAndShowsZincChange(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "cur.log")
	prev := filepath.Join(dir, "prev.log")
	if err := os.WriteFile(cur, []byte(ginTestLog), 0o644); err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(ginTestLog, "25000 ns/op", "19000 ns/op", 1)
	old = strings.Replace(old, "25400 ns/op", "19000 ns/op", 1)
	if err := os.WriteFile(prev, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := readGinLog(cur)
	if err != nil {
		t.Fatal(err)
	}
	p, err := readGinLog(prev)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := writeGinReport(&b, c, p, ginReportInfo{Label: "v9", Date: "1 Jan", Go: "go1.27.1", Upstream: "abc", PrevLabel: "v8"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"- Machine: Test CPU",
		"ranking **2nd of 3** `net/http` routers",
		"| GitHub API (203 routes) | 2 / 3 | 25,200 | 19,000 | +33% | 0 | 0 |",
		"| — | Fiber† | 10,000 |",
		"| 1 | Gin | 14,100 |",
		"| **Zinc** | 70,000 |",
		"| Twenty parameters | 2 / 2 | 615.0 |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q", want)
		}
	}
	if strings.Contains(out, "NoCache") {
		t.Error("report includes the 0.4 ZincNoCache rows")
	}
}
