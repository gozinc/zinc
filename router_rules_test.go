// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The router's rules, stated as properties rather than as today's answers:
//
//  1. Adding a route for one method never changes which route serves another
//     method, and turning 405s off never changes which route matches.
//  2. Dispatch, FindRoute and the Allow header agree: a method is in Allow
//     exactly when dispatch serves it, and FindRoute finds the route dispatch
//     runs, with the same parameters (automatic HEAD included).
//  3. A URL built for a named route resolves back to that route with the same
//     values, or building it fails.

var ruleConfigs = []struct {
	name   string
	config Config
}{
	{"default", Config{}},
	{"strict", Config{StrictRouting: true}},
	{"case-sensitive", Config{CaseSensitive: true}},
	{"strict case-sensitive", Config{StrictRouting: true, CaseSensitive: true}},
}

func reportRoute(c *Context) error {
	c.SetHeader("X-Route", c.Route().Name)
	return c.String(c.Route().Name + paramReport(c.Route().Params, c.Param))
}

func paramReport(names []string, param func(string) string) string {
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, " %s=%q", name, param(name))
	}
	return b.String()
}

// Rule 1, the reported case: a POST route with a trailing slash beside a GET
// route without one. POST /x/123/ must reach the POST route whether or not
// 405s are on, and Allow, FindRoute and dispatch must agree.
func TestRuleOtherMethodNeverHidesRoute(t *testing.T) {
	for _, tc := range ruleConfigs {
		for _, no405 := range []bool{false, true} {
			config := tc.config
			config.DisableMethodNotAllowed = no405
			app := New(config)
			app.Get("/x/{id}", reportRoute).Name("get")
			app.Post("/x/{id}/", reportRoute).Name("post")

			rec := performRequest(t, app, MethodPost, "/x/123/", nil, nil)
			if rec.Code != http.StatusOK || rec.Header().Get("X-Route") != "post" {
				t.Errorf("%s no405=%v: POST /x/123/ = %d %q, want the POST route", tc.name, no405, rec.Code, rec.Body.String())
			}
			if info, ok := app.FindRoute(MethodPost, "/x/123/"); !ok || info.Name != "post" {
				t.Errorf("%s no405=%v: FindRoute(POST /x/123/) = %+v %v", tc.name, no405, info, ok)
			}
			rec = performRequest(t, app, MethodGet, "/x/123", nil, nil)
			if rec.Header().Get("X-Route") != "get" {
				t.Errorf("%s no405=%v: GET /x/123 = %d %q, want the GET route", tc.name, no405, rec.Code, rec.Body.String())
			}
		}
	}
}

// A parameter route registered with a trailing slash is reachable without it
// under default routing, as a static route is.
func TestRuleDynamicTrailingSlashRoute(t *testing.T) {
	for _, tc := range ruleConfigs {
		app := New(tc.config)
		app.Get("/x/{id}/", reportRoute).Name("slash")
		app.Get("/s/", reportRoute).Name("static")
		for _, target := range []string{"/x/1/", "/x/1", "/s/", "/s"} {
			rec := performRequest(t, app, MethodGet, target, nil, nil)
			want := !tc.config.StrictRouting || strings.HasSuffix(target, "/")
			if got := rec.Code == http.StatusOK; got != want {
				t.Errorf("%s: GET %s = %d, want served=%v", tc.name, target, rec.Code, want)
			}
			if _, found := app.FindRoute(MethodGet, target); found != want {
				t.Errorf("%s: FindRoute(GET %s) = %v, want %v", tc.name, target, found, want)
			}
		}
	}
}

// Two routes for one method that match the same requests can't both be
// served, so registering the second panics, naming both.
func TestRuleUnreachableDuplicateRejected(t *testing.T) {
	h := func(*Context) error { return nil }
	for _, order := range [][2]string{{"/x/{id}", "/x/{id}/"}, {"/x/{id}/", "/x/{id}"}, {"/s", "/s/"}, {"/X/{id}", "/x/{name}/"}} {
		app := New()
		app.Get(order[0], h)
		mustPanic(t, "GET "+order[1], func() { app.Get(order[1], h) })
		mustPanic(t, "GET "+order[0], func() { app.Get(order[1], h) })
		mustPanic(t, "route already registered", func() { app.Get(order[1], h) })

		// Under strict routing the trailing slash makes them different paths.
		strict := New(Config{StrictRouting: true})
		strict.Get(order[0], h)
		strict.Get(order[1], h)
	}
	app := New()
	app.Get("/same", h)
	mustPanic(t, "route already registered: GET /same", func() { app.Get("/same", h) })
}

