// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppRoutingMiddlewareAndFallbacks(t *testing.T) {
	t.Run("middleware cache and params", func(t *testing.T) {
		app := New()
		calls := 0
		app.Use(func(c *Context) error {
			calls++
			return c.Next()
		})
		app.Get("/users/{id}", func(c *Context) error {
			return c.String(c.Param("id"))
		})

		for i := 0; i < 3; i++ {
			resp := performRequest(t, app, http.MethodGet, "/users/42", nil, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("status=%d", resp.Code)
			}
			if body := resp.Body.String(); body != "42" {
				t.Fatalf("body=%q", body)
			}
		}
		if calls != 3 {
			t.Fatalf("middleware calls=%d want 3", calls)
		}
	})

	t.Run("middleware post-next sees route metadata", func(t *testing.T) {
		app := New()
		fullPath := ""
		app.Use(func(c *Context) error {
			if err := c.Next(); err != nil {
				return err
			}
			fullPath = c.FullPath()
			return nil
		})
		app.Get("/post-next", func(c *Context) error {
			return c.String("ok")
		})

		resp := performRequest(t, app, http.MethodGet, "/post-next", nil, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d", resp.Code)
		}
		if fullPath != "/post-next" {
			t.Fatalf("fullPath=%q", fullPath)
		}
	})

	t.Run("prefix middleware and route metadata", func(t *testing.T) {
		app := New()
		app.UsePrefix("/api", func(c *Context) error {
			c.Set("prefix", true)
			return c.Next()
		})
		app.Route("/api", func(api *Group) {
			api.Use(func(c *Context) error {
				c.Set("group", "users")
				return c.Next()
			})
			api.Get("/users/{id}", func(c *Context) error {
				prefix, _ := c.Get("prefix")
				group, _ := c.Get("group")
				return c.JSON(Map{
					"prefix":    prefix,
					"group":     group,
					"full_path": c.FullPath(),
					"method":    c.Route().Method,
					"path":      c.Route().Path,
					"id":        c.Param("id"),
				})
			})
		})

		resp := performRequest(t, app, http.MethodGet, "/api/users/99", nil, nil)
		body := resp.Body.String()
		for _, fragment := range []string{"\"prefix\":true", "\"group\":\"users\"", "\"full_path\":\"/api/users/{id}\"", "\"method\":\"GET\"", "\"id\":\"99\""} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("body missing %q: %s", fragment, body)
			}
		}
	})

	t.Run("middleware stop semantics", func(t *testing.T) {
		app := New()
		called := false
		app.Use(func(c *Context) error {
			return nil
		})
		app.Get("/blocked", func(c *Context) error {
			called = true
			return c.String("nope")
		})

		resp := performRequest(t, app, http.MethodGet, "/blocked", nil, nil)
		if called {
			t.Fatal("handler should not have been called")
		}
		if resp.Code != http.StatusOK || resp.Body.Len() != 0 {
			t.Fatalf("unexpected response: code=%d body=%q", resp.Code, resp.Body.String())
		}
	})

	t.Run("not found and method not allowed", func(t *testing.T) {
		app := New()
		app.Get("/items", func(c *Context) error { return c.String("ok") })
		app.NotFound(func(c *Context) error { return c.String("missing") })
		app.MethodNotAllowed(func(c *Context) error { return c.String("wrong method") })

		notFound := performRequest(t, app, http.MethodGet, "/missing", nil, nil)
		if notFound.Code != http.StatusNotFound || notFound.Body.String() != "missing" {
			t.Fatalf("not found response = %d %q", notFound.Code, notFound.Body.String())
		}

		methodNA := performRequest(t, app, http.MethodPost, "/items", nil, nil)
		if methodNA.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d", methodNA.Code)
		}
		allow := methodNA.Header().Get(HeaderAllow)
		for _, method := range []string{MethodGet, MethodHead, MethodOptions} {
			if !strings.Contains(allow, method) {
				t.Fatalf("allow header missing %s: %q", method, allow)
			}
		}
		if methodNA.Body.String() != "wrong method" {
			t.Fatalf("body=%q", methodNA.Body.String())
		}
	})

	t.Run("auto head and auto options", func(t *testing.T) {
		app := New()
		app.Get("/health", func(c *Context) error {
			return c.String("ok")
		})

		head := performRequest(t, app, http.MethodHead, "/health", nil, nil)
		if head.Code != http.StatusOK || head.Body.Len() != 0 {
			t.Fatalf("head response = %d %q", head.Code, head.Body.String())
		}

		options := performRequest(t, app, http.MethodOptions, "/health", nil, nil)
		if options.Code != http.StatusNoContent {
			t.Fatalf("status=%d", options.Code)
		}
		if allow := options.Header().Get(HeaderAllow); !strings.Contains(allow, MethodGet) {
			t.Fatalf("allow=%q", allow)
		}
	})

	t.Run("method not allowed unions overlapping route shapes", func(t *testing.T) {
		app := New()
		app.Get("/users/{id}", func(c *Context) error { return c.String("param") })
		app.Post("/users/me", func(c *Context) error { return c.String("static") })

		resp := performRequest(t, app, http.MethodPut, "/users/me", nil, nil)
		if resp.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d", resp.Code)
		}
		if allow := resp.Header().Get(HeaderAllow); allow != "GET, HEAD, POST, OPTIONS" {
			t.Fatalf("allow=%q", allow)
		}

		options := performRequest(t, app, http.MethodOptions, "/users/me", nil, nil)
		if options.Code != http.StatusNoContent {
			t.Fatalf("status=%d", options.Code)
		}
		if allow := options.Header().Get(HeaderAllow); allow != "GET, HEAD, POST, OPTIONS" {
			t.Fatalf("allow=%q", allow)
		}
	})

	t.Run("custom methods route and advertise allow", func(t *testing.T) {
		app := New()
		app.Add("PURGE", "/cache/{key}", func(c *Context) error {
			return c.String(c.Param("key"))
		})

		purge := performRequest(t, app, "PURGE", "/cache/home", nil, nil)
		if purge.Code != http.StatusOK || purge.Body.String() != "home" {
			t.Fatalf("purge response = %d %q", purge.Code, purge.Body.String())
		}

		get := performRequest(t, app, http.MethodGet, "/cache/home", nil, nil)
		if get.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d", get.Code)
		}
		if allow := get.Header().Get(HeaderAllow); allow != "OPTIONS, PURGE" {
			t.Fatalf("allow=%q", allow)
		}

		options := performRequest(t, app, http.MethodOptions, "/cache/home", nil, nil)
		if options.Code != http.StatusNoContent {
			t.Fatalf("status=%d", options.Code)
		}
		if allow := options.Header().Get(HeaderAllow); allow != "OPTIONS, PURGE" {
			t.Fatalf("allow=%q", allow)
		}
	})

	t.Run("mount strips prefix", func(t *testing.T) {
		app := New()
		mux := http.NewServeMux()
		mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, r.URL.Path)
		})
		app.Mount("/sub", mux)

		resp := performRequest(t, app, http.MethodGet, "/sub/hello", nil, nil)
		if resp.Body.String() != "/hello" {
			t.Fatalf("body=%q", resp.Body.String())
		}

		routes := app.Routes()
		foundMount := false
		for _, route := range routes {
			if route.Method == methodUse && route.Path == "/sub" {
				if !route.Mounted {
					t.Fatalf("mount route should be marked mounted: %+v", route)
				}
				foundMount = true
			}
		}
		if !foundMount {
			t.Fatal("mount route not present in Routes()")
		}
	})
}

