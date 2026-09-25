package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed dashboard.html
var dashboardTemplate string

func cmdDash(s *Store, args []string) error {
	fs := newFlags("dash")
	out := fs.String("o", "", "output file (default benchmarks/results/index.html)")
	open := fs.Bool("open", false, "open the dashboard in the default browser")
	_ = fs.Parse(args)

	path, err := buildDashboard(s, *out)
	if err != nil {
		return err
	}
	fmt.Println("Dashboard:", path)
	if *open {
		return openBrowser(path)
	}
	return nil
}

func buildDashboard(s *Store, out string) (string, error) {
	runs, err := s.runs()
	if err != nil {
		return "", err
	}
	if out == "" {
		out = filepath.Join(s.results, "index.html")
	}
	if out, err = filepath.Abs(out); err != nil {
		return "", err
	}

	// The dashboard only needs head-to-head data; drop Zinc-only benchmarks.
	type view struct {
		*Run
		ZincOnly map[string]Samples `json:"zincOnly,omitempty"`
	}
	views := make([]view, len(runs))
	for i, r := range runs {
		views[i] = view{Run: r}
	}
	data, err := json.Marshal(map[string]any{
		"baseline": s.baseline(),
		"runs":     views,
	})
	if err != nil {
		return "", err
	}

	logo := "zn-spangle-hd.webp"
	if rel, err := filepath.Rel(filepath.Dir(out), filepath.Join(s.root, "website", "public", "zn-spangle-hd.webp")); err == nil {
		logo = filepath.ToSlash(rel)
	}
	html := strings.Replace(dashboardTemplate, "/*__DATA__*/null", string(data), 1)
	html = strings.Replace(html, "__LOGO__", logo, 1)
	return out, os.WriteFile(out, []byte(html), 0o644)
}

func openBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