func TestRuleFindRouteAutomaticHead(t *testing.T) {
	app := New()
	app.Get("/x/{id}", reportRoute).Name("get")
	if info, ok := app.FindRoute(MethodHead, "/x/1"); !ok || info.Method != MethodGet || info.Name != "get" {
		t.Fatalf("FindRoute(HEAD) = %+v %v, want the GET route", info, ok)
	}
	app.Head("/x/{id}", reportRoute).Name("head")
	if info, ok := app.FindRoute(MethodHead, "/x/1"); !ok || info.Name != "head" {
		t.Fatalf("FindRoute(HEAD) with a HEAD route = %+v %v", info, ok)
	}
	off := New(Config{DisableAutoHead: true})
	off.Get("/x/{id}", reportRoute)
	if info, ok := off.FindRoute(MethodHead, "/x/1"); ok {
		t.Fatalf("FindRoute(HEAD) without automatic HEAD = %+v", info)
	}
}

func TestRuleParameterLimit(t *testing.T) {
	pattern, path, names, _ := buildSequentialParamRoute(maxRouteParams)
	app := New()
	app.Get(pattern, func(c *Context) error { return c.String(c.Param(names[0]) + "|" + c.Param(names[len(names)-1])) })
	if rec := performRequest(t, app, MethodGet, path, nil, nil); rec.Body.String() != "1|"+strconv.Itoa(maxRouteParams) {
		t.Fatalf("%d params: %q", maxRouteParams, rec.Body.String())
	}
	wide, _, _, _ := buildSequentialParamRoute(maxRouteParams + 1)
	mustPanic(t, "at most "+strconv.Itoa(maxRouteParams), func() { New().Get(wide, reportRoute) })
}

func TestRuleNilHandlerRejected(t *testing.T) {
	h := func(*Context) error { return nil }
	mustPanic(t, "nil handler", func() { New().Get("/nil", nil) })
	mustPanic(t, "nil handler", func() { New().Get("/nil", nil, h) })
	mustPanic(t, "nil handler", func() { New().Group("/g").Get("/nil", nil) })
	mustPanic(t, "nil handler", func() { New().RouteNotFound("/nil", nil) })
}

// Rule 3: every value either routes back unchanged or fails to build.
func TestRuleURLRoundTrip(t *testing.T) {
	catchAll := []string{"", "a", "a/", "/a", "//a", "/", "//", "a//b", "a/b/", "a?x#b%", "A/B", "é/", "%2F"}
	segments := []string{"a", "A", "é", "x.y", "a b", "%", "me"}
	for _, tc := range ruleConfigs {
		app := New(tc.config)
		app.Get("/files/{path...}", reportRoute).Name("files")
		app.Get("/u/{id}", reportRoute).Name("user")
		app.Get("/u/me", reportRoute).Name("me")
		app.Get("/d/{id}/", reportRoute).Name("slash")
		app.Get("/n/{id}/f/{rest...}", reportRoute).Name("nested")
		app.Get("/bare", reportRoute).Name("bare")
		app.Get("/bare/{rest...}", reportRoute).Name("bare.rest")
		check := func(name string, values ...string) bool {
			t.Helper()
			url, err := app.URL(name, values...)
			if err != nil {
				return false
			}
			rec := performRequest(t, app, MethodGet, url, nil, nil)
			info, _ := app.RouteByName(name)
			want := name + paramReport(info.Params, func(p string) string { return values[slices.Index(info.Params, p)] })
			if rec.Code != http.StatusOK || rec.Body.String() != want {
				t.Errorf("%s: URL(%s, %q) = %q serves %d %q, want %q", tc.name, name, values, url, rec.Code, rec.Body.String(), want)
			}
			return true
		}
		for _, v := range catchAll {
			// Every value but the empty one is representable in every mode.
			if !check("files", v) && v != "" {
				t.Errorf("%s: URL(files, %q) failed", tc.name, v)
			}
			check("nested", "1", v)
			check("bare.rest", v)
		}
		for _, v := range segments {
			ok := check("user", v)
			if want := v != "me"; ok != want {
				t.Errorf("%s: URL(user, %q) built=%v, want %v", tc.name, v, ok, want)
			}
			if !check("slash", v) {
				t.Errorf("%s: URL(slash, %q) failed", tc.name, v)
			}
		}
		// Under default routing /bare/ is /bare, so the empty value has no URL.
		if _, err := app.URL("bare.rest", ""); (err == nil) != tc.config.StrictRouting {
			t.Errorf("%s: URL(bare.rest, \"\") err=%v", tc.name, err)
		}
	}
}