func TestAppConfigLifecycleAndErrors(t *testing.T) {
	t.Run("new with config and handler", func(t *testing.T) {
		app := NewWithConfig(Config{ServerHeader: "zinc-test", CaseSensitive: true})
		if got := app.Handler(); got != app {
			t.Fatal("Handler should return app")
		}
		if app.config.ServerHeader != "zinc-test" {
			t.Fatalf("server header=%q", app.config.ServerHeader)
		}
		if !app.config.CaseSensitive {
			t.Fatal("CaseSensitive should be true")
		}
	})

	t.Run("case sensitive and strict routing", func(t *testing.T) {
		app := NewWithConfig(Config{CaseSensitive: true, StrictRouting: true})
		app.Get("/Hello", func(c *Context) error { return c.String("ok") })

		lower := performRequest(t, app, http.MethodGet, "/hello", nil, nil)
		if lower.Code != http.StatusNotFound {
			t.Fatalf("status=%d", lower.Code)
		}

		slash := performRequest(t, app, http.MethodGet, "/Hello/", nil, nil)
		if slash.Code != http.StatusNotFound {
			t.Fatalf("status=%d", slash.Code)
		}
	})

	t.Run("custom error handler", func(t *testing.T) {
		app := NewWithConfig(Config{ErrorHandler: func(c *Context, err error) {
			_ = c.Status(http.StatusTeapot).String("handled")
		}})
		app.Get("/boom", func(c *Context) error {
			return errors.New("boom")
		})

		resp := performRequest(t, app, http.MethodGet, "/boom", nil, nil)
		if resp.Code != http.StatusTeapot || resp.Body.String() != "handled" {
			t.Fatalf("response=%d %q", resp.Code, resp.Body.String())
		}
	})

	t.Run("server header", func(t *testing.T) {
		app := NewWithConfig(Config{ServerHeader: "zinc/edge"})
		app.Get("/", func(c *Context) error { return c.String("ok") })
		resp := performRequest(t, app, http.MethodGet, "/", nil, nil)
		if got := resp.Header().Get(HeaderServer); got != "zinc/edge" {
			t.Fatalf("server header=%q", got)
		}
	})

	t.Run("serve and shutdown", func(t *testing.T) {
		app := New()
		app.Get("/ping", func(c *Context) error { return c.String("pong") })

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		mustDo(t, err)
		defer ln.Close()

		errCh := make(chan error, 1)
		go func() { errCh <- app.Serve(ln) }()

		url := "http://" + ln.Addr().String() + "/ping"
		var resp *http.Response
		for i := 0; i < 20; i++ {
			resp, err = http.Get(url)
			if err == nil {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		mustDo(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		mustDo(t, err)
		if string(body) != "pong" {
			t.Fatalf("body=%q", string(body))
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		mustDo(t, app.Shutdown(ctx))
		select {
		case err := <-errCh:
			mustDo(t, err)
		case <-time.After(time.Second):
			t.Fatal("server did not shut down")
		}
	})

	t.Run("invalid lifecycle inputs", func(t *testing.T) {
		app := New()
		if err := app.Serve(nil); err == nil {
			t.Fatal("Serve(nil) should fail")
		}
		if err := app.Listen("bad-addr"); err == nil {
			t.Fatal("Listen should fail for invalid address")
		}
		if err := app.ListenTLS("bad-addr", "missing.crt", "missing.key"); err == nil {
			t.Fatal("ListenTLS should fail for invalid address")
		}
		if err := app.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown without server: %v", err)
		}
	})

	t.Run("listen resolves address", func(t *testing.T) {
		tests := []struct {
			name string
			addr []string
			want string
		}{
			{name: "default", want: DefaultListenAddr},
			{name: "empty uses default", addr: []string{""}, want: DefaultListenAddr},
			{name: "custom", addr: []string{":3000"}, want: ":3000"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := resolveListenAddr(tt.addr...)
				mustDo(t, err)
				if got != tt.want {
					t.Fatalf("addr=%q, want %q", got, tt.want)
				}
			})
		}

		if _, err := resolveListenAddr(":3000", ":4000"); err == nil {
			t.Fatal("resolveListenAddr should fail with multiple addresses")
		}

		if err := New().Listen("bad-addr", ":4000"); err == nil {
			t.Fatal("Listen should fail with multiple addresses")
		}
	})
}

func TestWrapAndWrapFunc(t *testing.T) {
	app := New()
	app.Get("/h", Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "handler")
	})))
	app.Get("/f", WrapFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "func")
	}))

	resp1 := performRequest(t, app, http.MethodGet, "/h", nil, nil)
	if resp1.Body.String() != "handler" {
		t.Fatalf("body=%q", resp1.Body.String())
	}
	resp2 := performRequest(t, app, http.MethodGet, "/f", nil, nil)
	if resp2.Body.String() != "func" {
		t.Fatalf("body=%q", resp2.Body.String())
	}
}

