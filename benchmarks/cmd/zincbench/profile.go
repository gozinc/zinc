package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cmdProfile records CPU and memory profiles of Zinc's run of one scenario
// and prints the top of each. The profiles and the test binary are kept in
// benchmarks/results/profiles/<time>-<scenario>/ for `go tool pprof`.
func cmdProfile(s *Store, args []string) error {
	fs := newFlags("profile")
	benchtime := fs.String("benchtime", "3s", "go test -benchtime while profiling")
	top := fs.Int("top", 20, "rows of each profile to print")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("profile takes one scenario name, such as HelloWorld or ScenarioRouteSetAll/GitHubAPI203")
	}
	name := fs.Arg(0)
	pattern := abPatterns([]string{name})[0]

	dir := filepath.Join(s.results, "profiles", time.Now().Format("20060102-150405")+"-"+strings.ReplaceAll(name, "/", "_"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cpu, mem := filepath.Join(dir, "cpu.pprof"), filepath.Join(dir, "mem.pprof")
	fmt.Printf("Profiling %s (%s) for %s\n\n", name, pattern, *benchtime)

	cmd := exec.Command("go", "test", "-run", "^$", "-bench", pattern, "-benchmem", "-count", "1",
		"-benchtime", *benchtime, "-cpuprofile", cpu, "-memprofile", mem, "-o", filepath.Join(dir, "benchmarks.test"), ".")
	cmd.Dir = filepath.Join(s.root, "benchmarks")
	out, err := cmd.CombinedOutput()
	os.Stdout.Write(out)
	if err != nil {
		return fmt.Errorf("go test failed: %w", err)
	}
	if !strings.Contains(string(out), "/Zinc") && !strings.Contains(string(out), "Benchmark"+name) {
		return fmt.Errorf("no benchmark matched %s", pattern)
	}

	for _, p := range []struct {
		title, file string
		extra       []string
	}{
		{"CPU", cpu, nil},
		{"Allocations", mem, []string{"-sample_index=alloc_space"}},
	} {
		args := append([]string{"tool", "pprof", "-top", fmt.Sprintf("-nodecount=%d", *top)}, p.extra...)
		args = append(args, filepath.Join(dir, "benchmarks.test"), p.file)
		pp := exec.Command("go", args...)
		pp.Dir = cmd.Dir
		res, err := pp.CombinedOutput()
		if err != nil {
			return fmt.Errorf("pprof %s: %w\n%s", p.title, err, res)
		}
		fmt.Printf("\n%s\n%s", p.title, res)
	}
	fmt.Printf("\nSaved in %s\nExplore: go tool pprof -http=: %s %s\n", dir, filepath.Join(dir, "benchmarks.test"), cpu)
	return nil
}
