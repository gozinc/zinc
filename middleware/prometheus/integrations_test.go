// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package prometheus

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
)

func TestPrometheusRecordsRequestsAndServesMetrics(t *testing.T) {
	metrics := NewMetrics(0)
	times := []time.Time{
		time.Unix(10, 0),
		time.Unix(10, int64(50*time.Millisecond)),
	}
	now := func() time.Time {
		next := times[0]
		if len(times) > 1 {
			times = times[1:]
		}
		return next
	}

	app := zinc.New()
	app.Use(New(Config{Metrics: metrics, Now: now}))
	app.Get("/users/{id}", func(c *zinc.Context) error {
		return c.String("ok")
	})
	app.Get("/metrics", Handler(metrics))

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `zinc_http_requests_total{method="GET",route="/users/{id}",status="200"} 1`) {
		t.Fatalf("metrics body=%s", body)
	}
}
