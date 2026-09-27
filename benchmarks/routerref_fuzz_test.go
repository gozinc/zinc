// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	. "github.com/0mjs/zinc"
	"github.com/0mjs/zinc/benchmarks/internal/zinc040"
)

// FuzzRouterReference serves random route tables and random requests with the
// router under development and with Zinc v0.4.0 (internal/zinc040), and
// fails on any difference in status, headers, body, or the route and
// parameters a handler sees. ROUTER_SPEC.md describes the behaviour it holds
// fixed. Each request is served twice, so cache hits are checked as well as
// misses.
//
//	go test -run '^$' -fuzz '^FuzzRouterReference$' -fuzztime 60s .
func FuzzRouterReference(f *testing.F) {
	for _, seed := range []string{
		"",
		"\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f",
		"routes and requests",
		"\xff\xfe\xfd\xfc\xfb\xfa\xf9\xf8\xf7\xf6\xf5\xf4\xf3\xf2\xf1\xf0",
		"\x07\x03\x01\x02\x00\x05\x09\x04\x08\x06\x0a\x01\x03\x05\x07\x09\x02\x04",
		strings.Repeat("\x13\x37", 64),
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &fuzzGen{data: data}
		spec := g.spec()
		cur, ref, routes := buildReferencePair(t, spec)
		for i := 0; i < spec.requests; i++ {
			method, target := g.request(routes)
			for pass := 0; pass < 2; pass++ {
				got := serveReference(cur, method, target)
				want := serveReference(ref, method, target)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s %s (pass %d) with %s, RouteNotFound %q, mount %q\nroutes: %v\ngot  %+v\nwant %+v",
						method, target, pass+1, spec.config, spec.notFound, spec.mount, routes, got, want)
				}
			}
		}
	})
}

// fuzzGen turns fuzz input into choices; exhausted input reads as zeros.
type fuzzGen struct {
	data []byte
	pos  int
}

func (g *fuzzGen) byte() byte {
	if g.pos >= len(g.data) {
		g.pos++
		return byte(g.pos * 31)
	}
	b := g.data[g.pos]
	g.pos++
	return b
}

func (g *fuzzGen) pick(n int) int { return int(g.byte()) % n }

func pickFrom[T any](g *fuzzGen, xs []T) T { return xs[g.pick(len(xs))] }

var (
	fuzzStatic  = []string{"users", "items", "api", "v1", "Users", "a", "b", "files", "health", "é"}
	fuzzParams  = []string{"id", "name", "slug"}
	fuzzMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	fuzzReqMeth = []string{http.MethodGet, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions}
	fuzzValues  = []string{"1", "42", "abc", "Users", "x.y", "a-b", "%20", "%2F", "é"}
)

type fuzzSpec struct {
	config   fuzzConfig
	routes   []fuzzRoute
	requests int
	// notFound is a RouteNotFound pattern and mount a mount prefix; empty
	// means none.
	notFound, mount string
}

type fuzzConfig struct {
	strict, caseSensitive, noHead, noOptions, noMethodNotAllowed bool
	cacheSize                                                     int
}

func (c fuzzConfig) String() string {
	return fmt.Sprintf("strict=%v caseSensitive=%v noHead=%v noOptions=%v no405=%v cache=%d",
		c.strict, c.caseSensitive, c.noHead, c.noOptions, c.noMethodNotAllowed, c.cacheSize)
}

type fuzzRoute struct {
	method, pattern string
	params          []string
}

func (g *fuzzGen) spec() fuzzSpec {
	flags := g.byte()
	s := fuzzSpec{config: fuzzConfig{
		strict:             flags&1 != 0,
		caseSensitive:      flags&2 != 0,
		noHead:             flags&4 != 0 && flags&32 != 0,
		noOptions:          flags&8 != 0 && flags&32 != 0,
		noMethodNotAllowed: flags&16 != 0 && flags&64 != 0,
		cacheSize:          []int{0, -1, 2}[g.pick(3)],
	}}
	for n := 1 + g.pick(24); n > 0; n-- {
		s.routes = append(s.routes, g.route())
	}
	s.requests = 1 + g.pick(32)
	if g.pick(3) == 0 {
		s.notFound = "/" + pickFrom(g, fuzzStatic) + "/{rest...}"
	}
	if g.pick(3) == 0 {
		s.mount = "/" + pickFrom(g, fuzzStatic)
	}
	return s
}

func (g *fuzzGen) route() fuzzRoute {
	r := fuzzRoute{method: pickFrom(g, fuzzMethods)}
	var b strings.Builder
	depth := g.pick(5)
	used := map[string]bool{}
	for d := 0; d < depth; d++ {
		b.WriteByte('/')
		switch k := g.pick(10); {
		case k < 6:
			b.WriteString(pickFrom(g, fuzzStatic))
		case k < 9 || d < depth-1:
			name := pickFrom(g, fuzzParams)
			if used[name] {
				name += fmt.Sprint(d)
			}
			used[name] = true
			r.params = append(r.params, name)
			b.WriteString("{" + name + "}")
		default:
			r.params = append(r.params, "tail")
			b.WriteString("{tail...}")
		}
	}
	if depth > 0 && g.pick(6) == 0 && !strings.HasSuffix(b.String(), "...}") {
		b.WriteByte('/')
	}
	r.pattern = b.String()
	if r.pattern == "" {
		r.pattern = "/"
	}
	return r
}

