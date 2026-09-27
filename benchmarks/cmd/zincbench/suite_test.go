package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestSuiteNamesExist catches a suite entry whose benchmark was renamed or
// removed, which would silently drop its scenarios from every record.
func TestSuiteNamesExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no benchmark files found: %v", err)
	}
	defined := map[string]bool{}
	fn := regexp.MustCompile(`(?m)^func Benchmark(\w+)\(b \*testing\.B\)`)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range fn.FindAllSubmatch(data, -1) {
			defined[string(m[1])] = true
		}
	}
	for _, name := range suite {
		if !defined[name] {
			t.Errorf("suite lists %s, but there is no Benchmark%s", name, name)
		}
	}
}
