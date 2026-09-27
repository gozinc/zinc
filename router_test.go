// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRouterDynamicRoutesAndHelpers(t *testing.T) {
	app := New()
	app.Get("/files/{path...}", func(c *Context) error {
		return c.String(c.Param("path"))
	})
	app.All("/any", func(c *Context) error {
		return c.String(c.Method())
	})

	wild := performRequest(t, app, http.MethodGet, "/files/a/b/c.txt", nil, nil)
	if wild.Body.String() != "a/b/c.txt" {
		t.Fatalf("wildcard=%q", wild.Body.String())
	}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace} {
		resp := performRequest(t, app, method, "/any", nil, nil)
		if method == http.MethodHead {
			if resp.Code != http.StatusOK {
				t.Fatalf("status=%d", resp.Code)
			}
			continue
		}
		if resp.Body.String() != method {
			t.Fatalf("method=%s body=%q", method, resp.Body.String())
		}
	}
}

func TestRouterCaseInsensitiveDynamicRoutes(t *testing.T) {
	app := New()
	app.Post("/Reports/{year}/Files/{path...}", func(c *Context) error {
		return c.String(c.Param("year") + "|" + c.Param("path"))
	})

	matched := performRequest(t, app, http.MethodPost, "/reports/2026/files/Q3/Summary.csv", nil, nil)
	if matched.Code != http.StatusOK || matched.Body.String() != "2026|Q3/Summary.csv" {
		t.Fatalf("matched=%d %q", matched.Code, matched.Body.String())
	}

	mismatch := performRequest(t, app, http.MethodGet, "/REPORTS/2026/FILES/Q3/Summary.csv", nil, nil)
	if mismatch.Code != http.StatusMethodNotAllowed {
		t.Fatalf("mismatch status=%d", mismatch.Code)
	}
	if allow := mismatch.Header().Get(HeaderAllow); allow != "POST, OPTIONS" {
		t.Fatalf("allow=%q", allow)
	}

	router := &routeTable{config: &Config{}}
	handler := func(*Context) error { return nil }
	mustDo(t, router.Add(MethodGet, "/Users/{id}", handler))
	if err := router.Add(MethodGet, "/users/{name}", handler); err == nil {
		t.Fatal("expected case-insensitive dynamic route conflict")
	}
}

func TestRouterCatchAllMatchesEmptyRemainder(t *testing.T) {
	app := New()
	app.Get("/files/{path...}", func(c *Context) error {
		return c.String("catch:" + c.Param("path"))
	})

	empty := performRequest(t, app, http.MethodGet, "/files/", nil, nil)
	if empty.Code != http.StatusOK || empty.Body.String() != "catch:" {
		t.Fatalf("empty catch-all=%d %q", empty.Code, empty.Body.String())
	}

	app.Get("/exact/", func(c *Context) error { return c.String("exact") })
	app.Get("/exact/{path...}", func(c *Context) error { return c.String("catch") })
	exact := performRequest(t, app, http.MethodGet, "/exact/", nil, nil)
	if exact.Code != http.StatusOK || exact.Body.String() != "exact" {
		t.Fatalf("exact precedence=%d %q", exact.Code, exact.Body.String())
	}

	native := New()
	native.HandleHTTP("GET /native/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "path:%s", r.PathValue("path"))
	}))
	nativeEmpty := performRequest(t, native, http.MethodGet, "/native/", nil, nil)
	if nativeEmpty.Code != http.StatusOK || nativeEmpty.Body.String() != "path:" {
		t.Fatalf("native empty catch-all=%d %q", nativeEmpty.Code, nativeEmpty.Body.String())
	}
}