// ruleRoute is one generated route: its method, pattern and name.
type ruleRoute struct{ method, pattern string }

var (
	ruleStatic  = []string{"a", "b", "users", "Users", "é", "v1"}
	ruleParams  = []string{"id", "name", "slug"}
	ruleMethods = []string{MethodGet, MethodPost, MethodPut, MethodDelete, MethodPatch}
	ruleValues  = []string{"1", "a", "A", "users", "é", "x.y"}
)

func genRuleRoute(rng *rand.Rand) ruleRoute {
	var b strings.Builder
	depth := rng.IntN(4)
	for d := 0; d < depth; d++ {
		b.WriteByte('/')
		switch k := rng.IntN(10); {
		case k < 5:
			b.WriteString(ruleStatic[rng.IntN(len(ruleStatic))])
		case k < 9 || d < depth-1:
			b.WriteString("{" + ruleParams[rng.IntN(len(ruleParams))] + strconv.Itoa(d) + "}")
		default:
			b.WriteString("{tail...}")
		}
	}
	if depth > 0 && rng.IntN(3) == 0 && !strings.HasSuffix(b.String(), "...}") {
		b.WriteByte('/')
	}
	pattern := b.String()
	if pattern == "" {
		pattern = "/"
	}
	return ruleRoute{method: ruleMethods[rng.IntN(len(ruleMethods))], pattern: pattern}
}

func genRulePath(rng *rand.Rand, routes []ruleRoute) string {
	var segs []string
	if len(routes) > 0 && rng.IntN(5) != 0 {
		for _, seg := range strings.Split(strings.Trim(routes[rng.IntN(len(routes))].pattern, "/"), "/") {
			switch {
			case seg == "":
			case strings.HasPrefix(seg, "{"):
				v := ruleValues[rng.IntN(len(ruleValues))]
				if strings.HasSuffix(seg, "...}") && rng.IntN(2) == 0 {
					v += "/" + ruleValues[rng.IntN(len(ruleValues))]
				}
				segs = append(segs, v)
			default:
				segs = append(segs, seg)
			}
		}
	} else {
		for n := rng.IntN(4); n > 0; n-- {
			segs = append(segs, ruleStatic[rng.IntN(len(ruleStatic))])
		}
	}
	if len(segs) > 0 && rng.IntN(6) == 0 {
		i := rng.IntN(len(segs))
		segs[i] = strings.ToUpper(segs[i])
	}
	path := "/" + strings.Join(segs, "/")
	switch rng.IntN(5) {
	case 0:
		path += "/"
	case 1:
		path = strings.Replace(path, "/", "//", 1)
	}
	return path
}

// buildRuleApp registers routes, skipping those registration rejects, and
// returns the routes it kept.
func buildRuleApp(config Config, routes []ruleRoute) (*App, []ruleRoute) {
	app := New(config)
	var kept []ruleRoute
	for _, r := range routes {
		name := strconv.Itoa(len(kept))
		if registerPanics(func() { app.Add(r.method, r.pattern, reportRoute).Name(name) }) {
			continue
		}
		kept = append(kept, r)
	}
	return app, kept
}

func registerPanics(register func()) (panicked bool) {
	defer func() { panicked = recover() != nil }()
	register()
	return false
}

type ruleOutcome struct {
	status int
	route  string // the serving route's name, empty for none
	body   string
	allow  string
}

func serveRule(app *App, method, path string) ruleOutcome {
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(method, "http://example.com"+path, nil))
	return ruleOutcome{status: rec.Code, route: rec.Header().Get("X-Route"), body: rec.Body.String(), allow: rec.Header().Get(HeaderAllow)}
}

