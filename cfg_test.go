// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A Config literal that sets one field must keep every other default. In 0.3
// it silently disabled HEAD, OPTIONS, 405 responses, and the route cache.
func TestZeroValueConfigKeepsDefaults(t *testing.T) {
	for name, app := range map[string]*App{
		"New()":                New(),
		"New(Config{})":        New(Config{}),
		"New(partial literal)": New(Config{ServerHeader: "z"}),
	} {
		t.Run(name, func(t *testing.T) {
			app.Get("/x", func(c *Context) error { return c.String("x") })
			for _, tc := range []struct {
				method string
				status int
				allow  string
			}{
				{http.MethodHead, http.StatusOK, ""},
				{http.MethodOptions, http.StatusNoContent, "GET, HEAD, OPTIONS"},
				{http.MethodPost, http.StatusMethodNotAllowed, "GET, HEAD, OPTIONS"},
			} {
				rec := httptest.NewRecorder()
				app.ServeHTTP(rec, httptest.NewRequest(tc.method, "/x", nil))
				if rec.Code != tc.status || rec.Header().Get(HeaderAllow) != tc.allow {
					t.Errorf("%s: %d allow=%q, want %d allow=%q", tc.method, rec.Code, rec.Header().Get(HeaderAllow), tc.status, tc.allow)
				}
			}
			if app.router.cache == nil {
				t.Error("route cache disabled by a zero RouteCacheSize")
			}
			cfg := app.config
			if cfg.BodyLimit != DefaultBodyLimit || cfg.ReadTimeout != DefaultReadTimeout ||
				cfg.WriteTimeout != DefaultWriteTimeout || cfg.IdleTimeout != DefaultIdleTimeout ||
				cfg.ShutdownTimeout != DefaultShutdownTimeout || cfg.RouteCacheSize != DefaultRouteCacheSize ||
				cfg.ProxyHeader != DefaultProxyHeader {
				t.Errorf("defaults not applied: %+v", cfg)
			}
		})
	}
}

func TestDisableFlagsAndNegativeLimits(t *testing.T) {
	app := New(Config{
		DisableAutoHead:         true,
		DisableAutoOptions:      true,
		DisableMethodNotAllowed: true,
		RouteCacheSize:          -1,
		BodyLimit:               -1,
		ReadTimeout:             -1,
		WriteTimeout:            -1,
		IdleTimeout:             -1,
	})
	app.Get("/x", func(c *Context) error { return c.String("x") })
	for _, method := range []string{http.MethodHead, http.MethodOptions, http.MethodPost} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(method, "/x", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s with auto features disabled: %d, want 404", method, rec.Code)
		}
	}
	if app.router.cache != nil {
		t.Error("RouteCacheSize -1 should disable the cache")
	}

	srv := app.newServer()
	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 || srv.IdleTimeout != 0 {
		t.Errorf("negative timeouts should mean none: %v %v %v", srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout)
	}

	// BodyLimit -1 lifts the limit, so a body over the default binds.
	large := bytes.Repeat([]byte("a"), int(DefaultBodyLimit)+1)
	app.Post("/body", func(c *Context) error {
		body, err := c.BodyBytes()
		if err != nil {
			return err
		}
		return c.String(strconv.Itoa(len(body)))
	})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/body", bytes.NewReader(large)))
	if rec.Code != http.StatusOK || rec.Body.String() != strconv.Itoa(len(large)) {
		t.Fatalf("unlimited body: %d %q", rec.Code, rec.Body.String())
	}
}

func TestNewRejectsMoreThanOneConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New with two Configs did not panic")
		}
	}()
	New(Config{}, Config{})
}

// startListenContext serves app on a free port until the returned cancel is
// called, and reports ListenContext's result on done.
func startListenContext(t *testing.T, app *App) (addr string, cancel context.CancelFunc, done chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done = make(chan error, 1)
	go func() { done <- app.serveContext(ctx, ln) }()
	return "http://" + ln.Addr().String(), cancel, done
}

func TestListenContextDrainsInFlightRequests(t *testing.T) {
	app := New()
	started := make(chan struct{})
	app.Get("/slow", func(c *Context) error {
		close(started)
		time.Sleep(200 * time.Millisecond)
		return c.String("finished")
	})
	addr, cancel, done := startListenContext(t, app)

	result := make(chan string, 1)
	go func() {
		resp, err := http.Get(addr + "/slow")
		if err != nil {
			result <- "error: " + err.Error()
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		result <- string(body)
	}()
	<-started
	cancel()

	if got := <-result; got != "finished" {
		t.Fatalf("in-flight request: %q", got)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListenContext after a clean drain: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenContext did not return after shutdown")
	}
	if _, err := http.Get(addr + "/slow"); err == nil {
		t.Fatal("server still accepting after shutdown")
	}
}

func TestListenContextShutdownTimeoutClosesConnections(t *testing.T) {
	app := New(Config{ShutdownTimeout: 50 * time.Millisecond})
	started := make(chan struct{})
	release := make(chan struct{})
	app.Get("/stuck", func(c *Context) error {
		close(started)
		<-release
		return nil
	})
	defer close(release)
	addr, cancel, done := startListenContext(t, app)
	go func() { _, _ = http.Get(addr + "/stuck") }()
	<-started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "graceful shutdown") {
			t.Fatalf("err = %v, want a graceful-shutdown deadline error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenContext ignored ShutdownTimeout")
	}
}

func TestListenContextReportsListenErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := New().ListenContext(context.Background(), ln.Addr().String()); err == nil {
		t.Fatal("expected an error for an address in use")
	}
}
