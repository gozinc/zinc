// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import "testing"

func TestVersionHelpers(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
	if GetVersion() != Version {
		t.Fatalf("GetVersion=%q Version=%q", GetVersion(), Version)
	}
	if GetVersionHeader() != "Zinc/"+Version {
		t.Fatalf("header=%q", GetVersionHeader())
	}
}
