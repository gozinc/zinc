package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

// suite lists the benchmark functions in the 77-scenario head-to-head
// comparison. Zinc-only benchmarks in the module are deliberately excluded.
var suite = []string{
	"HelloWorld", "StaticRoute", "StaticRouteCold", "RouterParam", "RouterParamCold",
	"JSONResponse", "QueryParams", "MiddlewareChain", "NotFound",
	"LargeRouteSetStatic", "LargeRouteSetStaticMixed", "LargeRouteSetNotFound",
	"LargeRouteSetMethodMismatch", "LargeRouteSetParam", "LargeRouteSetParamMixed",
	"RouteRegistrationStatic", "RouteRegistrationParam",
	"APIParamQueryJSON", "APIHappyPath", "APIBindJSONHappyPath", "APIBindHeaderQueryJSON",
	"APIBindInvalidJSON", "APIBindValidationFailure", "APIBindMultipartHappyPath",
	"LargeJSONResponse", "LargeJSONBind", "StaticFileHit", "StaticFileNotFound",
	"NestedGroupMiddlewareAPI", "APIUnauthorizedReject",
	"ParallelStaticRoute", "ParallelRouterParam", "ParallelMiddlewareChain", "ParallelAPIHappyPath",
	"Param5", "Param10", "NestedGroupStatic", "NestedGroupParam", "NestedGroupNotFound",
	"NestedGroupMethodMismatch", "WildcardTail", "WildcardTailNotFound",
	"ScenarioRouteSetBuild", "ScenarioRouteSetStatic", "ScenarioRouteSetParam",
	"ScenarioRouteSetNotFound", "ScenarioRouteSetMethodMismatch", "ScenarioRouteSetAll",
}

func suitePattern() string {
	return "^Benchmark(" + strings.Join(suite, "|") + ")$"
}

func cmdRecord(s *Store, args []string) error {
	fs := newFlags("record")
	release := fs.String("release", "", "release this run belongs to, e.g. 0.3.0")
	note := fs.String("note", "", "what this run measures, e.g. \"router: cache promotion at 8\"")
	count := fs.Int("count", 10, "samples per benchmark")
	benchtime := fs.String("benchtime", "100ms", "go test -benchtime for each sample")
	bench := fs.String("bench", "", "override the benchmark regexp (default: the head-to-head suite)")
	pin := fs.Bool("baseline", false, "pin this run as the comparison baseline")
	toolchain := fs.String("toolchain", "", "Go toolchain, e.g. go1.27.1 (default: the baseline's, so runs stay comparable; \"local\" uses your go)")
	_ = fs.Parse(args)

	tc := *toolchain
	if tc == "" {
		if base := s.baselineRun(); base != nil {
			tc = base.Env.Go
		}
	}
	if tc == "local" {
		tc = ""
	}
	if tc != "" {
		os.Setenv("GOTOOLCHAIN", tc)
	}

	pattern := *bench
	if pattern == "" {
		pattern = suitePattern()
	}
	git := s.currentGit()
	started := time.Now()
	state := "clean"
	if git.Dirty {
		state = fmt.Sprintf("dirty, %d changed files", len(git.Changed))
	}
	fmt.Printf("Recording %s on %s (%s) with %s: %d × %s per benchmark\n\n", git.Short, git.Branch, state, goVersion(), *count, *benchtime)

	cmd := exec.Command("go", "test", "-run", "^$", "-bench", pattern, "-benchmem",
		"-count", fmt.Sprint(*count), "-benchtime", *benchtime, ".")
	cmd.Dir = filepath.Join(s.root, "benchmarks")
	var raw bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &raw)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test failed: %w", err)
	}

	run, err := buildRun(s, raw.Bytes(), started, git, *note, *benchtime, *count)
	if err != nil {
		return err
	}
	run.Release = *release
	return finish(s, run, raw.Bytes(), *pin)
}

