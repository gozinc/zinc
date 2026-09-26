// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package limiter

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/testutil"
)

func TestRateLimiter(t *testing.T) {
	// Create a new app
	app := zinc.New()

	// Add rate limiter middleware
	app.Use(New(Config{
		Rate:     2, // 2 requests per second
		Capacity: 2, // Max burst of 2 requests
	}))

	// Add a test route
	app.Get("/test", func(c *zinc.Context) error {
		return c.Send("OK")
	})

	// Make 3 requests - first 2 should succeed, 3rd should fail
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		if i < 2 {
			// First 2 requests should succeed
			if resp.Code != http.StatusOK {
				t.Errorf("Expected status code %d but got %d on request %d", http.StatusOK, resp.Code, i+1)
			}
		} else {
			// 3rd request should fail with 429
			if resp.Code != http.StatusTooManyRequests {
				t.Errorf("Expected status code %d but got %d on request %d", http.StatusTooManyRequests, resp.Code, i+1)
			}
		}
	}

	// Wait for tokens to replenish
	time.Sleep(1200 * time.Millisecond) // Wait just over 1 second for 2 tokens (at rate of 2/sec)

	// Make another request, should succeed now
	req := httptest.NewRequest("GET", "/test", nil)
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("Expected status code %d but got %d after waiting", http.StatusOK, resp.Code)
	}
}

func TestKeyedRateLimiter(t *testing.T) {
	// Create a new app
	app := zinc.New()

	// Add IP rate limiter middleware
	app.Use(New(Config{Rate: 2, Capacity: 2, Key: (*zinc.Context).IP}))

	// Add a test route
	app.Get("/test", func(c *zinc.Context) error {
		return c.Send("OK")
	})

	// Make 3 requests with same IP - first 2 should succeed, 3rd should fail
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345" // Set consistent remote IP
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		if i < 2 {
			// First 2 requests should succeed
			if resp.Code != http.StatusOK {
				t.Errorf("Expected status code %d but got %d on request %d", http.StatusOK, resp.Code, i+1)
			}
		} else {
			// 3rd request should fail with 429
			if resp.Code != http.StatusTooManyRequests {
				t.Errorf("Expected status code %d but got %d on request %d", http.StatusTooManyRequests, resp.Code, i+1)
			}
		}
	}

	// Make a request with different IP - should succeed
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.2:12345" // Different IP
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Errorf("Expected status code %d but got %d for different IP", http.StatusOK, resp.Code)
	}
}

func TestDefaultLimitReached(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Get("/default", func(c *zinc.Context) error {
		return c.Send("OK")
	})

	var limited *httptest.ResponseRecorder
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest("GET", "/default", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)
		if resp.Code == http.StatusTooManyRequests {
			limited = resp
			break
		}
	}
	if limited == nil {
		t.Fatal("expected at least one request to hit default rate limit")
	}
	if body := limited.Body.String(); body != testutil.JSONErrorBody(http.StatusTooManyRequests, "rate limit exceeded") {
		t.Fatalf("body=%q", body)
	}
}

func TestCustomKey(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Rate:     1,
		Capacity: 1,
		Key: func(c *zinc.Context) string {
			return c.Header("X-Key")
		},
	}))
	app.Get("/keyed", func(c *zinc.Context) error {
		return c.Send("OK")
	})

	reqA1 := httptest.NewRequest("GET", "/keyed", nil)
	reqA1.Header.Set("X-Key", "A")
	resA1 := httptest.NewRecorder()
	app.ServeHTTP(resA1, reqA1)
	if resA1.Code != http.StatusOK {
		t.Fatalf("status=%d", resA1.Code)
	}

	reqA2 := httptest.NewRequest("GET", "/keyed", nil)
	reqA2.Header.Set("X-Key", "A")
	resA2 := httptest.NewRecorder()
	app.ServeHTTP(resA2, reqA2)
	if resA2.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d", resA2.Code)
	}

	reqB := httptest.NewRequest("GET", "/keyed", nil)
	reqB.Header.Set("X-Key", "B")
	resB := httptest.NewRecorder()
	app.ServeHTTP(resB, reqB)
	if resB.Code != http.StatusOK {
		t.Fatalf("status=%d", resB.Code)
	}
}

func TestConcurrency(t *testing.T) {
	app := zinc.New()
	app.Use(Concurrency(1))
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