func TestRouterBraceParamsAndWrappedRequestPathValues(t *testing.T) {
	app := New()
	app.Get("/users/{userID}", func(c *Context) error {
		if got := c.Request().PathValue("userID"); got != "" {
			t.Fatalf("native path value populated for Zinc handler: %q", got)
		}
		return c.String(c.Param("userID"))
	})
	app.Get("/files/{path...}", func(c *Context) error {
		if got := c.Request().PathValue("path"); got != "" {
			t.Fatalf("native wildcard path value populated for Zinc handler: %q", got)
		}
		return c.String(c.Param("path"))
	})
	app.Get("/native/{id}/files/{path...}", Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s|%s", r.PathValue("id"), r.PathValue("path"))
	})))

	user := performRequest(t, app, http.MethodGet, "/users/42", nil, nil)
	if got := user.Body.String(); got != "42" {
		t.Fatalf("user id=%q", got)
	}

	file := performRequest(t, app, http.MethodGet, "/files/assets/app.css", nil, nil)
	if got := file.Body.String(); got != "assets/app.css" {
		t.Fatalf("file path=%q", got)
	}

	native := performRequest(t, app, http.MethodGet, "/native/84/files/assets/app.css", nil, nil)
	if got := native.Body.String(); got != "84|assets/app.css" {
		t.Fatalf("native path value=%q", got)
	}
}

func TestBraceRoutePatternValidationAndMetadata(t *testing.T) {
	router := &routeTable{config: &Config{}}
	handler := func(*Context) error { return nil }

	mustDo(t, router.AddNamed(MethodGet, "/teams/{teamID}/users/{userID}", "users.show", handler))
	routes := router.Routes()
	if len(routes) != 1 {
		t.Fatalf("routes=%d", len(routes))
	}
	if routes[0].Path != "/teams/{teamID}/users/{userID}" {
		t.Fatalf("path=%q", routes[0].Path)
	}
	if !reflect.DeepEqual(routes[0].Params, []string{"teamID", "userID"}) {
		t.Fatalf("params=%v", routes[0].Params)
	}

	meta, ok := router.routeMetaByName("users.show")
	if !ok {
		t.Fatal("named route missing")
	}
	url, err := meta.url([]string{"platform", "matt smith"})
	mustDo(t, err)
	if url != "/teams/platform/users/matt%20smith" {
		t.Fatalf("url=%q", url)
	}

	invalid := []string{
		"/users/{id",
		"/users/id}",
		"/users/prefix-{id}",
		"/users/{9id}",
		"/users/{id}/{id}",
		"/files/{path...}/meta",
	}
	for _, pattern := range invalid {
		if err := router.Add(MethodPost, pattern, handler); err == nil {
			t.Fatalf("expected %q to be rejected", pattern)
		}
	}
}

func TestRouterConflictsAndNormalization(t *testing.T) {
	router := &routeTable{config: &Config{}}
	mustDo(t, router.Add(MethodGet, "users/{id}", func(c *Context) error { return nil }))
	if err := router.Add(MethodGet, "/users/{id}", func(c *Context) error { return nil }); err == nil {
		t.Fatal("expected duplicate route error")
	}
	if got := router.normalizePath("users"); got != "/users" {
		t.Fatalf("normalized=%q", got)
	}
}

func TestRouterRejectsLegacyRoutePatterns(t *testing.T) {
	router := &routeTable{config: &Config{}}
	handler := func(*Context) error { return nil }
	patterns := []string{
		"/users/:id",
		"/users/:",
		"/users/prefix:id",
		"/files/*path",
		"/files/prefix*path",
		"/users/:id<\\d+>",
		// Reserved on purpose: 0.3 read this as a parameter named batch.
		"/v1/users:batch",
	}
	for _, pattern := range patterns {
		if err := router.Add(MethodGet, pattern, handler); err == nil || !strings.Contains(err.Error(), "legacy route") {
			t.Fatalf("pattern=%q err=%v", pattern, err)
		}
	}
}

func TestGroupHelpers(t *testing.T) {
	app := New()
	api := app.Group("/api", func(c *Context) error {
		c.Set("group", true)
		return c.Next()
	})
	v1 := api.Route("/v1", nil)
	v1.Get("/ping", func(c *Context) error {
		group, _ := c.Get("group")
		if group != true {
			t.Fatal("group middleware missing")
		}
		return c.String("pong")
	})

	resp := performRequest(t, app, http.MethodGet, "/api/v1/ping", nil, nil)
	if resp.Body.String() != "pong" {
		t.Fatalf("body=%q", resp.Body.String())
	}
}