// request returns a request near a registered route: the route's own path,
// or one with a changed case, method, trailing slash or segment count.
func (g *fuzzGen) request(routes []fuzzRoute) (method, target string) {
	method = pickFrom(g, fuzzReqMeth)
	var segments []string
	if len(routes) > 0 && g.pick(5) != 0 {
		r := pickFrom(g, routes)
		if g.pick(3) == 0 {
			method = r.method
		}
		for _, seg := range strings.Split(strings.Trim(r.pattern, "/"), "/") {
			switch {
			case seg == "":
			case strings.HasPrefix(seg, "{"):
				v := pickFrom(g, fuzzValues)
				if strings.HasSuffix(seg, "...}") && g.pick(2) == 0 {
					v += "/" + pickFrom(g, fuzzValues)
				}
				segments = append(segments, v)
			default:
				segments = append(segments, seg)
			}
		}
	} else {
		for n := g.pick(4); n > 0; n-- {
			segments = append(segments, pickFrom(g, fuzzStatic))
		}
	}
	switch g.pick(8) {
	case 0:
		if len(segments) > 0 {
			i := g.pick(len(segments))
			segments[i] = strings.ToUpper(segments[i])
		}
	case 1:
		segments = append(segments, pickFrom(g, fuzzStatic))
	case 2:
		if len(segments) > 0 {
			segments = segments[:len(segments)-1]
		}
	}
	target = "/" + strings.Join(segments, "/")
	switch g.pick(8) {
	case 0:
		target += "/"
	case 1:
		target = strings.Replace(target, "/", "//", 1)
	}
	return method, target
}

// buildReferencePair registers the same routes, in the same order, in the
// router under development and in v0.4.0. Registration must succeed or fail
// the same way in both; routes that both reject are left out.
func buildReferencePair(t *testing.T, s fuzzSpec) (cur, ref http.Handler, routes []fuzzRoute) {
	c := s.config
	curApp := New(Config{StrictRouting: c.strict, CaseSensitive: c.caseSensitive, DisableAutoHead: c.noHead,
		DisableAutoOptions: c.noOptions, DisableMethodNotAllowed: c.noMethodNotAllowed, RouteCacheSize: c.cacheSize})
	refApp := zinc040.New(zinc040.Config{StrictRouting: c.strict, CaseSensitive: c.caseSensitive, DisableAutoHead: c.noHead,
		DisableAutoOptions: c.noOptions, DisableMethodNotAllowed: c.noMethodNotAllowed, RouteCacheSize: c.cacheSize})
	for i, r := range s.routes {
		id, params := i, r.params
		curErr := registerErr(func() {
			curApp.Add(r.method, r.pattern, func(ctx *Context) error {
				return ctx.String(routeReport(id, params, ctx.Param))
			})
		})
		refErr := registerErr(func() {
			refApp.Add(r.method, r.pattern, func(ctx *zinc040.Context) error {
				return ctx.String(routeReport(id, params, ctx.Param))
			})
		})
		if curErr != refErr {
			t.Fatalf("registering %s %s: got error %q, v0.4.0 %q\nearlier routes: %v", r.method, r.pattern, curErr, refErr, routes)
		}
		if curErr == "" {
			routes = append(routes, r)
		}
	}
	if s.notFound != "" {
		curErr := registerErr(func() {
			curApp.RouteNotFound(s.notFound, func(ctx *Context) error { return ctx.String("scoped 404 " + ctx.Param("rest")) })
		})
		refErr := registerErr(func() {
			refApp.RouteNotFound(s.notFound, func(ctx *zinc040.Context) error { return ctx.String("scoped 404 " + ctx.Param("rest")) })
		})
		if curErr != refErr {
			t.Fatalf("RouteNotFound %s: got error %q, v0.4.0 %q", s.notFound, curErr, refErr)
		}
	}
	if s.mount != "" {
		mounted := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, "mounted %s", r.URL.Path)
		})
		curErr := registerErr(func() { curApp.Mount(s.mount, mounted) })
		refErr := registerErr(func() { refApp.Mount(s.mount, mounted) })
		if curErr != refErr {
			t.Fatalf("Mount %s: got error %q, v0.4.0 %q", s.mount, curErr, refErr)
		}
	}
	return curApp, refApp, routes
}

func routeReport(id int, params []string, param func(string) string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "route %d", id)
	for _, p := range params {
		fmt.Fprintf(&b, " %s=%q", p, param(p))
	}
	return b.String()
}

func registerErr(register func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	register()
	return ""
}

type referenceResponse struct {
	Status int
	Header http.Header
	Body   string
}

func serveReference(h http.Handler, method, target string) referenceResponse {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return referenceResponse{Status: rec.Code, Header: rec.Header(), Body: rec.Body.String()}
}