func TestStaticAndFileRoutes(t *testing.T) {
	dir := t.TempDir()
	mustDo(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644))

	app := New()
	mustDo(t, app.Static("/assets", dir))
	mustDo(t, app.File("/single", filepath.Join(dir, "hello.txt")))

	assets := performRequest(t, app, http.MethodGet, "/assets/hello.txt", nil, nil)
	if assets.Body.String() != "world" {
		t.Fatalf("body=%q", assets.Body.String())
	}

	single := performRequest(t, app, http.MethodGet, "/single", nil, nil)
	if single.Body.String() != "world" {
		t.Fatalf("body=%q", single.Body.String())
	}
}

func TestNamedRoutesAndURLGeneration(t *testing.T) {
	app := New()
	api := app.Group("/api")

	api.Handle(RouteSpec{
		Name:   "users.show",
		Method: MethodGet,
		Path:   "/users/{id}",
		Handler: func(c *Context) error {
			info := c.Route()
			return c.JSON(Map{
				"name":  info.Name,
				"path":  info.Path,
				"param": info.Params[0],
				"id":    c.Param("id"),
			})
		},
	})

	route, ok := app.RouteByName("users.show")
	if !ok {
		t.Fatal("expected named route")
	}
	if route.Name != "users.show" || route.Path != "/api/users/{id}" || len(route.Params) != 1 || route.Params[0] != "id" {
		t.Fatalf("route=%+v", route)
	}

	url, err := app.URL("users.show", "42")
	mustDo(t, err)
	if url != "/api/users/42" {
		t.Fatalf("url=%q", url)
	}

	resp := performRequest(t, app, http.MethodGet, "/api/users/42", nil, nil)
	body := resp.Body.String()
	for _, fragment := range []string{`"name":"users.show"`, `"path":"/api/users/{id}"`, `"param":"id"`, `"id":"42"`} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("body missing %q: %s", fragment, body)
		}
	}
}

