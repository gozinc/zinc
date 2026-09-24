package zinc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutingFoldContractsWithAndWithoutCache(t *testing.T) {
	for _, size := range []int{0, 1000} {
		cfg := DefaultConfig
		cfg.RouteCacheSize = size
		app := NewWithConfig(cfg)
		app.Get("/users/new", func(c *Context) error { return c.Send("static") })
		app.Get("/users/{id}", func(c *Context) error { return c.Send(c.Param("id")) })
		app.Get("/k/new", func(c *Context) error { return c.Send("unicode-static") })
		app.Get("/k/{id}/{part}/{third}", func(c *Context) error { return c.Send(c.Param("id") + "|" + c.Param("part") + "|" + c.Param("third")) })
		for i := 0; i < 80; i++ {
			app.Get(fmt.Sprintf("/r%d/{id}", i), func(c *Context) error { return c.Send(c.Param("id")) })
		}
		for pass := 0; pass < 4; pass++ {
			for _, tt := range []struct{ path, want string }{{"/users/NEW", "static"}, {"/K/NEW", "unicode-static"}, {"/K/ABC/İ/XKZ", "ABC|İ|XKZ"}, {"/K/ABC/İ/XKZ/", "ABC|İ|XKZ"}} {
				w := httptest.NewRecorder()
				app.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
				if w.Body.String() != tt.want {
					t.Fatalf("cache=%d path=%s got=%q want=%q", size, tt.path, w.Body.String(), tt.want)
				}
				handler, c := app.router.Find("GET", tt.path)
				if handler == nil {
					t.Fatal("Find missed route")
				}
				if strings.Contains(tt.want, "|") && c.Param("id")+"|"+c.Param("part")+"|"+c.Param("third") != tt.want {
					t.Fatalf("Find corrupted params: %q %q %q", c.Param("id"), c.Param("part"), c.Param("third"))
				}
			}
		}
	}
}

func TestMountEscapedAndCaseFoldedRequest(t *testing.T) {
	for _, tt := range []struct{ prefix, target, path, uri string }{
		{"/api", "/API/item?x=1", "/item", "/item?x=1"},
		{"/api", "/%61pi/a%2Fb?x=1", "/a/b", "/a%2Fb?x=1"},
		{"/k", "/%E2%84%AA/%61", "/a", "/%61"},
		{"/api", "/api%2Fitem", "/item", "/item"},
		{"/api", "/api?x=1", "/", "/?x=1"},
	} {
		app := New()
		app.Mount(tt.prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != tt.path || r.RequestURI != tt.uri || r.URL.RequestURI() != tt.uri {
				t.Errorf("%s: path=%q uri=%q raw=%q", tt.target, r.URL.Path, r.RequestURI, r.URL.RawPath)
			}
		}))
		r := httptest.NewRequest("GET", tt.target, nil)
		before := *r.URL
		uri := r.RequestURI
		app.ServeHTTP(httptest.NewRecorder(), r)
		if *r.URL != before || r.RequestURI != uri {
			t.Fatal("original request mutated")
		}
	}
}

func TestNamedURLSegmentPolicy(t *testing.T) {
	app := New()
	app.Handle(RouteSpec{Name: "item", Method: "GET", Path: "/items/{id}", Handler: func(c *Context) error { return c.Send(c.Param("id")) }})
	app.Handle(RouteSpec{Name: "file", Method: "GET", Path: "/files/{path...}", Handler: func(c *Context) error { return c.Send(c.Param("path")) }})
	for _, value := range []string{"", "a/b"} {
		if _, err := app.URL("item", value); err == nil {
			t.Fatalf("accepted unsupported segment %q", value)
		}
	}
	for _, tt := range []struct{ name, value string }{{"item", "a?b#c% d"}, {"file", "a/b?c#d% e"}} {
		path, err := app.URL(tt.name, tt.value)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.String() != tt.value {
			t.Fatalf("%s: %d %q", path, w.Code, w.Body.String())
		}
	}
}

func TestRouteCacheOversizedKeyBypass(t *testing.T) {
	cache := NewRouteCache(10)
	key := routeCacheKey{method: "GET", path: strings.Repeat("x", routeCacheMaxKeyBytes+1)}
	cache.setWithMask(key, methodMaskFor("GET"), routeCacheEntry{})
	if _, ok := cache.getWithMask(key, methodMaskFor("GET")); ok {
		t.Fatal("oversized request retained")
	}
}
