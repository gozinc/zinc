// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRouterDynamicRoutesAndHelpers(t *testing.T) {
	app := New()
	app.Get("/files/{path...}", func(c *Context) error {
		return c.String(c.Param("path"))
	})
	app.Any("/any", func(c *Context) error {
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

	router := &Router{config: &DefaultConfig}
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
	router := &Router{config: &DefaultConfig}
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
	router := &Router{config: &DefaultConfig}
	mustDo(t, router.Add(MethodGet, "users/{id}", func(c *Context) error { return nil }))
	if err := router.Add(MethodGet, "/users/{id}", func(c *Context) error { return nil }); err == nil {
		t.Fatal("expected duplicate route error")
	}
	if got := router.normalizePath("users"); got != "/users" {
		t.Fatalf("normalized=%q", got)
	}
}

func TestRouterRejectsLegacyRoutePatterns(t *testing.T) {
	router := &Router{config: &DefaultConfig}
	handler := func(*Context) error { return nil }
	patterns := []string{
		"/users/:id",
		"/users/:",
		"/users/prefix:id",
		"/files/*path",
		"/files/prefix*path",
		"/users/:id<\\d+>",
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

	if name := handlerName(nil); name != "" {
		t.Fatalf("name=%q", name)
	}
	handler := HandlerFunc(func(*Context) error { return nil })
	name := handlerName(handler)
	if name == "" {
		t.Fatal("expected handler name")
	}
	if again := handlerName(handler); again != name {
		t.Fatalf("cached name=%q first=%q", again, name)
	}
}

func TestRouteCacheSetUpdateAndEviction(t *testing.T) {
	cache := NewRouteCache(1)

	key1 := routeCacheKey{method: MethodGet, path: "/one"}
	key2 := routeCacheKey{method: MethodGet, path: "/two"}
	route1 := &radixRoute{infoIndex: 1}
	route1Updated := &radixRoute{infoIndex: 2}
	route2 := &radixRoute{infoIndex: 3}
	entry1 := routeCacheEntry{route: route1}
	entry1Updated := routeCacheEntry{route: route1Updated}
	entry2 := routeCacheEntry{route: route2}

	cache.set(key1, entry1)
	cache.set(key1, entry1Updated) // update existing key branch
	if got, ok := cache.get(key1); !ok || got.route != route1Updated {
		t.Fatalf("updated entry=%v ok=%v", got, ok)
	}

	cache.set(key2, entry2) // eviction branch for size=1
	if _, ok := cache.get(key1); ok {
		t.Fatal("expected key1 to be evicted")
	}
	if got, ok := cache.get(key2); !ok || got.route != route2 {
		t.Fatalf("entry2=%v ok=%v", got, ok)
	}
}

func TestRouteCachePartitionsStandardAndCustomMethods(t *testing.T) {
	cache := NewRouteCache(3)
	keys := []routeCacheKey{
		{method: MethodGet, path: "/shared"},
		{method: MethodPost, path: "/shared"},
		{method: "PURGE", path: "/shared"},
	}
	for i, key := range keys {
		cache.set(key, routeCacheEntry{route: &radixRoute{infoIndex: uint32(i + 1)}})
	}

	for i, key := range keys {
		entry, ok := cache.get(key)
		if !ok || entry.route == nil || entry.route.infoIndex != uint32(i+1) {
			t.Fatalf("key=%+v entry=%+v ok=%v", key, entry, ok)
		}
	}
}

func TestRouteCachePromotesStablePartialCacheAndKeepsOverlayAdaptive(t *testing.T) {
	cache := NewRouteCache(100)
	keys := make([]routeCacheKey, routeCacheMinRoutes)
	for i := range keys {
		keys[i] = routeCacheKey{method: MethodGet, path: fmt.Sprintf("/stable/%d", i)}
		cache.set(keys[i], routeCacheEntry{route: &radixRoute{infoIndex: uint32(i + 1)}})
	}
	for cycle := 0; cycle < routeCacheFreezeHitCycles+1; cycle++ {
		for _, key := range keys {
			if _, ok := cache.get(key); !ok {
				t.Fatalf("stable key missing before promotion: %+v", key)
			}
		}
	}
	if cache.snapshot.Load() == nil {
		t.Fatal("expected stable partial cache to promote a read snapshot")
	}

	overlayKey := routeCacheKey{method: MethodPost, path: "/after-promotion"}
	overlayEntry := routeCacheEntry{route: &radixRoute{infoIndex: 1000}}
	cache.set(overlayKey, overlayEntry)
	if got, ok := cache.get(overlayKey); !ok || got.route != overlayEntry.route {
		t.Fatalf("overlay entry=%+v ok=%v", got, ok)
	}

	cache.invalidate()
	if _, ok := cache.get(keys[0]); ok {
		t.Fatal("expected invalidation to clear the promoted snapshot")
	}
}

func TestRouteCacheRepromotesStableOverlay(t *testing.T) {
	cache := NewRouteCache(256)
	phaseA := make([]routeCacheKey, routeCacheMinRoutes)
	phaseB := make([]routeCacheKey, routeCacheMinRoutes)
	for i := range phaseA {
		phaseA[i] = routeCacheKey{method: MethodGet, path: fmt.Sprintf("/phase-a/%d", i)}
		phaseB[i] = routeCacheKey{method: MethodGet, path: fmt.Sprintf("/phase-b/%d", i)}
		cache.set(phaseA[i], routeCacheEntry{route: &radixRoute{infoIndex: uint32(i + 1)}})
	}
	for cycle := 0; cycle < routeCacheFreezeHitCycles+1; cycle++ {
		for _, key := range phaseA {
			cache.get(key)
		}
	}
	firstSnapshot := cache.snapshot.Load()
	if firstSnapshot == nil {
		t.Fatal("expected phase A to promote")
	}

	for i, key := range phaseB {
		cache.set(key, routeCacheEntry{route: &radixRoute{infoIndex: uint32(i + 101)}})
	}
	for cycle := 0; cycle < routeCacheFreezeHitCycles+1; cycle++ {
		for _, key := range phaseB {
			if _, ok := cache.get(key); !ok {
				t.Fatalf("phase B key missing before re-promotion: %+v", key)
			}
		}
	}

	if snapshot := cache.snapshot.Load(); snapshot == nil || snapshot == firstSnapshot {
		t.Fatal("expected the stable overlay to replace the original snapshot")
	}
	if got := atomic.LoadUint32(&cache.count); got != routeCacheMinRoutes {
		t.Fatalf("cache count=%d, want %d after re-promotion", got, routeCacheMinRoutes)
	}
	if _, ok := cache.get(phaseA[0]); ok {
		t.Fatal("expected the old phase to be dropped from the cache")
	}
	if _, ok := cache.get(phaseB[0]); !ok {
		t.Fatal("expected the new phase in the replacement snapshot")
	}
}

var benchmarkRouteCacheSink *RouteCache

func BenchmarkRouteCachePromotionAllocation(b *testing.B) {
	keys := make([]routeCacheKey, routeCacheMinRoutes)
	entries := make([]routeCacheEntry, routeCacheMinRoutes)
	for i := range keys {
		keys[i] = routeCacheKey{method: MethodGet, path: fmt.Sprintf("/stable/%d", i)}
		entries[i] = routeCacheEntry{route: &radixRoute{infoIndex: uint32(i + 1)}}
	}
	prepare := func(promote bool) *RouteCache {
		cache := NewRouteCache(100)
		for i := range keys {
			cache.set(keys[i], entries[i])
		}
		for hit := 1; hit < routeCacheMinRoutes*routeCacheFreezeHitCycles; hit++ {
			cache.get(keys[0])
		}
		if promote {
			cache.get(keys[0])
		}
		return cache
	}

	b.Run("BeforePromotion", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRouteCacheSink = prepare(false)
		}
	})
	b.Run("Promote", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRouteCacheSink = prepare(true)
		}
	})
}