func TestNamedRouteWildcardAndDuplicateName(t *testing.T) {
	app := New()

	app.Handle(RouteSpec{
		Name:    "files.show",
		Method:  MethodGet,
		Path:    "/files/{rest...}",
		Handler: func(c *Context) error { return c.String("ok") },
	})

	url, err := app.URL("files.show", "a/b/c.txt")
	mustDo(t, err)
	if url != "/files/a/b/c.txt" {
		t.Fatalf("url=%q", url)
	}

	if _, err := app.URL("files.show"); err == nil {
		t.Fatal("expected param count error")
	}

	err = app.TryHandle(RouteSpec{
		Name:    "files.show",
		Method:  MethodPost,
		Path:    "/files",
		Handler: func(c *Context) error { return c.String("dup") },
	})
	if err == nil || !strings.Contains(err.Error(), "route name already registered") {
		t.Fatalf("err=%v", err)
	}
}

func TestTryHandleDynamicRegistration(t *testing.T) {
	app := New()
	handler := func(c *Context) error { return c.String("ok") }

	if err := app.TryHandle(RouteSpec{
		Name:    "dynamic.show",
		Method:  MethodGet,
		Path:    "/dynamic/{id}",
		Handler: handler,
	}); err != nil {
		t.Fatalf("register dynamic route: %v", err)
	}
	if err := app.TryHandle(RouteSpec{
		Name:    "dynamic.duplicate",
		Method:  MethodGet,
		Path:    "/dynamic/{id}",
		Handler: handler,
	}); err == nil {
		t.Fatal("expected duplicate dynamic route error")
	}
	if err := app.TryHandle(RouteSpec{Method: MethodGet, Path: "/nil"}); err == nil {
		t.Fatal("expected nil dynamic handler error")
	}

	group := app.Group("/api")
	if err := group.TryHandle(RouteSpec{
		Name:    "group.dynamic",
		Method:  MethodPost,
		Path:    "/dynamic",
		Handler: handler,
	}); err != nil {
		t.Fatalf("register group dynamic route: %v", err)
	}
	if route, ok := app.RouteByName("group.dynamic"); !ok || route.Path != "/api/dynamic" {
		t.Fatalf("group dynamic route=%+v ok=%v", route, ok)
	}

	mustPanic(t, "route already registered", func() {
		app.Get("/dynamic/{id}", handler)
	})
}

