package zinc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

// Cache state and request spelling must not change the externally visible
// routing decision, including after a non-executing Find warms the cache.
func FuzzRoutingCacheEquivalence(f *testing.F) {
	build := func(size int) *App {
		cfg := Config{}
		cfg.RouteCacheSize = size
		app := New(cfg)
		for i := 0; i < 80; i++ {
			for _, method := range []string{"GET", "POST", "CUSTOM"} {
				app.Add(method, fmt.Sprintf("/r%d/{id}/{part}", i), func(c *Context) error { return c.Send(c.FullPath() + "|" + c.Param("id") + "|" + c.Param("part")) })
			}
		}
		app.Get("/k/{id}/{part}", func(c *Context) error { return c.Send(c.Param("id") + "|" + c.Param("part")) })
		app.Get("/r1/new/item", func(c *Context) error { return c.Send("static") })
		return app
	}
	cached, plain := build(100), build(-1) // -1 disables the cache
	for _, path := range []string{"/r1/a/b", "/r1/NEW/item", "/R1/K/İ/", "/K/ABC/İ", "/missing", "/r79/%2F/x", "//r2/a/b"} {
		f.Add(path, uint8(0))
	}
	f.Fuzz(func(t *testing.T, path string, index uint8) {
		if len(path) > 8192 {
			t.Skip()
		}
		methods := []string{"GET", "POST", "CUSTOM", "HEAD", "OPTIONS", "DELETE"}
		method := methods[int(index)%len(methods)]
		for _, app := range []*App{plain, cached} {
			app.router.Find(method, path)
		}
		for pass := 0; pass < 2; pass++ {
			invoke := func(app *App) *httptest.ResponseRecorder {
				r := &http.Request{Method: method, URL: &url.URL{Path: path}, Header: make(http.Header), Body: http.NoBody}
				w := httptest.NewRecorder()
				app.ServeHTTP(w, r)
				return w
			}
			a, b := invoke(plain), invoke(cached)
			if a.Code != b.Code || a.Body.String() != b.Body.String() || !reflect.DeepEqual(a.Header(), b.Header()) {
				t.Fatalf("cache changed response for %s %q: %d %q vs %d %q", method, path, a.Code, a.Body.String(), b.Code, b.Body.String())
			}
		}
	})
}