func TestRouteCacheDoesNotFreezeAtCapacity(t *testing.T) {
	cache := NewRouteCache(routeCacheMinRoutes)
	keys := make([]routeCacheKey, routeCacheMinRoutes)
	for i := range keys {
		keys[i] = routeCacheKey{method: MethodGet, path: fmt.Sprintf("/full/%d", i)}
		cache.set(keys[i], routeCacheEntry{route: &radixRoute{infoIndex: uint32(i + 1)}})
	}
	for cycle := 0; cycle < routeCacheFreezeHitCycles+2; cycle++ {
		for _, key := range keys {
			if _, ok := cache.get(key); !ok {
				t.Fatalf("full-cache key missing: %+v", key)
			}
		}
	}
	if cache.snapshot.Load() != nil {
		t.Fatal("full cache must remain adaptive")
	}
}

func TestRouteCacheConcurrentSnapshotPromotion(t *testing.T) {
	cache := NewRouteCache(100)
	keys := make([]routeCacheKey, routeCacheMinRoutes)
	for i := range keys {
		keys[i] = routeCacheKey{method: MethodGet, path: fmt.Sprintf("/concurrent/%d", i)}
		cache.set(keys[i], routeCacheEntry{route: &radixRoute{infoIndex: uint32(i + 1)}})
	}

	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(offset int) {
			defer workers.Done()
			for i := 0; i < len(keys)*4; i++ {
				key := keys[(i+offset)%len(keys)]
				if _, ok := cache.get(key); !ok {
					t.Errorf("stable key missing during promotion: %+v", key)
					return
				}
			}
		}(worker)
	}
	workers.Wait()

	if cache.snapshot.Load() == nil {
		t.Fatal("expected concurrent hits to promote a read snapshot")
	}
}

