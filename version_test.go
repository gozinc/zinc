// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"regexp"
	"testing"
)

// Version is bumped by hand at release, so guard its shape.
func TestVersionIsSemantic(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`).MatchString(Version) {
		t.Fatalf("Version = %q, want MAJOR.MINOR.PATCH", Version)
	}
}
