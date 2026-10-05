// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package marks

import "testing"

func build(n int) func() int { return func() int { return n } }

func TestAnswers(t *testing.T) {
	answering := build(1)
	Answers(answering)
	if !CanAnswer(answering) {
		t.Fatal("a recorded function can't answer")
	}
	if CanAnswer(func() int { return 3 }) {
		t.Fatal("an unrelated function can answer")
	}
	var nilFn func()
	for _, v := range []any{nil, nilFn, 42} {
		Answers(v) // ignored
		if CanAnswer(v) {
			t.Fatalf("%v can answer", v)
		}
	}
}