func TestRouteCacheInvalidateClearsLazilyOnNextAccess(t *testing.T) {
	cache := NewRouteCache(2)
	key := routeCacheKey{method: MethodGet, path: "/one"}
	entry := routeCacheEntry{route: &radixRoute{infoIndex: 1}}

	cache.set(key, entry)
	cache.invalidate()

	if _, ok := cache.get(key); ok {
		t.Fatal("expected first cache access after invalidation to clear stale entries")
	}

	cache.set(key, entry)
	if got, ok := cache.get(key); !ok || got.route == nil || got.route.infoIndex != 1 {
		t.Fatalf("entry=%+v ok=%v", got, ok)
	}
}

func TestRouteCacheMissAdmissionProtectsFullCache(t *testing.T) {
	cache := NewRouteCache(1)
	hotKey := routeCacheKey{method: MethodGet, path: "/hot"}
	coldKey := routeCacheKey{method: MethodGet, path: "/cold"}
	hotEntry := routeCacheEntry{route: &radixRoute{infoIndex: 1}}
	coldEntry := routeCacheEntry{route: &radixRoute{infoIndex: 2}}

	cache.setMiss(hotKey, hotEntry)
	for i := 1; i < routeCacheAdmissionInterval; i++ {
		cache.setMiss(coldKey, coldEntry)
		if got, ok := cache.get(hotKey); !ok || got.route != hotEntry.route {
			t.Fatalf("attempt %d evicted protected hot entry: got=%+v ok=%v", i, got, ok)
		}
	}

	cache.setMiss(coldKey, coldEntry)
	if _, ok := cache.get(hotKey); ok {
		t.Fatal("expected admitted cold entry to evict hot entry")
	}
	if got, ok := cache.get(coldKey); !ok || got.route != coldEntry.route {
		t.Fatalf("admitted entry=%+v ok=%v", got, ok)
	}
}

func TestRouteCacheRingEvictionWrapsWithoutStaleHotEntry(t *testing.T) {
	cache := NewRouteCache(2)
	keys := []routeCacheKey{
		{method: MethodGet, path: "/one"},
		{method: MethodGet, path: "/two"},
		{method: MethodGet, path: "/three"},
		{method: MethodGet, path: "/four"},
	}
	entries := []routeCacheEntry{
		{route: &radixRoute{infoIndex: 1}},
		{route: &radixRoute{infoIndex: 2}},
		{route: &radixRoute{infoIndex: 3}},
		{route: &radixRoute{infoIndex: 4}},
	}

	cache.set(keys[0], entries[0])
	cache.set(keys[1], entries[1])
	cache.set(keys[2], entries[2])
	if _, ok := cache.get(keys[0]); ok {
		t.Fatal("expected first ring entry to be evicted")
	}
	if got, ok := cache.get(keys[1]); !ok || got.route != entries[1].route {
		t.Fatalf("second entry=%+v ok=%v", got, ok)
	}

	cache.set(keys[3], entries[3])
	if _, ok := cache.get(keys[1]); ok {
		t.Fatal("expected hot entry to be evicted after ring wrap")
	}
	if _, ok := cache.getHot(keys[1]); ok {
		t.Fatal("expected evicted hot entry to be cleared")
	}
	for i := 2; i < len(keys); i++ {
		if got, ok := cache.get(keys[i]); !ok || got.route != entries[i].route {
			t.Fatalf("entry %d=%+v ok=%v", i, got, ok)
		}
	}
}