// checkRouterRules checks rules 1 and 2 for one generated table.
func checkRouterRules(t *testing.T, rng *rand.Rand) {
	t.Helper()
	tc := ruleConfigs[rng.IntN(len(ruleConfigs))]
	var generated []ruleRoute
	for n := 1 + rng.IntN(12); n > 0; n-- {
		generated = append(generated, genRuleRoute(rng))
	}
	app, routes := buildRuleApp(tc.config, generated)
	no405Config := tc.config
	no405Config.DisableMethodNotAllowed = true
	no405, _ := buildRuleApp(no405Config, routes)
	extra := genRuleRoute(rng)
	withExtra, extraRoutes := buildRuleApp(tc.config, append(slices.Clone(routes), extra))
	extraKept := len(extraRoutes) > len(routes)

	for i := 0; i < 16; i++ {
		path := genRulePath(rng, routes)
		served := map[string]ruleOutcome{}
		var allowed []string
		for _, method := range ruleMethods {
			got := serveRule(app, method, path)
			served[method] = got
			if got.route != "" {
				allowed = append(allowed, method)
			}

			// Rule 2: FindRoute finds the route dispatch ran, with its values.
			_, found := app.router.Find(method, path)
			info, ok := app.FindRoute(method, path)
			if (got.route != "") != ok || (ok && info.Name != got.route) {
				t.Fatalf("%s: %s %s: dispatch ran %q (%d), FindRoute = %q %v\nroutes %v", tc.name, method, path, got.route, got.status, info.Name, ok, routes)
			}
			if found != nil {
				if want := found.Route().Name + paramReport(found.Route().Params, found.Param); want != got.body {
					t.Fatalf("%s: %s %s: dispatch %q, Find %q\nroutes %v", tc.name, method, path, got.body, want, routes)
				}
			}

			// Rule 1: 405s off doesn't change the match.
			if off := serveRule(no405, method, path); off.route != got.route || (got.route != "" && off.body != got.body) {
				t.Fatalf("%s: %s %s: with 405s %q %q, without %q %q\nroutes %v", tc.name, method, path, got.route, got.body, off.route, off.body, routes)
			}
			// Rule 1: a route for another method changes nothing.
			if extraKept && method != extra.method {
				if again := serveRule(withExtra, method, path); again.route != got.route || again.body != got.body && got.route != "" {
					t.Fatalf("%s: %s %s: served %q %q, after adding %s %s served %q %q\nroutes %v", tc.name, method, path, got.route, got.body, extra.method, extra.pattern, again.route, again.body, routes)
				}
			}
		}

		// Automatic HEAD runs the GET route.
		head := serveRule(app, MethodHead, path)
		if head.route != served[MethodGet].route {
			t.Fatalf("%s: HEAD %s ran %q, GET ran %q\nroutes %v", tc.name, path, head.route, served[MethodGet].route, routes)
		}
		if info, ok := app.FindRoute(MethodHead, path); ok != (head.route != "") || ok && info.Name != head.route {
			t.Fatalf("%s: HEAD %s ran %q, FindRoute = %q %v\nroutes %v", tc.name, path, head.route, info.Name, ok, routes)
		}

		// Rule 2: Allow lists exactly the methods dispatch serves.
		want := ""
		if len(allowed) > 0 {
			set := allowedMethodSet{}
			for _, m := range allowed {
				set.addMethod(m)
			}
			want = set.header(true, true)
		}
		options := serveRule(app, MethodOptions, path)
		if options.allow != want || (want != "" && options.status != http.StatusNoContent) {
			t.Fatalf("%s: OPTIONS %s = %d Allow %q, want Allow %q (served %v)\nroutes %v", tc.name, path, options.status, options.allow, want, allowed, routes)
		}
		for _, method := range ruleMethods {
			got := served[method]
			if got.route != "" {
				continue
			}
			wantStatus := http.StatusNotFound
			if want != "" {
				wantStatus = http.StatusMethodNotAllowed
			}
			if got.status != wantStatus || got.allow != want {
				t.Fatalf("%s: %s %s = %d Allow %q, want %d Allow %q\nroutes %v", tc.name, method, path, got.status, got.allow, wantStatus, want, routes)
			}
		}
	}
}

func TestRouterRulesRandomTables(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 2026))
	for i := 0; i < 400; i++ {
		checkRouterRules(t, rng)
	}
}

// FuzzRouterRules checks rules 1 and 2 over route tables drawn from the
// input.
//
//	go test -run '^$' -fuzz '^FuzzRouterRules$' -fuzztime 60s .
func FuzzRouterRules(f *testing.F) {
	for _, seed := range []uint64{0, 1, 42, 2026, 0xdecafbad} {
		f.Add(seed, seed^0x9e3779b97f4a7c15)
	}
	f.Fuzz(func(t *testing.T, a, b uint64) {
		checkRouterRules(t, rand.New(rand.NewPCG(a, b)))
	})
}

func TestRuleNilAppMiddlewareRejected(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "nil middleware") {
			t.Fatalf("App.Use(nil): %v", r)
		}
	}()
	New().Use(nil)
}
