// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"testing"

	"github.com/0mjs/zinc/internal/marks"
)

// markedFactory makes closures from one literal, so they share a code
// address; each answers a probe with its own marks.
//
//go:noinline
func markedFactory(m *marks.Marks) Middleware {
	mw := func(c *Context) error {
		if c.request == marks.Probe {
			if m == nil {
				return nil
			}
			return m
		}
		return c.Next()
	}
	marks.Answers(mw)
	return mw
}

func TestMarksBelongToTheInstance(t *testing.T) {
	marked := markedFactory(&marks.Marks{Prerouting: "demo", Preflight: true})
	plain := markedFactory(nil)
	if got := middlewareMarks(marked); got.Prerouting != "demo" || !got.Preflight {
		t.Fatalf("the marked instance: %+v", got)
	}
	if got := middlewareMarks(plain); got != (marks.Marks{}) {
		t.Fatalf("an unmarked instance from the same factory: %+v", got)
	}
	// Made after the marked one, still unmarked.
	if got := middlewareMarks(markedFactory(nil)); got != (marks.Marks{}) {
		t.Fatalf("a later unmarked instance: %+v", got)
	}
	New().Group("/plain", plain)
	defer func() {
		if recover() == nil {
			t.Error("the marked instance registered on a group")
		}
	}()
	New().Group("/marked", marked)
}

//go:noinline
func countingFactory(calls *int) Middleware {
	return func(c *Context) error { *calls++; return c.Next() }
}

// Zinc never calls middleware whose code doesn't answer probes, even when
// it's wrapped with Skip.
func TestMarksNeverProbeOtherMiddleware(t *testing.T) {
	calls := 0
	mw := countingFactory(&calls)
	app := New()
	app.Use(mw, Skip(func(*Context) bool { return true }, mw))
	app.Group("/api", mw, Skip(func(*Context) bool { return false }, mw)).
		Get("/x", func(c *Context) error { return c.NoContent() })
	if calls != 0 {
		t.Fatalf("plain middleware ran %d times at registration", calls)
	}
	if middlewareMarks(mw) != (marks.Marks{}) || middlewareMarks(nil) != (marks.Marks{}) {
		t.Fatal("plain middleware has marks")
	}
}