func TestLowercasePathAndHandlerNameBranches(t *testing.T) {
	if got, changed := lowercasePath("/abc"); got != "/abc" || changed {
		t.Fatalf("got=%q changed=%v", got, changed)
	}
	if got, changed := lowercasePath("/ABC"); got != "/abc" || !changed {
		t.Fatalf("got=%q changed=%v", got, changed)
	}
	if got, changed := lowercasePath("/ÄBC"); got != "/äbc" || !changed {
		t.Fatalf("got=%q changed=%v", got, changed)
	}

	if name := handlerNameFromPC(handlerPC(nil)); name != "" {
		t.Fatalf("name=%q", name)
	}
	handler := HandlerFunc(func(*Context) error { return nil })
	name := handlerNameFromPC(handlerPC(handler))
	if name == "" {
		t.Fatal("expected handler name")
	}
	if again := handlerNameFromPC(handlerPC(handler)); again != name {
		t.Fatalf("cached name=%q first=%q", again, name)
	}
}

func TestRadixNodeBranches(t *testing.T) {
	router := &routeTable{config: &Config{CaseSensitive: true}}
	mustDo(t, router.Add(MethodGet, "/foo", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodGet, "/fob", func(*Context) error { return nil })) // triggers static-node split branch
	if err := router.Add(MethodGet, "/foo", func(*Context) error { return nil }); err == nil {
		t.Fatal("expected duplicate route error")
	}

	paramHolder := &radixNode{}
	if first, second := paramHolder.addParamChild(nil), paramHolder.addParamChild(nil); first != second {
		t.Fatal("param child should be reused")
	}
	catchAllHolder := &radixNode{}
	if first, second := catchAllHolder.addCatchAllChild(nil), catchAllHolder.addCatchAllChild(nil); first != second {
		t.Fatal("catch-all child should be reused")
	}

	slot := singleBitIndex(methodMaskFor(MethodGet))
	static := &radixNode{kind: radixStatic, prefix: "abc"}
	if matched := static.lookupMethod("ab", 0, &paramRanges{}, 0, false, slot, MethodGet); matched != nil {
		t.Fatalf("matched=%v", matched)
	}

	param := &radixNode{kind: radixParam}
	if matched := param.lookupMethod("/x", 0, &paramRanges{}, 0, false, slot, MethodGet); matched != nil {
		t.Fatalf("matched=%v", matched)
	}

	mismatch := &radixNode{methods: &nodeMethods{}}
	mismatch.methods.set(slot, MethodGet, &radixRoute{paramCount: 1})
	if matched := mismatch.methodRoute(slot, MethodGet, 0); matched != nil {
		t.Fatalf("matched=%v", matched)
	}
}

func TestRouterFindIntoBranches(t *testing.T) {
	router := &routeTable{config: &Config{}}
	mustDo(t, router.Add(MethodGet, "/Case", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodGet, "/trim", func(*Context) error { return nil }))

	ctxCase := &Context{}
	if handler := router.findInto(MethodGet, "/CASE", ctxCase); handler == nil {
		t.Fatal("expected case-insensitive match")
	}
	if ctxCase.FullPath() != "/Case" {
		t.Fatalf("full path=%q", ctxCase.FullPath())
	}

	ctxTrim := &Context{}
	if handler := router.findInto(MethodGet, "/trim/", ctxTrim); handler == nil {
		t.Fatal("expected non-strict trailing slash match")
	}

	empty := &routeTable{}
	if handler := empty.findInto(MethodGet, "/missing", &Context{}); handler != nil {
		t.Fatalf("handler=%v", handler)
	}

}

func TestRouterAddAndMatchErrorBranches(t *testing.T) {
	router := &routeTable{config: &Config{}}
	if err := router.Add(MethodGet, "/users"); err == nil || !strings.Contains(err.Error(), "no handler provided") {
		t.Fatalf("err=%v", err)
	}

	app := New()
	mustPanic(t, "no handler provided", func() {
		app.Match([]string{MethodGet}, "/users")
	})
}