func TestAppRejectsLegacyRoutePatterns(t *testing.T) {
	app := New()
	handler := func(c *Context) error { return c.String("ok") }
	for _, pattern := range []string{"/users/:id", "/users/prefix:id", "/files/*path", "/files/prefix*path", "/users/:id<\\d+>"} {
		err := app.TryHandle(RouteSpec{Method: MethodGet, Path: pattern, Handler: handler})
		if err == nil || !strings.Contains(err.Error(), "legacy route") {
			t.Fatalf("pattern=%q err=%v", pattern, err)
		}
	}
	mustPanic(t, "legacy route", func() {
		app.Get("/users/:id", handler)
	})
}

func TestRouteIntrospectionHelpers(t *testing.T) {
	app := New()
	api := app.Group("/api")
	api.Handle(RouteSpec{
		Name:    "users.show",
		Method:  MethodGet,
		Path:    "/users/{id}",
		Handler: func(c *Context) error { return c.String("ok") },
	})
	app.Post("/submit", func(c *Context) error { return c.String("ok") })

	found, ok := app.FindRoute(MethodGet, "/api/users/17")
	if !ok {
		t.Fatal("expected route match")
	}
	if found.Name != "users.show" || found.Path != "/api/users/{id}" {
		t.Fatalf("found=%+v", found)
	}

	getRoutes := app.RoutesByMethod(MethodGet)
	if len(getRoutes) != 1 || getRoutes[0].Name != "users.show" {
		t.Fatalf("get routes=%v", getRoutes)
	}

	apiRoutes := app.RoutesByPrefix("/api")
	if len(apiRoutes) != 1 || apiRoutes[0].Name != "users.show" {
		t.Fatalf("api routes=%v", apiRoutes)
	}
}

func TestMountedRouteIntrospection(t *testing.T) {
	app := New()
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	app.Mount("/sub", mux)

	found, ok := app.FindRoute(MethodGet, "/sub/hello")
	if !ok {
		t.Fatal("expected mounted route match")
	}
	if !found.Mounted || found.Path != "/sub" || found.Method != methodUse {
		t.Fatalf("found=%+v", found)
	}

	routes := app.RoutesByPrefix("/sub")
	if len(routes) != 1 || !routes[0].Mounted {
		t.Fatalf("routes=%v", routes)
	}
}

func TestRouteNotFoundPatterns(t *testing.T) {
	app := New()
	api := app.Group("/api", func(c *Context) error {
		c.Set("mw", "group")
		return c.Next()
	})

	app.RouteNotFound("/docs", func(c *Context) error {
		return c.String("docs")
	})
	api.RouteNotFound("/users/{id}", func(c *Context) error {
		if got, _ := c.Get("mw"); got != "group" {
			t.Fatalf("mw=%v", got)
		}
		return c.String("user:" + c.Param("id"))
	})
	app.RouteNotFound("/files/{path...}", func(c *Context) error {
		return c.String("file:" + c.Param("path"))
	})
	app.RouteNotFound("/silent", func(c *Context) error {
		return nil
	})
	app.NotFound(func(c *Context) error {
		return c.String("global")
	})

	docs := performRequest(t, app, http.MethodGet, "/docs", nil, nil)
	if docs.Code != http.StatusNotFound || docs.Body.String() != "docs" {
		t.Fatalf("docs=%d %q", docs.Code, docs.Body.String())
	}

	user := performRequest(t, app, http.MethodGet, "/api/users/42", nil, nil)
	if user.Code != http.StatusNotFound || user.Body.String() != "user:42" {
		t.Fatalf("user=%d %q", user.Code, user.Body.String())
	}

	file := performRequest(t, app, http.MethodGet, "/files/a/b/c.txt", nil, nil)
	if file.Code != http.StatusNotFound || file.Body.String() != "file:a/b/c.txt" {
		t.Fatalf("file=%d %q", file.Code, file.Body.String())
	}

	silent := performRequest(t, app, http.MethodGet, "/silent", nil, nil)
	if silent.Code != http.StatusNotFound || strings.TrimSpace(silent.Body.String()) != http.StatusText(http.StatusNotFound) {
		t.Fatalf("silent=%d %q", silent.Code, silent.Body.String())
	}

	other := performRequest(t, app, http.MethodGet, "/missing", nil, nil)
	if other.Code != http.StatusNotFound || other.Body.String() != "global" {
		t.Fatalf("other=%d %q", other.Code, other.Body.String())
	}
}

