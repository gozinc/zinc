// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package prometheus

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestPrometheusConvenienceMiddleware(t *testing.T) {
	metrics := NewMetrics(0)
	app := zinc.New()
	app.Use(New(Config{Metrics: metrics}))
	app.Get("/ok", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	text := metrics.Text()
	if !strings.Contains(text, `method="GET"`) || !strings.Contains(text, `route="/ok"`) || !strings.Contains(text, `status="200"`) {
		t.Fatalf("metrics=%s", text)
	}
}

func TestPrometheusDefaultAndEscaping(t *testing.T) {
	metrics := NewMetrics(0)
	metrics.Observe("GE\"T", "/line\n\\route", 200, 0)
	text := metrics.Text()
	if !strings.Contains(text, `method="GE\"T"`) {
		t.Fatalf("metrics missing escaped quote: %s", text)
	}
	if !strings.Contains(text, `route="/line\n\\route"`) {
		t.Fatalf("metrics missing escaped route: %s", text)
	}
	var nilMetrics *Metrics
	if nilMetrics.Text() != "" {
		t.Fatal("nil metrics should render empty text")
	}
}