func TestRouterAllowedMethodsSharedPathIndex(t *testing.T) {
	router := &routeTable{config: &Config{}}
	mustDo(t, router.Add(MethodGet, "/shared/one", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodPost, "/shared/two", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodGet, "/users/{id}", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodPost, "/users/me", func(*Context) error { return nil }))

	shared := strings.Split(allowedHeaderForTest(router, "/shared/one"), ", ")
	if want := []string{MethodGet, MethodHead, MethodOptions}; !reflect.DeepEqual(shared, want) {
		t.Fatalf("shared allow = %v want %v", shared, want)
	}

	overlap := strings.Split(allowedHeaderForTest(router, "/users/me"), ", ")
	if want := []string{MethodGet, MethodHead, MethodPost, MethodOptions}; !reflect.DeepEqual(overlap, want) {
		t.Fatalf("overlap allow = %v want %v", overlap, want)
	}

	if header := allowedHeaderForTest(router, "/users/me"); header != "GET, HEAD, POST, OPTIONS" {
		t.Fatalf("allow header = %q", header)
	}
}

// A 405 must not outlive the registration that fixes it.
func TestRouterDispatchIntoMissThenAdd(t *testing.T) {
	router := &routeTable{config: &Config{}}
	mustDo(t, router.Add(MethodPost, "/only-post", func(*Context) error { return nil }))

	handled, allowed, err := router.dispatchInto(MethodPut, "/only-post", true, &Context{})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if handled {
		t.Fatal("expected method mismatch to miss the handler")
	}
	if allowed.mask != methodMaskPost || len(allowed.extra) != 0 {
		t.Fatalf("allowed=%v", allowed)
	}

	mustDo(t, router.Add(MethodPut, "/only-post", func(*Context) error { return nil }))

	handled, allowed, err = router.dispatchInto(MethodPut, "/only-post", true, &Context{})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !handled {
		t.Fatal("expected registered method to handle the request")
	}
	if !allowed.empty() {
		t.Fatalf("allowed=%v", allowed)
	}
}

func TestRouterSupportsMoreThanInlinePathParams(t *testing.T) {
	pattern, path, names, _ := buildSequentialParamRoute(10)

	app := New()
	app.Get(pattern, func(c *Context) error {
		return c.String(c.Param(names[len(names)-1]))
	})

	resp := performRequest(t, app, http.MethodGet, path, nil, nil)
	if resp.Body.String() != "10" {
		t.Fatalf("body=%q", resp.Body.String())
	}
}

func TestRouterDispatchIntoManyParams(t *testing.T) {
	router := &routeTable{config: &Config{}}

	pattern, path, names, _ := buildSequentialParamRoute(10)
	mustDo(t, router.Add(MethodGet, pattern, func(*Context) error { return nil }))

	ctx := &Context{}
	handled, allowed, err := router.dispatchInto(MethodGet, path, false, ctx)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !handled {
		t.Fatal("expected dynamic route to handle the request")
	}
	if !allowed.empty() {
		t.Fatalf("allowed=%v", allowed)
	}
	if got := ctx.Param(names[8]); got != "9" {
		t.Fatalf("param9=%q", got)
	}
	if got := ctx.Param(names[9]); got != "10" {
		t.Fatalf("param10=%q", got)
	}
	if len(ctx.pathParams) < len(names) || ctx.pathParams[9].key != names[9] {
		t.Fatalf("path params not expanded: len=%d last=%+v", len(ctx.pathParams), ctx.pathParams[9])
	}

	// A second dispatch reuses the route's parameter names.
	ctxCached := &Context{}
	handled, allowed, err = router.dispatchInto(MethodGet, path, false, ctxCached)
	if err != nil {
		t.Fatalf("again err=%v", err)
	}
	if !handled {
		t.Fatal("expected cached dispatch to handle the request")
	}
	if !allowed.empty() {
		t.Fatalf("cached allowed=%v", allowed)
	}
	if got := ctxCached.Param(names[9]); got != "10" {
		t.Fatalf("again param10=%q", got)
	}
}

