package zinc_test

import (
	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestNestedGroupFileHelpersPreserveMiddleware(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(file, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{"Mount", "Static", "StaticFS", "File", "FileFS"} {
		t.Run(helper, func(t *testing.T) {
			app := zinc.New()
			var order []string
			wrap := func(name string) zinc.HandlerFunc {
				return func(c *zinc.Context) error {
					order = append(order, name+":before")
					err := c.Next()
					order = append(order, name+":after")
					return err
				}
			}
			app.Use(wrap("global"))
			group := app.Group("/parent", wrap("parent")).Group("/child", wrap("child"))
			var err error
			switch helper {
			case "Mount":
				group.Mount("/files", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "secret") }))
			case "Static":
				err = group.Static("/files", root)
			case "StaticFS":
				err = group.StaticFS("/files", os.DirFS(root))
			case "File":
				err = group.File("/files/secret.txt", file)
			case "FileFS":
				err = group.FileFS("/files/secret.txt", "secret.txt", os.DirFS(root))
			}
			if err != nil {
				t.Fatal(err)
			}
			// Registration snapshots the group chain, as ordinary routes do.
			group.Use(func(c *zinc.Context) error { return zinc.ErrUnauthorized })
			w := hardeningRequest(app, "GET", "/parent/child/files/secret.txt")
			if w.Code != 200 || w.Body.String() != "secret" {
				t.Fatalf("response: %d %q", w.Code, w.Body.String())
			}
			want := []string{"global:before", "parent:before", "child:before", "child:after", "parent:after", "global:after"}
			if !reflect.DeepEqual(order, want) {
				t.Fatalf("order: %v", order)
			}
		})
	}
}

func TestPrefixRewriteRechecksEarlierScopes(t *testing.T) {
	app := zinc.New()
	app.UsePrefix("/private", func(c *zinc.Context) error { return zinc.ErrUnauthorized })
	app.UsePrefix("/public", middleware.Rewrite("/public", "/private"))
	app.Get("/private", func(c *zinc.Context) error { return c.String("secret") })
	if w := hardeningRequest(app, "GET", "/public"); w.Code != 401 {
		t.Fatalf("status %d", w.Code)
	}
}

func TestStaticRootContainsSymlinksAndDecodesOnce(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(filepath.Join(root, "%2F.txt"), []byte("literal"), 0600); err != nil {
		t.Fatal(err)
	}
	app := zinc.New()
	if err := app.Static("/files", root); err != nil {
		t.Fatal(err)
	}
	if w := hardeningRequest(app, "GET", "/files/escape/secret"); w.Code == 200 {
		t.Fatal("symlink escaped static root")
	}
	if w := hardeningRequest(app, "GET", "/files/%252F.txt"); w.Code != 200 || w.Body.String() != "literal" {
		t.Fatalf("literal escaped name: %d %q", w.Code, w.Body.String())
	}
}

func hardeningRequest(app http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestGroupHelpersPreserveAuthentication(t *testing.T) {
	for _, helper := range []string{"Mount", "StaticFS", "FileFS"} {
		t.Run(helper, func(t *testing.T) {
			app := zinc.New()
			group := app.Group("/private", func(c *zinc.Context) error { return zinc.ErrUnauthorized })
			files := fstest.MapFS{"secret.txt": &fstest.MapFile{Data: []byte("secret")}}
			switch helper {
			case "Mount":
				group.Mount("/files", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "secret") }))
			case "StaticFS":
				if err := group.StaticFS("/files", files); err != nil {
					t.Fatal(err)
				}
			case "FileFS":
				if err := group.FileFS("/files/secret.txt", "secret.txt", files); err != nil {
					t.Fatal(err)
				}
			}
			w := hardeningRequest(app, "GET", "/private/files/secret.txt")
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("authentication bypass: got %d %q", w.Code, w.Body.String())
			}
		})
	}
}

func TestStaticUsesSamePathAsPrefixAuthentication(t *testing.T) {
	app := zinc.New()
	app.UsePrefix("/files/private", func(c *zinc.Context) error { return zinc.ErrUnauthorized })
	if err := app.StaticFS("/files", fstest.MapFS{"private/secret.txt": &fstest.MapFile{Data: []byte("secret")}}); err != nil {
		t.Fatal(err)
	}
	if w := hardeningRequest(app, "GET", "/files/private/secret.txt"); w.Code != 401 {
		t.Fatalf("baseline: %d", w.Code)
	}
	for _, target := range []string{"/files/private%252Fsecret.txt", "/files/private%5Csecret.txt", "/files/./private/secret.txt"} {
		w := hardeningRequest(app, "GET", target)
		if w.Code == 200 && w.Body.String() == "secret" {
			t.Errorf("path/auth mismatch for %s: %d %q", target, w.Code, w.Body.String())
		}
	}
}

func TestPrefixAuthenticationUsesRewrittenPath(t *testing.T) {
	app := zinc.New()
	app.Use(middleware.Rewrite("/public-alias", "/private/secret"))
	app.UsePrefix("/private", func(c *zinc.Context) error { return zinc.ErrUnauthorized })
	app.Get("/private/secret", func(c *zinc.Context) error { return c.String("secret") })
	w := hardeningRequest(app, "GET", "/public-alias")
	if w.Code != 401 {
		t.Fatalf("rewrite skipped prefix authentication: %d %q", w.Code, w.Body.String())
	}
}

func TestGroupPreservesStrictTrailingSlash(t *testing.T) {
	cfg := zinc.DefaultConfig
	cfg.StrictRouting = true
	app := zinc.NewWithConfig(cfg)
	app.Group("/api").Get("/items/", func(c *zinc.Context) error { return c.String("ok") })
	w := hardeningRequest(app, "GET", "/api/items/")
	if w.Code != 200 {
		t.Fatalf("registered trailing slash lost: %d", w.Code)
	}
}
