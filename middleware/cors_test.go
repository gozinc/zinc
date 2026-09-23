// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0mjs/zinc"
)

func TestCORS(t *testing.T) {
	// Helper function to create a test request
	createRequest := func(method, origin string, headers map[string]string) *http.Request {
		req := httptest.NewRequest(method, "http://example.com", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req
	}

	// Helper function to run a test
	runTest := func(middleware zinc.Middleware, req *http.Request, handler func(*zinc.Context) error) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c := zinc.NewContext(w, req)

		// Create a simple app for the test
		app := zinc.New()
		c.Set("app", app)

		// Manually run middleware then handler
		if err := middleware(c); err != nil {
			t.Fatalf("Middleware returned an error: %v", err)
		}

		// Only call handler if response hasn't been written
		if w.Code == http.StatusOK && w.Body.Len() == 0 {
			if err := handler(c); err != nil {
				t.Fatalf("Handler returned an error: %v", err)
			}
		}

		return w
	}

	// Simple handler that just responds with OK
	okHandler := func(c *zinc.Context) error {
		return c.Send("OK")
	}

	t.Run("Default CORS Configuration", func(t *testing.T) {
		cors := CORS()

		t.Run("Simple Request", func(t *testing.T) {
			// Test request from allowed origin
			req := createRequest(http.MethodGet, "http://example.org", nil)
			res := runTest(cors, req, okHandler)

			// Check response
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Allow-Origin header = %v, want %v", got, "*")
			}
			if res.Body.String() != "OK" {
				t.Errorf("Response body = %v, want %v", res.Body.String(), "OK")
			}
		})

		t.Run("Preflight Request", func(t *testing.T) {
			// Test preflight request
			req := createRequest(http.MethodOptions, "http://example.org", map[string]string{
				"Access-Control-Request-Method": http.MethodPost,
			})
			res := runTest(cors, req, okHandler)

			// Check preflight response
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Allow-Origin header = %v, want %v", got, "*")
			}
			if got := res.Header().Get("Access-Control-Allow-Methods"); got == "" {
				t.Errorf("Allow-Methods header is empty")
			}
			if res.Code != http.StatusNoContent {
				t.Errorf("Response code = %v, want %v", res.Code, http.StatusNoContent)
			}
		})

		t.Run("Non-CORS Request", func(t *testing.T) {
			// Test request without origin
			req := createRequest(http.MethodGet, "", nil)
			res := runTest(cors, req, okHandler)

			// Check that CORS headers are not set
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("Allow-Origin header = %v, want empty", got)
			}
			if res.Body.String() != "OK" {
				t.Errorf("Response body = %v, want %v", res.Body.String(), "OK")
			}
		})
	})

	t.Run("Custom CORS Configuration", func(t *testing.T) {
		cors := CORSWithOptions(
			CORSAllowOrigins("http://example.org"),
			CORSAllowCredentials(true),
			CORSMaxAgeSeconds(86400), // 24 hours in seconds
			CORSExposeHeaders("X-Custom-Header"),
		)

		t.Run("Allowed Origin", func(t *testing.T) {
			// Test request from allowed origin
			req := createRequest(http.MethodGet, "http://example.org", nil)
			res := runTest(cors, req, okHandler)

			// Check response
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != "http://example.org" {
				t.Errorf("Allow-Origin header = %v, want %v", got, "http://example.org")
			}
			if got := res.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Errorf("Allow-Credentials header = %v, want %v", got, "true")
			}
			if got := res.Header().Get("Access-Control-Expose-Headers"); got != "X-Custom-Header" {
				t.Errorf("Expose-Headers header = %v, want %v", got, "X-Custom-Header")
			}
			if res.Body.String() != "OK" {
				t.Errorf("Response body = %v, want %v", res.Body.String(), "OK")
			}
		})

		t.Run("Disallowed Origin", func(t *testing.T) {
			// Test request from disallowed origin
			req := createRequest(http.MethodGet, "http://example.com", nil)
			res := runTest(cors, req, okHandler)

			// Check that CORS headers are not set for disallowed origin
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("Allow-Origin header = %v, want empty", got)
			}
			if res.Body.String() != "OK" {
				t.Errorf("Response body = %v, want %v", res.Body.String(), "OK")
			}
		})

		t.Run("Preflight with MaxAge", func(t *testing.T) {
			// Test preflight request with max age
			req := createRequest(http.MethodOptions, "http://example.org", map[string]string{
				"Access-Control-Request-Method":  http.MethodPost,
				"Access-Control-Request-Headers": "Content-Type",
			})
			res := runTest(cors, req, okHandler)

			// Print headers for debugging
			t.Logf("All response headers: %v", res.Header())
			t.Logf("Response code: %d", res.Code)
			t.Logf("MaxAge config: %d seconds", 24*60*60)

			// Directly inspect if the header was set correctly
			maxAgeHeader := res.Header().Get("Access-Control-Max-Age")
			t.Logf("Access-Control-Max-Age header: '%s'", maxAgeHeader)

			// Check max age header
			if maxAgeHeader != "86400" {
				t.Errorf("Max-Age header = '%s', want '86400'", maxAgeHeader)
			}
		})
	})

	t.Run("With Skipper", func(t *testing.T) {
		// Create CORS middleware with skipper that skips all requests
		cors := CORSWithOptions(
			CORSSkipper(func(c *zinc.Context) bool {
				return true
			}),
		)

		// Test request
		req := createRequest(http.MethodGet, "http://example.org", nil)
		res := runTest(cors, req, okHandler)

		// Check that CORS headers are not set because skipper skipped the middleware
		if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Allow-Origin header = %v, want empty", got)
		}
		if res.Body.String() != "OK" {
			t.Errorf("Response body = %v, want %v", res.Body.String(), "OK")
		}
	})
}

func TestOptionHelpersMutateConfig(t *testing.T) {
	cfg := DefaultCORSConfig()

	CORSAllowMethods(http.MethodPatch, http.MethodOptions)(&cfg)
	if len(cfg.AllowMethods) != 2 || cfg.AllowMethods[0] != http.MethodPatch || cfg.AllowMethods[1] != http.MethodOptions {
		t.Fatalf("allow methods=%v", cfg.AllowMethods)
	}

	CORSAllowHeaders("X-Trace", "X-Auth")(&cfg)
	if len(cfg.AllowHeaders) != 2 || cfg.AllowHeaders[0] != "X-Trace" || cfg.AllowHeaders[1] != "X-Auth" {
		t.Fatalf("allow headers=%v", cfg.AllowHeaders)
	}

	CORSMaxAge(90 * time.Second)(&cfg)
	if cfg.MaxAge != 90 {
		t.Fatalf("max age=%d", cfg.MaxAge)
	}
}
