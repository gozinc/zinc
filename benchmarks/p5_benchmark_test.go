// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	. "github.com/0mjs/zinc"
)

// Zinc-only benchmarks for 0.5 P5 (uncached dynamic matching), outside the
// head-to-head suite. They cover what suite v2's lowercase pools can't:
// paths with capital letters, which the default case-insensitive routing
// used to lowercase with a copy, and deep parameter routes.

func p5App(register func(app *App)) http.Handler {
	app := New()
	register(app)
	return app
}

func runP5(b *testing.B, h http.Handler, reqs []*http.Request) {
	rw := newDiscardResponseWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		h.ServeHTTP(rw, reqs[i%len(reqs)])
	}
	benchmarkSinkInt = rw.status + rw.bytes
}

// BenchmarkP5MixedCaseParam: one parameter route, IDs with capital letters.
func BenchmarkP5MixedCaseParam(b *testing.B) {
	h := p5App(func(app *App) {
		app.Get("/users/{id}", func(c *Context) error {
			benchmarkSinkString = c.Param("id")
			return c.String(benchmarkOKResponse)
		})
	})
	runP5(b, h, poolRequests(http.MethodGet, poolSize, 50, func(i int) string {
		return "/users/AbC" + strconv.FormatInt(int64(i), 36) + "XyZ"
	}))
}

// BenchmarkP5LowerCaseParam is the same route with lowercase IDs: the
// control for MixedCaseParam.
func BenchmarkP5LowerCaseParam(b *testing.B) {
	h := p5App(func(app *App) {
		app.Get("/users/{id}", func(c *Context) error {
			benchmarkSinkString = c.Param("id")
			return c.String(benchmarkOKResponse)
		})
	})
	runP5(b, h, poolRequests(http.MethodGet, poolSize, 51, func(i int) string {
		return "/users/abc" + strconv.FormatInt(int64(i), 36) + "xyz"
	}))
}

// BenchmarkP5CamelCaseRoutes: camel-case static segments around parameters,
// requested with the same spelling, like the Parse and Google+ APIs.
func BenchmarkP5CamelCaseRoutes(b *testing.B) {
	patterns := []string{
		"/1/classes/{className}/{objectId}", "/1/users/{objectId}/sessionToken",
		"/plus/v1/people/{userId}/activities/{collection}", "/1/installations/{objectId}/pushStatus",
		"/1/functions/{functionName}/jobStatus", "/plus/v1/activities/{activityId}/peopleList",
	}
	h := p5App(func(app *App) {
		for _, p := range patterns {
			app.Get(p, func(c *Context) error { return c.String(benchmarkOKResponse) })
		}
	})
	runP5(b, h, poolRequests(http.MethodGet, poolSize, 52, func(i int) string {
		p := patterns[i%len(patterns)]
		for _, name := range []string{"{className}", "{objectId}", "{userId}", "{collection}", "{functionName}", "{activityId}"} {
			p = strings.ReplaceAll(p, name, "Val"+strconv.Itoa(i))
		}
		return p
	}))
}

// BenchmarkP5Param20: twenty parameters, lowercase values.
func BenchmarkP5Param20(b *testing.B) {
	var pattern, names strings.Builder
	for i := 0; i < 20; i++ {
		pattern.WriteString("/{p" + strconv.Itoa(i) + "}")
		names.WriteString("p" + strconv.Itoa(i) + " ")
	}
	params := strings.Fields(names.String())
	h := p5App(func(app *App) {
		app.Get(pattern.String(), func(c *Context) error {
			n := 0
			for _, p := range params {
				n += len(c.Param(p))
			}
			benchmarkSinkInt = n
			return c.String(benchmarkOKResponse)
		})
	})
	runP5(b, h, poolRequests(http.MethodGet, poolSize, 53, func(i int) string {
		var p strings.Builder
		for j := 0; j < 20; j++ {
			p.WriteString("/v" + strconv.Itoa(i+j))
		}
		return p.String()
	}))
}