func TestAppAndDispatchEdgeCoverage(t *testing.T) {
	t.Run("use with zero handlers keeps middleware chain nil", func(t *testing.T) {
		app := New()
		app.Use()
		if app.middlewareChain != nil {
			t.Fatalf("middlewareChain=%v", app.middlewareChain)
		}
		if handlers := app.preHandlersForPath("/x"); handlers != nil {
			t.Fatalf("pre handlers=%v", handlers)
		}
	})

	t.Run("listen tls reaches serveTLS branch", func(t *testing.T) {
		app := New()
		err := app.ListenTLS("127.0.0.1:0", "missing.crt", "missing.key")
		if err == nil {
			t.Fatal("ListenTLS should fail without certificate files")
		}
	})

	t.Run("routes without mounts returns router routes directly", func(t *testing.T) {
		app := New()
		app.Get("/plain", func(c *Context) error { return c.String("ok") })
		routes := app.Routes()
		if len(routes) != 1 || routes[0].Path != "/plain" {
			t.Fatalf("routes=%v", routes)
		}
	})

	t.Run("middleware errors hit both ServeHTTP branches", func(t *testing.T) {
		app := New()
		app.Use(func(*Context) error { return errors.New("mw boom") })
		app.Get("/x", func(c *Context) error { return c.String("ok") })

		resp := performRequest(t, app, http.MethodGet, "/x", nil, nil)
		if resp.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", resp.Code)
		}

		app2 := New()
		app2.UsePrefix("/api", func(*Context) error { return errors.New("prefix boom") })
		app2.Get("/api/x", func(c *Context) error { return c.String("ok") })
		resp2 := performRequest(t, app2, http.MethodGet, "/api/x", nil, nil)
		if resp2.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", resp2.Code)
		}
	})

	t.Run("method not allowed and not found fallback branches", func(t *testing.T) {
		app := NewWithConfig(Config{HandleMethodNotAllowed: true})
		app.Get("/only", func(c *Context) error { return c.String("ok") })

		mna := performRequest(t, app, http.MethodPost, "/only", nil, nil)
		if mna.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d", mna.Code)
		}
		if body := strings.TrimSpace(mna.Body.String()); body != http.StatusText(http.StatusMethodNotAllowed) {
			t.Fatalf("body=%q", mna.Body.String())
		}
		if allow := mna.Header().Get(HeaderAllow); allow != "GET" {
			t.Fatalf("allow=%q", allow)
		}
		if ctype := mna.Header().Get(HeaderContentType); ctype != "text/plain; charset=utf-8" {
			t.Fatalf("content-type=%q", ctype)
		}

		mnaHead := performRequest(t, app, http.MethodHead, "/missing", nil, nil)
		if mnaHead.Code != http.StatusNotFound {
			t.Fatalf("status=%d", mnaHead.Code)
		}
		if mnaHead.Body.Len() != 0 {
			t.Fatalf("body=%q", mnaHead.Body.String())
		}

		app.MethodNotAllowed(func(*Context) error { return nil })
		mnaNoWrite := performRequest(t, app, http.MethodPost, "/only", nil, nil)
		if mnaNoWrite.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d", mnaNoWrite.Code)
		}
		if mnaNoWrite.Body.Len() != 0 {
			t.Fatalf("body=%q", mnaNoWrite.Body.String())
		}

		app.NotFound(func(*Context) error { return nil })
		notFound := performRequest(t, app, http.MethodGet, "/missing", nil, nil)
		if notFound.Code != http.StatusNotFound {
			t.Fatalf("status=%d", notFound.Code)
		}
		if body := strings.TrimSpace(notFound.Body.String()); body != http.StatusText(http.StatusNotFound) {
			t.Fatalf("body=%q", notFound.Body.String())
		}
		if ctype := notFound.Header().Get(HeaderContentType); ctype != "text/plain; charset=utf-8" {
			t.Fatalf("content-type=%q", ctype)
		}
	})

	t.Run("custom error handler still handles default 404 and 405", func(t *testing.T) {
		app := NewWithConfig(Config{
			HandleMethodNotAllowed: true,
			ErrorHandler: func(c *Context, err error) {
				_ = c.Status(http.StatusTeapot).String("handled")
			},
		})
		app.Get("/only", func(c *Context) error { return c.String("ok") })

		notFound := performRequest(t, app, http.MethodGet, "/missing", nil, nil)
		if notFound.Code != http.StatusTeapot || notFound.Body.String() != "handled" {
			t.Fatalf("notFound=%d %q", notFound.Code, notFound.Body.String())
		}

		methodNA := performRequest(t, app, http.MethodPost, "/only", nil, nil)
		if methodNA.Code != http.StatusTeapot || methodNA.Body.String() != "handled" {
			t.Fatalf("methodNA=%d %q", methodNA.Code, methodNA.Body.String())
		}
	})

	t.Run("direct dispatch helpers", func(t *testing.T) {
		app := New()
		app.handleError(nil, nil) // no-op branch

		mustDo(t, appDispatchHandler(nil))
		c := &Context{}
		mustDo(t, appDispatchHandler(c))
	})

	t.Run("mounted handler and stripMountPrefix branches", func(t *testing.T) {
		ctx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
		defer ctx.release()

		var nilMount *mountedHandler
		nilMount.serve(ctx) // no-op branch

		emptyMount := &mountedHandler{}
		emptyMount.serve(ctx) // no-op branch

		req := httptest.NewRequest(http.MethodGet, "http://example.com/sub/%61", nil)
		req.URL.Path = "/sub/a"
		req.URL.RawPath = "/sub/%61"
		req.RequestURI = "/sub/%61"
		ctx2, rec2 := newRecorderContext(t, req)
		defer ctx2.release()
		mount := &mountedHandler{
			prefixPath: "/sub",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, r.URL.Path+"|"+r.URL.RawPath)
			}),
		}
		mount.serve(ctx2)
		if body := rec2.Body.String(); body != "/a|/%61" {
			t.Fatalf("body=%q", body)
		}
		if req.URL.Path != "/sub/a" || req.URL.RawPath != "/sub/%61" || req.RequestURI != "/sub/%61" {
			t.Fatalf("mount mutated original request: path=%q rawPath=%q requestURI=%q", req.URL.Path, req.URL.RawPath, req.RequestURI)
		}

		if got := stripMountPrefix("/sub/hello", "/"); got != "/sub/hello" {
			t.Fatalf("strip=%q", got)
		}
		if got := stripMountPrefix("/sub", "/sub"); got != "/" {
			t.Fatalf("strip=%q", got)
		}
		if got := stripMountPrefix("/subhello", "/sub"); got != "/hello" {
			t.Fatalf("strip=%q", got)
		}
	})
}

