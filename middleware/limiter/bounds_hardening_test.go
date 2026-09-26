// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package limiter

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0mjs/zinc"
)

func TestRateLimiterBoundedAdmissionAndExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	app := zinc.New()
	app.Use(New(Config{Rate: 0.1, Capacity: 1, MaxKeys: 2, MaxKeyBytes: 4, IdleTTL: time.Second, Now: func() time.Time { return now }, LimitReached: func(*zinc.Context) error { return zinc.ServiceUnavailable("busy") }, Key: func(c *zinc.Context) string { return c.Query("key") }}))
	app.Get("/", func(c *zinc.Context) error { return c.Send("ok") })
	request := func(key string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", "/?key="+key, nil))
		if w.Code != want {
			t.Fatalf("key %q: got %d want %d", key, w.Code, want)
		}
	}
	request("a", 200)
	request("b", 200)
	for i := 0; i < 100; i++ {
		request(fmt.Sprint(i), 503)
	}
	now = now.Add(2 * time.Second)
	request("c", 503) // Idle alone must not reset a depleted quota.
	request("a", 503)
	now = now.Add(11 * time.Second)
	request("c", 200)
	request("large", 503)
	request("a", 200)
	request("a", 503)
}