func TestRadixNodeBranches(t *testing.T) {
	root := &radixNode{kind: radixRoot}
	mustDo(t, root.addBrace("/foo", &radixRoute{}, false))
	mustDo(t, root.addBrace("/fob", &radixRoute{}, false)) // triggers static-node split branch

	if err := root.addBrace("/foo", &radixRoute{}, false); err == nil {
		t.Fatal("expected duplicate route error")
	}

	paramHolder := &radixNode{}
	if first, second := paramHolder.addParamChild(), paramHolder.addParamChild(); first != second {
		t.Fatal("param child should be reused")
	}
	catchAllHolder := &radixNode{}
	if first, second := catchAllHolder.addCatchAllChild(), catchAllHolder.addCatchAllChild(); first != second {
		t.Fatal("catch-all child should be reused")
	}

	static := &radixNode{kind: radixStatic, prefix: "abc"}
	if matched := static.lookup("ab", 0, &paramRanges{}, 0); matched != nil {
		t.Fatalf("matched=%v", matched)
	}

	param := &radixNode{kind: radixParam}
	if matched := param.lookup("/x", 0, &paramRanges{}, 0); matched != nil {
		t.Fatalf("matched=%v", matched)
	}

	mismatch := &radixNode{route: &radixRoute{paramCount: 1}}
	if matched := mismatch.matchRoute(0); matched != nil {
		t.Fatalf("matched=%v", matched)
	}
}

func TestRouterFindIntoAndDynamicCacheBranches(t *testing.T) {
	router := &Router{
		cache:  NewRouteCache(2),
		config: &DefaultConfig,
	}
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

	dynamic := &Router{
		cache: NewRouteCache(2),
	}
	cacheKey := routeCacheKey{method: MethodGet, path: "/cached"}
	dynamic.cache.set(cacheKey, routeCacheEntry{route: nil})
	if handler := dynamic.findDynamicInto(MethodGet, "/cached", &Context{}); handler != nil {
		t.Fatalf("handler=%v", handler)
	}
	if handler := dynamic.findDynamicInto(MethodGet, "/missing", &Context{}); handler != nil {
		t.Fatalf("handler=%v", handler)
	}

}

func TestRouterAddAndMatchErrorBranches(t *testing.T) {
	router := &Router{config: &DefaultConfig}
	if err := router.Add(MethodGet, "/users"); err == nil || !strings.Contains(err.Error(), "no handler provided") {
		t.Fatalf("err=%v", err)
	}

	app := New()
	mustPanic(t, "no handler provided", func() {
		app.Match([]string{MethodGet}, "/users")
	})
}

func TestRouterAllowedMethodsSharedPathIndex(t *testing.T) {
	router := &Router{config: &DefaultConfig}
	mustDo(t, router.Add(MethodGet, "/shared/one", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodPost, "/shared/two", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodGet, "/users/{id}", func(*Context) error { return nil }))
	mustDo(t, router.Add(MethodPost, "/users/me", func(*Context) error { return nil }))

	shared := router.allowedMethods("/shared/one", true, true)
	if want := []string{MethodGet, MethodHead, MethodOptions}; !reflect.DeepEqual(shared, want) {
		t.Fatalf("shared allow = %v want %v", shared, want)
	}

	overlap := router.allowedMethods("/users/me", true, true)
	if want := []string{MethodGet, MethodHead, MethodPost, MethodOptions}; !reflect.DeepEqual(overlap, want) {
		t.Fatalf("overlap allow = %v want %v", overlap, want)
	}

	if header := router.allowedMethodHeader("/users/me", true, true); header != "GET, HEAD, POST, OPTIONS" {
		t.Fatalf("allow header = %q", header)
	}
}