// A static route added over a parameter route wins from then on.
func TestRouterDispatchIntoStaticAddedOverParam(t *testing.T) {
	router := &routeTable{config: &Config{}}
	mustDo(t, router.Add(MethodGet, "/items/{id}", func(*Context) error { return nil }))

	ctxParam := &Context{}
	handled, allowed, err := router.dispatchInto(MethodGet, "/items/new", false, ctxParam)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !handled {
		t.Fatal("expected param route to handle the request")
	}
	if !allowed.empty() {
		t.Fatalf("allowed=%v", allowed)
	}
	if ctxParam.routeIndex != 0 {
		t.Fatalf("route index=%d", ctxParam.routeIndex)
	}

	mustDo(t, router.Add(MethodGet, "/items/new", func(*Context) error { return nil }))

	ctxStatic := &Context{}
	handled, allowed, err = router.dispatchInto(MethodGet, "/items/new", false, ctxStatic)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !handled {
		t.Fatal("expected static route to handle the request")
	}
	if !allowed.empty() {
		t.Fatalf("allowed=%v", allowed)
	}
	if ctxStatic.routeIndex != 1 {
		t.Fatalf("route index=%d", ctxStatic.routeIndex)
	}
}

func TestRouterStaticRouteLengthFilter(t *testing.T) {
	router := &routeTable{config: &Config{}}
	shortPath := "/fixed"
	longPath := "/" + strings.Repeat("x", 140)
	mustDo(t, router.Add(MethodGet, shortPath, func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodGet, longPath, func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodGet, "/items/{id}", func(*Context) error { return nil }))

	slot := singleBitIndex(methodMaskGet)
	if !router.hasStaticRouteLength(slot, methodMaskGet, len(shortPath)) {
		t.Fatal("expected short static path length to pass the filter")
	}
	if !router.hasStaticRouteLength(slot, methodMaskGet, len(longPath)) {
		t.Fatal("expected long static path length to pass the filter")
	}
	if router.hasStaticRouteLength(slot, methodMaskGet, len("/items/42")) {
		t.Fatal("expected unmatched dynamic path length to skip static lookup")
	}

	for _, path := range []string{shortPath, longPath, "/items/42"} {
		handled, _, err := router.dispatchInto(MethodGet, path, false, &Context{})
		if err != nil || !handled {
			t.Fatalf("dispatch %q handled=%v err=%v", path, handled, err)
		}
	}
}

// Two requests for the same route must not share parameter values.
func TestRouterDispatchIntoManyParamsIsolation(t *testing.T) {
	router := &routeTable{config: &Config{}}

	pattern, pathShort, names, _ := buildSequentialParamRoute(10)
	var pathLong strings.Builder
	for i := 0; i < 10; i++ {
		pathLong.WriteByte('/')
		pathLong.WriteString(fmt.Sprintf("%d", 1000+i))
	}

	mustDo(t, router.Add(MethodGet, pattern, func(*Context) error { return nil }))

	shortCtx := &Context{}
	handled, _, err := router.dispatchInto(MethodGet, pathShort, false, shortCtx)
	if err != nil {
		t.Fatalf("short err=%v", err)
	}
	if !handled {
		t.Fatal("expected short path to match")
	}
	if got := shortCtx.Param(names[9]); got != "10" {
		t.Fatalf("short param10=%q", got)
	}

	longCtx := &Context{}
	handled, _, err = router.dispatchInto(MethodGet, pathLong.String(), false, longCtx)
	if err != nil {
		t.Fatalf("long err=%v", err)
	}
	if !handled {
		t.Fatal("expected long path to match")
	}
	if got := longCtx.Param(names[9]); got != "1009" {
		t.Fatalf("long param10=%q", got)
	}

	shortAgainCtx := &Context{}
	handled, _, err = router.dispatchInto(MethodGet, pathShort, false, shortAgainCtx)
	if err != nil {
		t.Fatalf("again err=%v", err)
	}
	if !handled {
		t.Fatal("expected short path to match again")
	}
	if got := shortAgainCtx.Param(names[8]); got != "9" {
		t.Fatalf("again param9=%q", got)
	}
	if got := shortAgainCtx.Param(names[9]); got != "10" {
		t.Fatalf("cached param10=%q", got)
	}
}

