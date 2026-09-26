// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestProxySanitizesBeforeDirector(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Target: "http://upstream.invalid", Director: func(r *http.Request) { r.Header.Set("X-Verified-User", "trusted") }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Forwarded-For") != "203.0.113.9" || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Verified-User") != "trusted" || r.Header.Get("X-Forwarded-Proto") != "http" {
			t.Errorf("unsafe headers: %v", r.Header)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}))
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:1234"
	r.Header.Set("Forwarded", "for=attacker")
	r.Header.Set("X-Forwarded-For", "attacker")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("Connection", "X-Verified-User")
	app.ServeHTTP(httptest.NewRecorder(), r)
}

func TestProxyRetryPolicy(t *testing.T) {
	for _, tt := range []struct {
		method          string
		optIn, noReplay bool
		want            int
	}{{"GET", false, false, 3}, {"POST", false, false, 1}, {"PATCH", false, false, 1}, {"POST", true, false, 3}, {"PUT", false, true, 1}} {
		t.Run(tt.method+map[bool]string{true: "-optin", false: ""}[tt.optIn], func(t *testing.T) {
			attempts := 0
			tr := &proxyRetryTransport{retries: 2, base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				if r.Body != nil {
					io.Copy(io.Discard, r.Body)
					r.Body.Close()
				}
				return nil, errors.New("failed")
			})}
			if tt.optIn {
				tr.filter = func(*zinc.Context, error) bool { return true }
			}
			r, _ := http.NewRequest(tt.method, "http://upstream.invalid", strings.NewReader("body"))
			if tt.noReplay {
				r.GetBody = nil
			}
			tr.RoundTrip(r)
			if attempts != tt.want {
				t.Fatalf("attempts=%d want %d", attempts, tt.want)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tr.RoundTrip(r.WithContext(ctx))
			if attempts != tt.want {
				t.Fatal("retried canceled request")
			}
		})
	}
}