func TestRouterDispatchIntoCachesMissResults(t *testing.T) {
	router := &Router{
		cache:  NewRouteCache(routeCacheMinRoutes + 8),
		config: &DefaultConfig,
	}
	for i := 0; i < routeCacheMinRoutes; i++ {
		path := fmt.Sprintf("/bulk/%d", i)
		mustDo(t, router.Add(MethodGet, path, func(*Context) error { return nil }))
	}
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

	key := routeCacheKey{method: MethodPut, path: "/only-post"}
	entry, ok := router.cache.get(key)
	if !ok {
		t.Fatal("expected dispatch miss to be cached")
	}
	if entry.route != nil {
		t.Fatalf("cached route=%v", entry.route)
	}
	if entry.allowed.mask != methodMaskPost || len(entry.allowed.extra) != 0 {
		t.Fatalf("cached allowed=%v", entry.allowed)
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

	entry, ok = router.cache.get(key)
	if ok {
		t.Fatalf("expected stale miss cache entry to clear on first access, got=%+v", entry)
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

func TestRouterDispatchIntoCachesManyParams(t *testing.T) {
	router := &Router{
		cache:  NewRouteCache(routeCacheMinRoutes + 8),
		config: &DefaultConfig,
	}
	for i := 0; i < routeCacheMinRoutes; i++ {
		path := fmt.Sprintf("/bulk/%d", i)
		mustDo(t, router.Add(MethodGet, path, func(*Context) error { return nil }))
	}

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
	if len(ctx.PathParams) < len(names) || ctx.PathParams[9].key != names[9] {
		t.Fatalf("path params not expanded: len=%d last=%+v", len(ctx.PathParams), ctx.PathParams[9])
	}

	key := routeCacheKey{method: MethodGet, path: path}
	if entry, ok := router.cache.get(key); !ok || entry.route == nil {
		t.Fatalf("expected cached hit entry, got=%+v ok=%v", entry, ok)
	}

	ctxCached := &Context{}
	handled, allowed, err = router.dispatchInto(MethodGet, path, false, ctxCached)
	if err != nil {
		t.Fatalf("cached err=%v", err)
	}
	if !handled {
		t.Fatal("expected cached dispatch to handle the request")
	}
	if !allowed.empty() {
		t.Fatalf("cached allowed=%v", allowed)
	}
	if got := ctxCached.Param(names[9]); got != "10" {
		t.Fatalf("cached param10=%q", got)
	}
}

func TestRouterDispatchIntoCachesSmallDynamicRouteSets(t *testing.T) {
	router := &Router{
		cache:  NewRouteCache(8),
		config: &DefaultConfig,
	}
	mustDo(t, router.Add(MethodGet, "/items/{id}", func(*Context) error { return nil }))

	ctx := &Context{}
	handled, allowed, err := router.dispatchInto(MethodGet, "/items/42", false, ctx)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !handled {
		t.Fatal("expected dynamic route to handle the request")
	}
	if !allowed.empty() {
		t.Fatalf("allowed=%v", allowed)
	}

	key := routeCacheKey{method: MethodGet, path: "/items/42"}
	entry, ok := router.cache.get(key)
	if !ok {
		t.Fatal("expected small dynamic route set to populate the dispatch cache")
	}
	if entry.route == nil {
		t.Fatalf("cached entry=%+v", entry)
	}
}

func TestRouterDispatchIntoRefreshesCachedDynamicHitAfterAdd(t *testing.T) {
	router := &Router{
		cache:  NewRouteCache(8),
		config: &DefaultConfig,
	}
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

	key := routeCacheKey{method: MethodGet, path: "/items/new"}
	entry, ok := router.cache.get(key)
	if !ok || entry.route == nil || entry.route.infoIndex != 0 {
		t.Fatalf("cached entry=%+v ok=%v", entry, ok)
	}

	mustDo(t, router.Add(MethodGet, "/items/new", func(*Context) error { return nil }))
	if entry, ok := router.cache.get(key); ok {
		t.Fatalf("expected stale cached hit to clear on first access after Add, got=%+v", entry)
	}

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
	router := &Router{
		cache:  NewRouteCache(8),
		config: &DefaultConfig,
	}
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

func TestRouterDispatchIntoCachedManyParamsIsolation(t *testing.T) {
	router := &Router{
		cache:  NewRouteCache(routeCacheMinRoutes + 8),
		config: &DefaultConfig,
	}
	for i := 0; i < routeCacheMinRoutes; i++ {
		path := fmt.Sprintf("/bulk/%d", i)
		mustDo(t, router.Add(MethodGet, path, func(*Context) error { return nil }))
	}

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

	shortCachedCtx := &Context{}
	handled, _, err = router.dispatchInto(MethodGet, pathShort, false, shortCachedCtx)
	if err != nil {
		t.Fatalf("cached err=%v", err)
	}
	if !handled {
		t.Fatal("expected cached short path to match")
	}
	if got := shortCachedCtx.Param(names[8]); got != "9" {
		t.Fatalf("cached param9=%q", got)
	}
	if got := shortCachedCtx.Param(names[9]); got != "10" {
		t.Fatalf("cached param10=%q", got)
	}
}

func TestRouterSupportsCustomDynamicMethods(t *testing.T) {
	const methodPurge = "PURGE"

	router := &Router{config: &DefaultConfig}
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
	router := &Router{
		config:        &Config{CaseSensitive: false},
		staticAllowed: make(map[string]allowedMethodSet),
	}
	router.staticAllowed["/case"] = allowedMethodSet{mask: methodMaskPost}

	if allowed := lookupStaticAllowed(router.staticAllowed, "/CASE", "/CASE", false); allowed.mask != methodMaskPost {
		t.Fatalf("static allowed=%v", allowed)
	}
	if allowed := lookupStaticAllowed(router.staticAllowed, "/CASE", "/CASE", true); !allowed.empty() {
		t.Fatalf("case-sensitive allowed=%v", allowed)
	}

	methods := router.lookupStaticAllowedMethods("/CASE", "/CASE", false).methods(true, true)
	if want := []string{MethodPost, MethodOptions}; !reflect.DeepEqual(methods, want) {
		t.Fatalf("methods=%v want %v", methods, want)
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
