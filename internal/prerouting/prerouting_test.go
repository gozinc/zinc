// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package prerouting

import "testing"

func build(n int) func() int { return func() int { return n } }

func TestMarkCoversEveryClosureFromOneFunction(t *testing.T) {
	Mark(build(1), "demo")
	if name, ok := Name(build(2)); !ok || name != "demo" {
		t.Fatalf("a second closure from the same function: %q %v", name, ok)
	}
	if _, ok := Name(func() int { return 3 }); ok {
		t.Fatal("an unrelated function was marked")
	}
	var nilFn func()
	for _, v := range []any{nil, nilFn, 42} {
		Mark(v, "bad") // ignored
		if _, ok := Name(v); ok {
			t.Fatalf("%v was marked", v)
		}
	}
}
