package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cmdBundle creates a portable archive of every saved run for a release, plus
// any explicitly named reports or independent benchmark suites.
func cmdBundle(s *Store, args []string) error {
	fs := newFlags("bundle")
	release := fs.String("release", "", "release to bundle, e.g. 0.3.0")
	out := fs.String("o", "", "output zip (default benchmarks/results/bundles/<release>.zip)")
	include := fs.String("include", "", "comma-separated extra files or directories, relative to the repository root")
	_ = fs.Parse(args)
	if *release == "" {
		return errors.New("bundle needs -release")
	}
	runs, err := s.runs()
	if err != nil {
		return err
	}
	selected := make([]*Run, 0)
	for _, run := range runs {
		if run.Release == *release {
			selected = append(selected, run)
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("no runs recorded for release %q", *release)
	}
	if *out == "" {
		name := strings.ReplaceAll(*release, "/", "-") + ".zip"
		*out = filepath.Join(s.results, "bundles", name)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	manifest := struct {
		Release string   `json:"release"`
		Created string   `json:"createdAt"`
		Runs    []string `json:"runs"`
		Extras  []string `json:"extras,omitempty"`
	}{Release: *release, Created: time.Now().UTC().Format(time.RFC3339)}
	for _, run := range selected {
		manifest.Runs = append(manifest.Runs, run.ID)
		for _, ext := range []string{".json", ".log.gz"} {
			path := s.runPath(run.ID, ext)
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if err := addZipFile(zw, path, filepath.ToSlash(filepath.Join("runs", run.ID+ext))); err != nil {
				return err
			}
		}
	}
	if *include != "" {
		for _, item := range strings.Split(*include, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			path := filepath.Join(s.root, item)
			rel, err := filepath.Rel(s.root, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("-include path %q is outside the repository", item)
			}
			if err := filepath.Walk(path, func(file string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return nil
				}
				entry, err := filepath.Rel(s.root, file)
				if err != nil {
					return err
				}
				manifest.Extras = append(manifest.Extras, filepath.ToSlash(entry))
				return addZipFile(zw, file, filepath.ToSlash(filepath.Join("extras", entry)))
			}); err != nil {
				return err
			}
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	w, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("Release bundle: %s (%d runs, %d extra files)\n", *out, len(selected), len(manifest.Extras))
	return nil
}

func addZipFile(zw *zip.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
