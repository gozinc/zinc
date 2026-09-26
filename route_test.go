// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouteName(t *testing.T) {
	app := New()
	var seen string
	app.Get("/users/{id}", func(c *Context) error { seen = c.Route().Name; return nil }).Name("users.show")
	api := app.Group("/api")
	api.Post("/items", func(c *Context) error { return nil }).Name("items.create")

	if url, err := app.URL("users.show", "42"); err != nil || url != "/users/42" {
		t.Fatalf("URL = %q %v", url, err)
	}
	if info, ok := app.RouteByName("items.create"); !ok || info.Path != "/api/items" || info.Method != MethodPost {
		t.Fatalf("RouteByName = %+v %v", info, ok)
	}
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(MethodGet, "/users/7", nil))
	if seen != "users.show" {
		t.Fatalf("c.Route().Name = %q", seen)
	}

	// Renaming moves the name; the old one no longer resolves.
	route := app.Get("/about", func(c *Context) error { return nil }).Name("about")
	route.Name("about-us")
	if _, ok := app.RouteByName("about"); ok {
		t.Fatal("old name still resolves after renaming")
	}
	if url, _ := app.URL("about-us"); url != "/about" {
		t.Fatalf("renamed URL = %q", url)
	}

	for name, register := range map[string]func(){
		"duplicate": func() { app.Get("/other", func(c *Context) error { return nil }).Name("users.show") },
		"empty":     func() { app.Get("/empty", func(c *Context) error { return nil }).Name("") },
		"zero":      func() { Route{}.Name("x") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s name did not panic", name)
				}
			}()
			register()
		}()
	}

	// TryHandle reports the same mistake as an error.
	err := app.TryHandle(RouteSpec{Name: "users.show", Method: MethodGet, Path: "/dup", Handler: func(c *Context) error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("TryHandle duplicate name: %v", err)
	}
}

func TestStaticRejectsNilFilesystem(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("StaticFS with a nil filesystem did not panic")
		}
	}()
	New().StaticFS("/assets", nil)
}

type bridgeKey struct{}

// statusRecorder wraps a writer the way logging and tracing middleware do.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

type unwrappingRecorder struct{ statusRecorder }

func (w *unwrappingRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestFromHTTP(t *testing.T) {
	requireToken := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Token") == "" {
				http.Error(w, "no token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), bridgeKey{}, "traced")))
		})
	}
	var observed int
	observeStatus := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &unwrappingRecorder{statusRecorder{ResponseWriter: w}}
			next.ServeHTTP(rec, r)
			observed = rec.status
		})
	}

	app := New()
	api := app.Group("/api").UseHTTP(requireToken, observeStatus)
	api.Get("/value", func(c *Context) error {
		value, _ := c.Context().Value(bridgeKey{}).(string)
		return c.Status(http.StatusAccepted).String(value)
	})
	api.Get("/fail", func(c *Context) error { return Conflict("taken") })
	app.Get("/route", FromHTTP(requireToken), func(c *Context) error { return c.String("route") })

	cases := []struct {
		name, target, token string
		status              int
		body                string
	}{
		{"short-circuit", "/api/value", "", 401, "no token\n"},
		{"request replaced", "/api/value", "t", 202, "traced"},
		{"error reaches the error handler", "/api/fail", "t", 409, `{"error":{"status":409,"message":"taken"}}` + "\n"},
		{"single route", "/route", "t", 200, "route"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(MethodGet, tc.target, nil)
		if tc.token != "" {
			req.Header.Set("X-Token", tc.token)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != tc.status || rec.Body.String() != tc.body {
			t.Errorf("%s: got %d %q, want %d %q", tc.name, rec.Code, rec.Body.String(), tc.status, tc.body)
		}
	}
	// The wrapping middleware saw the status written through its wrapper.
	if observed != http.StatusConflict {
		t.Errorf("observed status = %d, want 409 from the error handler", observed)
	}
}

func TestFromHTTPRequiresUnwrap(t *testing.T) {
	hiding := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(&statusRecorder{ResponseWriter: w}, r)
		})
	}
	app := New()
	app.Get("/", FromHTTP(hiding), func(c *Context) error { return nil })
	defer func() {
		msg, _ := recover().(string)
		if !strings.Contains(msg, "Unwrap") {
			t.Fatalf("panic = %q, want a message about Unwrap", msg)
		}
	}()
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(MethodGet, "/", nil))
}

// FromHTTP adds no allocation beyond what the standard middleware itself does.
func TestFromHTTPAllocatesNothing(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are unreliable under -race")
	}
	passThrough := func(next http.Handler) http.Handler { return next }
	native := New()
	native.Get("/", func(c *Context) error { return c.Next() }, func(c *Context) error { return nil })
	bridged := New()
	bridged.Get("/", FromHTTP(passThrough), func(c *Context) error { return nil })

	req := httptest.NewRequest(MethodGet, "/", nil)
	w := &discardWriter{header: http.Header{}}
	measure := func(app *App) float64 {
		return testing.AllocsPerRun(200, func() { app.ServeHTTP(w, req) })
	}
	if n, b := measure(native), measure(bridged); b > n {
		t.Fatalf("FromHTTP allocates %.1f per request, native middleware %.1f", b, n)
	}
}

type discardWriter struct{ header http.Header }

func (w *discardWriter) Header() http.Header         { return w.header }
func (w *discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *discardWriter) WriteHeader(int)             {}
