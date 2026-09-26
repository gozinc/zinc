package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

// cmdAB confirms a suspected regression by measuring the baseline commit and
// the working tree in alternating rounds. Interleaving cancels the thermal
// and background drift that separates two recordings made minutes apart, so
// what remains is the difference between the two versions of the code.
func cmdAB(s *Store, args []string) error {
	fs := newFlags("ab")
	scenarios := fs.String("scenarios", "", "comma-separated scenario or Zinc-only benchmark names, as compare prints them")
	baseRef := fs.String("base", "", "commit to compare against (default: the baseline run's commit)")
	rounds := fs.Int("rounds", 6, "alternating rounds; each round measures both trees")
	count := fs.Int("count", 3, "samples per benchmark per round")
	benchtime := fs.String("benchtime", "100ms", "go test -benchtime for each sample")
	_ = fs.Parse(args)
	if *scenarios == "" {
		return errors.New("ab needs -scenarios")
	}

	commit := *baseRef
	if commit == "" {
		base := s.baselineRun()
		if base == nil {
			return errors.New("no baseline pinned; pass -base")
		}
		commit = base.Git.Commit
	}
	if tc := s.baselineToolchain(); tc != "" {
		os.Setenv("GOTOOLCHAIN", tc)
	}
	baseDir, err := s.worktree(commit)
	if err != nil {
		return err
	}
	names := strings.Split(*scenarios, ",")
	patterns := abPatterns(names)
	trees := []struct{ label, dir string }{
		{"base", filepath.Join(baseDir, "benchmarks")},
		{"head", filepath.Join(s.root, "benchmarks")},
	}

	fmt.Printf("A/B %s (base) against the working tree: %d rounds × %d samples × %s\n\n", shortCommit(commit), *rounds, *count, *benchtime)
	samples := map[string]map[string][]float64{"base": {}, "head": {}}
	for round := 0; round < *rounds; round++ {
		order := []int{0, 1}
		if round%2 == 1 {
			order = []int{1, 0}
		}
		for _, i := range order {
			tree := trees[i]
			for _, pattern := range patterns {
				var out bytes.Buffer
				cmd := exec.Command("go", "test", "-run", "^$", "-bench", pattern, "-benchmem",
					"-count", fmt.Sprint(*count), "-benchtime", *benchtime, ".")
				cmd.Dir = tree.dir
				cmd.Stdout = &out
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("%s tree: go test failed: %w", tree.label, err)
				}
				collectABSamples(&out, samples[tree.label])
			}
		}
		fmt.Printf("round %d/%d done\n", round+1, *rounds)
	}
	fmt.Println()
	return printAB(os.Stdout, names, samples)
}

// abPatterns builds one -bench pattern per name. Head-to-head scenarios select
// their Zinc case; other names are Zinc-only benchmark functions.
func abPatterns(names []string) []string {
	var patterns []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "API04") {
			patterns = append(patterns, "^Benchmark"+name+"$")
			continue
		}
		parts := strings.Split(name, "/")
		levels := []string{"^Benchmark" + parts[0] + "$"}
		for _, part := range parts[1:] {
			levels = append(levels, "^"+part+"$")
		}
		levels = append(levels, "^Zinc$")
		patterns = append(patterns, strings.Join(levels, "/"))
	}
	return patterns
}

func collectABSamples(r io.Reader, into map[string][]float64) {
	scenarios, zincOnly, _, _ := parseFrameworkOutput(r)
	for name, sc := range scenarios {
		into[name] = append(into[name], sc["Zinc"].NS...)
	}
	for name, s := range zincOnly {
		into[name] = append(into[name], s.NS...)
	}
}

func printAB(w io.Writer, names []string, samples map[string]map[string][]float64) error {
	sort.Strings(names)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "benchmark\tbase ns\thead ns\tΔ ns\tΔ %\tbase spread\thead spread\tverdict\t")
	for _, name := range names {
		base, head := samples["base"][name], samples["head"][name]
		if len(base) == 0 || len(head) == 0 {
			fmt.Fprintf(tw, "%s\t\t\t\t\t\t\tnot measured\t\n", name)
			continue
		}
		b, h := median(base), median(head)
		fmt.Fprintf(tw, "%s\t%.1f\t%.1f\t%+.1f\t%+.1f%%\t%s\t%s\t%s\t\n",
			name, b, h, h-b, (h-b)/b*100, spread(base), spread(head), abVerdict(base, head))
	}
	return tw.Flush()
}

// spread reports the interquartile range as a percentage of the median.
func spread(v []float64) string {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	q1, q3 := s[len(s)/4], s[(3*len(s))/4]
	return fmt.Sprintf("±%.1f%%", (q3-q1)/2/median(s)*100)
}

// abVerdict calls a difference real only when the interquartile ranges of
// the two sample sets do not overlap.
func abVerdict(base, head []float64) string {
	quartiles := func(v []float64) (float64, float64) {
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		return s[len(s)/4], s[(3*len(s))/4]
	}
	bq1, bq3 := quartiles(base)
	hq1, hq3 := quartiles(head)
	switch {
	case hq1 > bq3:
		return "slower"
	case hq3 < bq1:
		return "faster"
	default:
		return "same"
	}
}

// worktree returns a detached checkout of commit, creating it on first use.
// The benchmarks module there resolves Zinc from that tree.
func (s *Store) worktree(commit string) (string, error) {
	root, err := s.abRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, shortCommit(commit))
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return dir, nil
	}
	// A cleared cache leaves a stale worktree record that would block add.
	prune := exec.Command("git", "worktree", "prune")
	prune.Dir = s.root
	_ = prune.Run()
	cmd := exec.Command("git", "worktree", "add", "--detach", dir, commit)
	cmd.Dir = s.root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %v: %s", err, out)
	}
	return dir, nil
}

// abRoot is where ab keeps its baseline checkouts: the user cache directory,
// keyed by repository. Outside the repository, their nested go.mod files
// can't confuse editors or go tooling run from the repository root.
func (s *Store) abRoot() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find a directory for ab checkouts: %w", err)
	}
	sum := sha256.Sum256([]byte(s.root))
	return filepath.Join(cache, "zincbench", "ab", hex.EncodeToString(sum[:6])), nil
}

func (s *Store) baselineToolchain() string {
	if base := s.baselineRun(); base != nil {
		return base.Env.Go
	}
	return ""
}

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}