func TestRouterSupportsCustomDynamicMethods(t *testing.T) {
	const methodPurge = "PURGE"

	router := &routeTable{config: &Config{}}
	mustDo(t, router.Add(methodPurge, "/items/{id}", func(*Context) error { return nil }))

	ctx := &Context{}
	handled, allowed, err := router.dispatchInto(methodPurge, "/items/42", false, ctx)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !handled {
		t.Fatal("expected custom dynamic method to match")
	}
	if !allowed.empty() {
		t.Fatalf("allowed=%v", allowed)
	}
	if got := ctx.Param("id"); got != "42" {
		t.Fatalf("id=%q", got)
	}

	handled, allowed, err = router.dispatchInto(MethodGet, "/items/42", true, &Context{})
	if err != nil {
		t.Fatalf("method mismatch err=%v", err)
	}
	if handled {
		t.Fatal("expected GET to miss the PURGE route")
	}
	if allowed.mask != 0 {
		t.Fatalf("mask=%v", allowed.mask)
	}
	if !reflect.DeepEqual(allowed.extra, []string{methodPurge}) {
		t.Fatalf("extra=%v", allowed.extra)
	}
	if header := allowed.header(true, true); header != "OPTIONS, PURGE" {
		t.Fatalf("allow header=%q", header)
	}
}

func TestRouterAllowedMethodsCaseInsensitiveStaticIndex(t *testing.T) {
	for _, caseSensitive := range []bool{false, true} {
		router := &routeTable{config: &Config{CaseSensitive: caseSensitive}}
		mustDo(t, router.Add(MethodPost, "/case", func(*Context) error { return nil }))
		handled, allowed, err := router.dispatchInto(MethodPut, "/CASE", true, &Context{})
		if err != nil || handled {
			t.Fatalf("caseSensitive=%v: handled=%v err=%v", caseSensitive, handled, err)
		}
		want := "POST, OPTIONS"
		if caseSensitive {
			want = ""
		}
		if got := allowed.header(true, true); got != want {
			t.Fatalf("caseSensitive=%v: allow=%q want %q", caseSensitive, got, want)
		}
	}
}

func buildSequentialParamRoute(count int) (string, string, []string, paramRanges) {
	var (
		pattern strings.Builder
		path    strings.Builder
		names   = make([]string, count)
		ranges  paramRanges
	)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("p%d", i+1)
		value := fmt.Sprintf("%d", i+1)
		names[i] = name

		pattern.WriteByte('/')
		pattern.WriteByte('{')
		pattern.WriteString(name)
		pattern.WriteByte('}')

		path.WriteByte('/')
		start := path.Len()
		path.WriteString(value)
		ranges.set(i, paramRange{
			start: uint32(start),
			end:   uint32(path.Len()),
		})
	}
	return pattern.String(), path.String(), names, ranges
}

// Exercise the same method-negotiation path used by ServeHTTP.
func allowedHeaderForTest(r *routeTable, path string) string {
	_, allowed, _ := r.dispatchInto("UNREGISTERED", path, true, &Context{})
	return allowed.header(true, true)
}

// Allow must list every method a static path answers, including routes with a
// single accepted spelling next to routes with several (GET /v1/ also answers
// /v1). v0.4.0 dropped DELETE here; FuzzRouterReference found it.
func TestAllowListsEveryStaticMethod(t *testing.T) {
	app := New()
	app.Get("/v1/", func(c *Context) error { return c.String("get") })
	app.Delete("/v1", func(c *Context) error { return c.String("delete") })
	for _, method := range []string{MethodOptions, MethodPut} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(method, "/v1", nil))
		if got, want := rec.Header().Get(HeaderAllow), "GET, HEAD, DELETE, OPTIONS"; got != want {
			t.Errorf("%s /v1: Allow = %q, want %q", method, got, want)
		}
	}
}

// Custom methods are listed in Allow in sorted order, after the standard
// methods (deliberate change 3 in benchmarks/ROUTER_SPEC.md).
func TestAllowSortsCustomMethods(t *testing.T) {
	app := New()
	for _, method := range []string{"PURGE", "LINK", "GET", "BAN"} {
		app.Add(method, "/items/{id}", func(c *Context) error { return nil })
		app.Add(method, "/static", func(c *Context) error { return nil })
	}
	for _, target := range []string{"/items/1", "/static"} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(MethodPut, target, nil))
		if got, want := rec.Header().Get(HeaderAllow), "GET, HEAD, OPTIONS, BAN, LINK, PURGE"; got != want {
			t.Errorf("%s: Allow = %q, want %q", target, got, want)
		}
	}
}