func TestContextErrorAndLastError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, rec := newRecorderContext(t, req)
	defer ctx.release()

	if ctx.LastError() != nil {
		t.Fatalf("last error=%v", ctx.LastError())
	}

	ctx.app = nil
	ctx.Error(nil)
	if ctx.LastError() != nil {
		t.Fatalf("last error=%v", ctx.LastError())
	}

	want := errors.New("boom")
	ctx.Error(want)
	if !errors.Is(ctx.LastError(), want) {
		t.Fatalf("last error=%v", ctx.LastError())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}

	hit := false
	ctx2, rec2 := newRecorderContext(t, req)
	defer ctx2.release()
	ctx2.app = NewWithConfig(Config{
		ErrorHandler: func(c *Context, err error) {
			hit = true
			_ = c.Status(http.StatusTeapot).String("teapot")
			if !errors.Is(err, want) {
				t.Fatalf("error handler err=%v", err)
			}
		},
	})
	ctx2.Error(want)

	if !hit {
		t.Fatal("custom error handler was not invoked")
	}
	if rec2.Code != http.StatusTeapot {
		t.Fatalf("status=%d", rec2.Code)
	}
	if !errors.Is(ctx2.LastError(), want) {
		t.Fatalf("last error=%v", ctx2.LastError())
	}
}

