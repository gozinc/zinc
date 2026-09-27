package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Store is the benchmarks/results directory.
type Store struct {
	root    string // repository root
	results string // benchmarks/results
}

func openStore() (*Store, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, errors.New("run zincbench inside the zinc repository")
	}
	root := strings.TrimSpace(string(out))
	s := &Store{root: root, results: filepath.Join(root, "benchmarks", "results")}
	return s, os.MkdirAll(filepath.Join(s.results, "runs"), 0o755)
}

func (s *Store) runPath(id, ext string) string {
	return filepath.Join(s.results, "runs", id+ext)
}

func (s *Store) save(run *Run, rawLog []byte) error {
	data, err := json.MarshalIndent(run, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.runPath(run.ID, ".json"), data, 0o644); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(rawLog); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(s.runPath(run.ID, ".log.gz"), buf.Bytes(), 0o644)
}

// runs returns every saved run, oldest first.
func (s *Store) runs() ([]*Run, error) {
	paths, err := filepath.Glob(filepath.Join(s.results, "runs", "*.json"))
	if err != nil {
		return nil, err
	}
	var runs []*Run
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var r Run
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		runs = append(runs, &r)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt < runs[j].CreatedAt })
	return runs, nil
}

func (s *Store) findRun(id string) (*Run, error) {
	runs, err := s.runs()
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.ID == id {
			return run, nil
		}
	}
	return nil, fmt.Errorf("no run %q; see `zincbench list`", id)
}

func (s *Store) baseline() string {
	b, _ := os.ReadFile(filepath.Join(s.results, "BASELINE"))
	return strings.TrimSpace(string(b))
}

func (s *Store) baselineRun() *Run {
	id := s.baseline()
	if id == "" {
		return nil
	}
	data, err := os.ReadFile(s.runPath(id, ".json"))
	if err != nil {
		return nil
	}
	var r Run
	if json.Unmarshal(data, &r) != nil {
		return nil
	}
	return &r
}

func (s *Store) setBaseline(id string) error {
	return os.WriteFile(filepath.Join(s.results, "BASELINE"), []byte(id+"\n"), 0o644)
}

func runID(t time.Time, git GitInfo) string {
	id := t.UTC().Format("20060102-150405") + "-" + git.Short
	if git.Dirty {
		id += "-dirty"
	}
	return id
}

func gitOut(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// currentGit describes HEAD plus any uncommitted changes, so two experiments
// on the same commit get different identities.
func (s *Store) currentGit() GitInfo {
	g := GitInfo{
		Commit:  gitOut(s.root, "rev-parse", "HEAD"),
		Short:   gitOut(s.root, "rev-parse", "--short", "HEAD"),
		Branch:  gitOut(s.root, "rev-parse", "--abbrev-ref", "HEAD"),
		Subject: gitOut(s.root, "show", "-s", "--format=%s", "HEAD"),
	}
	// Porcelain lines start with a two-character status, often a space, so do not trim them.
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = s.root
	out, _ := cmd.Output()
	status := strings.TrimRight(string(out), "\n")
	if status == "" {
		return g
	}
	g.Dirty = true
	h := sha256.New()
	h.Write([]byte(gitOut(s.root, "diff", "HEAD", "--binary")))
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 4 {
			continue
		}
		path := line[3:]
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:] // renamed: keep the new path
		}
		g.Changed = append(g.Changed, path)
		if strings.HasPrefix(line, "??") {
			if data, err := os.ReadFile(filepath.Join(s.root, path)); err == nil {
				h.Write([]byte(path))
				h.Write(data)
			}
		}
	}
	g.DiffHash = hex.EncodeToString(h.Sum(nil))[:12]
	return g
}

// gitAt describes a specific commit, for imported runs.
func (s *Store) gitAt(commit string) (GitInfo, error) {
	out := gitOut(s.root, "show", "-s", "--format=%H%n%h%n%s", commit)
	parts := strings.SplitN(out, "\n", 3)
	if len(parts) < 3 {
		return GitInfo{}, fmt.Errorf("unknown commit %q", commit)
	}
	return GitInfo{Commit: parts[0], Short: parts[1], Subject: parts[2]}, nil
}

var requireLine = regexp.MustCompile(`^\s*(?:require\s+)?(github\.com/(?:gin-gonic/gin|labstack/echo/v\d+|go-chi/chi/v\d+))\s+(v\S+)`)

// rivalVersions reads the peer framework versions from benchmarks/go.mod.
func (s *Store) rivalVersions() map[string]string {
	data, err := os.ReadFile(filepath.Join(s.root, "benchmarks", "go.mod"))
	if err != nil {
		return nil
	}
	versions := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		m := requireLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch {
		case strings.Contains(m[1], "gin-gonic"):
			versions["Gin"] = m[2]
		case strings.Contains(m[1], "labstack"):
			versions["Echo"] = m[2]
		case strings.Contains(m[1], "go-chi"):
			versions["Chi"] = m[2]
		case strings.Contains(m[1], "uptrace/bunrouter"):
			versions["BunRouter"] = m[2]
		}
	}
	return versions
}

func goVersion() string {
	out, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolveRun finds a run by ID or unique ID prefix. "latest" names the newest
// run and "" names the pinned baseline.
func (s *Store) resolveRun(id string) (*Run, error) {
	runs, err := s.runs()
	if err != nil {
		return nil, err
	}
	switch id {
	case "":
		if id = s.baseline(); id == "" {
			return nil, errors.New("no baseline pinned; run `zincbench baseline <run-id>`")
		}
	case "latest":
		if len(runs) == 0 {
			return nil, errors.New("no runs recorded")
		}
		return runs[len(runs)-1], nil
	}
	var match *Run
	for _, run := range runs {
		if run.ID == id {
			return run, nil
		}
		if strings.HasPrefix(run.ID, id) {
			if match != nil {
				return nil, fmt.Errorf("%q matches more than one run", id)
			}
			match = run
		}
	}
	if match == nil {
		return nil, fmt.Errorf("no run matches %q; see `zincbench list`", id)
	}
	return match, nil
}

// rivalSource resolves the run whose rival samples a Zinc-only record reuses.
// A run that itself borrowed its rivals is fine: its samples are copies.
func (s *Store) rivalSource(id string) (*Run, error) {
	run, err := s.resolveRun(id)
	if err != nil {
		return nil, err
	}
	for _, sc := range run.Scenarios {
		if !headline.covers(sc) {
			return nil, fmt.Errorf("run %s has no rival samples to reuse", run.ID)
		}
		break
	}
	return run, nil
}