func cmdImport(s *Store, args []string) error {
	fs := newFlags("import")
	release := fs.String("release", "", "release this run belongs to, e.g. 0.3.0")
	logPath := fs.String("log", "", "go test -bench output to import (.gz allowed)")
	commit := fs.String("commit", "", "commit the log measured")
	date := fs.String("date", "", "when the run happened (RFC 3339); defaults to the file time")
	note := fs.String("note", "", "label for the run")
	goVer := fs.String("go", "", "Go version used (default: current)")
	benchtime := fs.String("benchtime", "100ms", "benchtime the run used")
	pin := fs.Bool("baseline", false, "pin this run as the comparison baseline")
	_ = fs.Parse(args)
	if *logPath == "" {
		return errors.New("import needs -log")
	}

	raw, err := readMaybeGzip(*logPath)
	if err != nil {
		return err
	}
	when := time.Now()
	if st, err := os.Stat(*logPath); err == nil {
		when = st.ModTime()
	}
	if *date != "" {
		if when, err = time.Parse(time.RFC3339, *date); err != nil {
			return fmt.Errorf("-date: %w", err)
		}
	}
	git := GitInfo{Short: "unknown"}
	if *commit != "" {
		if git, err = s.gitAt(*commit); err != nil {
			return err
		}
	}
	run, err := buildRun(s, raw, when, git, *note, *benchtime, 0)
	if err != nil {
		return err
	}
	run.Imported = true
	run.Release = *release
	if *goVer != "" {
		run.Env.Go = *goVer
	}
	return finish(s, run, raw, *pin)
}

// cmdImportZinc joins a Zinc-only measurement with a saved rival run. The
// source ID remains in the record so the mixed-date comparison is explicit.
func cmdImportZinc(s *Store, args []string) error {
	fs := newFlags("import-zinc")
	logs := fs.String("logs", "", "comma-separated Zinc-only benchmark logs")
	rivals := fs.String("rivals", "", "saved head-to-head run supplying Gin, Echo, and Chi samples")
	commit := fs.String("commit", "", "Zinc commit measured")
	date := fs.String("date", "", "measurement date and time (RFC 3339)")
	note := fs.String("note", "", "label for the run")
	release := fs.String("release", "", "release this run belongs to")
	goVer := fs.String("go", "", "Go version used (defaults to source run's version)")
	benchtime := fs.String("benchtime", "", "benchtime used (defaults to source run's value)")
	_ = fs.Parse(args)
	if *logs == "" || *rivals == "" || *commit == "" || *date == "" {
		return errors.New("import-zinc needs -logs, -rivals, -commit, and -date")
	}
	when, err := time.Parse(time.RFC3339, *date)
	if err != nil {
		return fmt.Errorf("-date: %w", err)
	}
	source, err := s.findRun(*rivals)
	if err != nil {
		return err
	}
	git, err := s.gitAt(*commit)
	if err != nil {
		return err
	}

	var raw bytes.Buffer
	for _, path := range strings.Split(*logs, ",") {
		data, err := readMaybeGzip(strings.TrimSpace(path))
		if err != nil {
			return err
		}
		raw.Write(data)
		raw.WriteByte('\n')
	}
	partial, _, header, err := parseFrameworkOutput(bytes.NewReader(raw.Bytes()))
	if err != nil {
		return err
	}
	scenarios, count, err := mergeZincWithRivals(partial, source)
	if err != nil {
		return err
	}
	if header["goos"] != "" && header["goos"] != source.Env.GOOS {
		return errors.New("Zinc and rival runs have different GOOS")
	}
	if header["goarch"] != "" && header["goarch"] != source.Env.GOARCH {
		return errors.New("Zinc and rival runs have different GOARCH")
	}
	if header["cpu"] != "" && header["cpu"] != source.Env.CPU {
		return errors.New("Zinc and rival runs have different CPUs")
	}
	env := source.Env
	env.Count = count
	if *goVer != "" {
		env.Go = *goVer
	}
	if *benchtime != "" {
		env.Benchtime = *benchtime
	}
	run := &Run{Schema: 1, ID: runID(when, git), CreatedAt: when.UTC().Format(time.RFC3339),
		Release: *release, Note: *note, Imported: true, RivalSource: source.ID,
		Git: git, Env: env, Scenarios: scenarios}
	return finish(s, run, raw.Bytes(), false)
}