func TestDefaultErrorHandlerBranches(t *testing.T) {
	defaultErrorHandler(nil, errors.New("ignored"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, rec := newRecorderContext(t, req)
	defer ctx.release()

	defaultErrorHandler(ctx, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}

	ctx.written = true
	defaultErrorHandler(ctx, errors.New("ignored"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}

	ctx2, rec2 := newRecorderContext(t, req)
	defer ctx2.release()
	defaultErrorHandler(ctx2, NewError(http.StatusConflict).WithMessage("conflict"))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("status=%d", rec2.Code)
	}
	if body := rec2.Body.String(); body != "conflict" {
		t.Fatalf("body=%q", body)
	}

	ctx3, rec3 := newRecorderContext(t, req)
	defer ctx3.release()
	wrapped := fmt.Errorf("wrapped: %w", NewError(http.StatusGone))
	defaultErrorHandler(ctx3, wrapped)
	if rec3.Code != http.StatusGone {
		t.Fatalf("status=%d", rec3.Code)
	}

	ctx4, rec4 := newRecorderContext(t, req)
	defer ctx4.release()
	defaultErrorHandler(ctx4, errors.New("boom"))
	if rec4.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec4.Code)
	}

	ctx5, rec5 := newRecorderContext(t, req)
	defer ctx5.release()
	httpErr := NewError(http.StatusUnauthorized).
		WithMessage("denied").
		WithHeader("X-Reason", "auth").
		WithCause(errors.New("root cause")).
		WithMeta("kind", "auth")
	defaultErrorHandler(ctx5, httpErr)
	if rec5.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec5.Code)
	}
	if rec5.Header().Get("X-Reason") != "auth" {
		t.Fatalf("x-reason=%q", rec5.Header().Get("X-Reason"))
	}
	if !errors.Is(httpErr, httpErr.Cause) {
		t.Fatal("expected cause to unwrap")
	}
	if httpErr.Meta["kind"] != "auth" {
		t.Fatalf("meta=%v", httpErr.Meta)
	}
}

func TestHTTPErrorHelpersCloneGlobalsAndAbortHelpers(t *testing.T) {
	base := ErrNotFound
	updated := base.WithMessage("custom").WithHeader("X-Test", "ok")
	if base.Message != "" {
		t.Fatalf("base message=%q", base.Message)
	}
	if base.Headers != nil {
		t.Fatalf("base headers=%v", base.Headers)
	}
	if updated.Message != "custom" || updated.Headers.Get("X-Test") != "ok" {
		t.Fatalf("updated=%+v", updated)
	}

	app := New()
	app.Get("/abort", func(c *Context) error {
		return c.AbortWithStatus(http.StatusForbidden)
	})
	app.Get("/abort-json", func(c *Context) error {
		return c.AbortWithJSON(http.StatusCreated, Map{"ok": true})
	})

	abortResp := performRequest(t, app, http.MethodGet, "/abort", nil, nil)
	if abortResp.Code != http.StatusForbidden {
		t.Fatalf("status=%d", abortResp.Code)
	}

	jsonResp := performRequest(t, app, http.MethodGet, "/abort-json", nil, nil)
	if jsonResp.Code != http.StatusCreated || !strings.Contains(jsonResp.Body.String(), `"ok":true`) {
		t.Fatalf("json resp=%d %q", jsonResp.Code, jsonResp.Body.String())
	}
}
