// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/0mjs/zinc"
)

func TestUtilityMiddleware(t *testing.T) {
	t.Run("no cache and set header", func(t *testing.T) {
		app := zinc.New()
		app.Use(NoCache(), SetHeader("X-App", "zinc"))
		app.Get("/", func(c *zinc.Context) error {
			return c.String("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if got := rec.Header().Get(zinc.HeaderCacheControl); got == "" {
			t.Fatal("cache-control header missing")
		}
		if got := rec.Header().Get(zinc.HeaderPragma); got != "no-cache" {
			t.Fatalf("pragma=%q", got)
		}
		if got := rec.Header().Get("X-App"); got != "zinc" {
			t.Fatalf("x-app=%q", got)
		}
	})

	t.Run("heartbeat", func(t *testing.T) {
		app := zinc.New()
		app.Use(Heartbeat("/healthz"))
		app.Get("/healthz", func(c *zinc.Context) error {
			return c.String("handler")
		})

		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("heartbeat status=%d body=%q", rec.Code, rec.Body.String())
		}
	})

	t.Run("maybe", func(t *testing.T) {
		app := zinc.New()
		app.Use(Maybe(func(c *zinc.Context) bool {
			return c.GetHeader("X-Guard") == "yes"
		}, func(c *zinc.Context) error {
			c.SetHeader("X-Guarded", "yes")
			return c.Next()
		}))
		app.Get("/", func(c *zinc.Context) error {
			return c.String("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Guard", "yes")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Guarded"); got != "yes" {
			t.Fatalf("guarded=%q", got)
		}

		req = httptest.NewRequest(http.MethodGet, "/", nil)
		rec = httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Guarded"); got != "" {
			t.Fatalf("unguarded=%q", got)
		}
	})
}

func TestThrottleMiddleware(t *testing.T) {
	app := zinc.New()
	app.Use(Throttle(1))
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	app.Get("/slow", func(c *zinc.Context) error {
		once.Do(func() { close(started) })
		<-release
		return c.String("ok")
	})

	done := make(chan struct{})
	first := httptest.NewRecorder()
	go func() {
		defer close(done)
		app.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/slow", nil))
	}()
	<-started

	second := httptest.NewRecorder()
	app.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status=%d body=%q", second.Code, second.Body.String())
	}

	close(release)
	<-done
	if first.Code != http.StatusOK || first.Body.String() != "ok" {
		t.Fatalf("first status=%d body=%q", first.Code, first.Body.String())
	}
}

func TestRealIPMiddleware(t *testing.T) {
	app := zinc.NewWithConfig(zinc.Config{
		ProxyHeader:    zinc.HeaderXForwardedFor,
		TrustedProxies: []string{"10.0.0.1"},
	})
	app.Use(RealIP())
	app.Get("/", func(c *zinc.Context) error {
		return c.String(c.RemoteIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set(zinc.HeaderXForwardedFor, "203.0.113.9")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "203.0.113.9" {
		t.Fatalf("remote ip=%q", rec.Body.String())
	}
}

func TestHeaderGuardMiddleware(t *testing.T) {
	app := zinc.New()
	app.Use(AllowContentType("application/json"), AllowContentEncoding("identity"), RouteHeaders(HeaderRoute{
		Header: "X-Mode",
		Value:  "blocked",
		Middleware: func(c *zinc.Context) error {
			return zinc.ErrForbidden
		},
	}))
	app.Post("/", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(zinc.HeaderContentType, "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(zinc.HeaderContentType, "application/json")
	req.Header.Set("X-Mode", "blocked")
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("route header status=%d", rec.Code)
	}
}

func TestPprofMiddleware(t *testing.T) {
	app := zinc.New()
	app.Use(PprofWithPrefix("/debug/pprof"))
	app.Get("/other", func(c *zinc.Context) error {
		return c.String("next")
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("pprof status=%d body len=%d", rec.Code, rec.Body.Len())
	}

	req = httptest.NewRequest(http.MethodGet, "/other", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "next" {
		t.Fatalf("fallthrough body=%q", rec.Body.String())
	}
}

func TestPprofDefaultAndPrefixNormalization(t *testing.T) {
	app := zinc.New()
	app.Use(Pprof())

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("default cmdline status=%d", rec.Code)
	}

	app = zinc.New()
	app.Use(PprofWithPrefix("custom/pprof/"))
	app.Get("/next", func(c *zinc.Context) error {
		return c.String("next")
	})

	req = httptest.NewRequest(http.MethodGet, "/custom/pprof/symbol", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("symbol status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/custom/pprof/heap", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("heap status=%d body len=%d", rec.Code, rec.Body.Len())
	}

	req = httptest.NewRequest(http.MethodGet, "/next", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "next" {
		t.Fatalf("fallthrough body=%q", rec.Body.String())
	}
}

func mustNoErrUtility(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