func mergeZincWithRivals(partial map[string]Scenario, source *Run) (map[string]Scenario, int, error) {
	if len(partial) != len(source.Scenarios) {
		return nil, 0, fmt.Errorf("Zinc logs have %d scenarios; rival run %s has %d", len(partial), source.ID, len(source.Scenarios))
	}
	scenarios := make(map[string]Scenario, len(partial))
	count := 0
	for name, row := range partial {
		zinc, ok := row["Zinc"]
		if !ok || len(row) != 1 {
			return nil, 0, fmt.Errorf("%s is not Zinc-only", name)
		}
		if count == 0 {
			count = len(zinc.NS)
		}
		if len(zinc.NS) != count {
			return nil, 0, fmt.Errorf("%s has %d Zinc samples; expected %d", name, len(zinc.NS), count)
		}
		peers, ok := source.Scenarios[name]
		if !ok {
			return nil, 0, fmt.Errorf("%s missing from rival run %s", name, source.ID)
		}
		combined := Scenario{"Zinc": zinc}
		for _, fw := range frameworks[1:] {
			peer, ok := peers[fw]
			if !ok {
				return nil, 0, fmt.Errorf("%s/%s missing from rival run %s", name, fw, source.ID)
			}
			combined[fw] = peer
		}
		scenarios[name] = combined
	}
	return scenarios, count, nil
}

func buildRun(s *Store, raw []byte, when time.Time, git GitInfo, note, benchtime string, count int) (*Run, error) {
	scenarios, zincOnly, header, err := parseOutput(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if len(scenarios) == 0 {
		return nil, errors.New("no head-to-head results found in the output")
	}
	if count == 0 {
		for _, sc := range scenarios {
			count = len(sc["Zinc"].NS)
			break
		}
	}
	host, _ := os.Hostname()
	return &Run{
		Schema:    1,
		ID:        runID(when, git),
		CreatedAt: when.UTC().Format(time.RFC3339),
		Note:      note,
		Git:       git,
		Env: Env{
			Go:        goVersion(),
			GOOS:      header["goos"],
			GOARCH:    header["goarch"],
			CPU:       header["cpu"],
			Host:      strings.TrimSuffix(host, ".local"),
			Benchtime: benchtime,
			Count:     count,
			Rivals:    s.rivalVersions(),
		},
		Scenarios: scenarios,
		ZincOnly:  zincOnly,
	}, nil
}

func finish(s *Store, run *Run, raw []byte, pin bool) error {
	if err := s.save(run, raw); err != nil {
		return err
	}
	fmt.Printf("\nSaved %s: %d scenarios, Zinc fastest in %d.\n", run.ID, len(run.Scenarios), run.wins())
	if pin || s.baseline() == "" {
		if err := s.setBaseline(run.ID); err != nil {
			return err
		}
		fmt.Printf("Pinned %s as the baseline.\n", run.ID)
	}
	out, err := buildDashboard(s, "")
	if err != nil {
		return err
	}
	fmt.Printf("Dashboard: %s\n", out)
	return nil
}

func cmdBaseline(s *Store, args []string) error {
	if len(args) == 0 {
		if b := s.baseline(); b != "" {
			fmt.Println(b)
			return nil
		}
		return errors.New("no baseline pinned; run `zincbench baseline <run-id>`")
	}
	runs, err := s.runs()
	if err != nil {
		return err
	}
	id := args[0]
	if id == "latest" && len(runs) > 0 {
		id = runs[len(runs)-1].ID
	}
	for _, r := range runs {
		if r.ID == id || strings.HasPrefix(r.ID, id) {
			if err := s.setBaseline(r.ID); err != nil {
				return err
			}
			fmt.Printf("Pinned %s as the baseline.\n", r.ID)
			_, err := buildDashboard(s, "")
			return err
		}
	}
	return fmt.Errorf("no run matches %q; see `zincbench list`", id)
}

func cmdList(s *Store) error {
	runs, err := s.runs()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Println("No runs yet. Record one with `zincbench record`.")
		return nil
	}
	base := s.baseline()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "\tRUN\tRELEASE\tWHEN\tCOMMIT\tWINS\tNOTE")
	for _, r := range runs {
		mark := ""
		if r.ID == base {
			mark = "base"
		}
		when, _ := time.Parse(time.RFC3339, r.CreatedAt)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d/%d\t%s\n", mark, r.ID, r.Release, when.Local().Format("Jan 02 15:04"), r.Git.Short, r.wins(), len(r.Scenarios), r.Note)
	}
	return w.Flush()
}

func readMaybeGzip(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(zr)
	}
	return data, nil
}
