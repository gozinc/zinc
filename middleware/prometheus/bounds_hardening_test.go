// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package prometheus

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
)

func TestPrometheusBoundsAndIsolation(t *testing.T) {
	a, b := zinc.New(), zinc.New()
	for _, app := range []*zinc.App{a, b} {
		app.Use(New())
		app.Get("/metrics", Handler())
	}
	for i := 0; i < 1000; i++ {
		a.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(fmt.Sprintf("METHOD%d", i), fmt.Sprintf("/missing/%d", i), nil))
	}
	scrape := func(app *zinc.App) string {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		return w.Body.String()
	}
	text := scrape(a)
	if !strings.Contains(text, `{method="OTHER",route="unmatched",status="404"} 1000`) || strings.Contains(text, "/missing/") {
		t.Fatal(text)
	}
	if strings.Contains(scrape(b), `route="unmatched"`) {
		t.Fatal("default registry leaked between apps")
	}
	m := NewMetrics(2)
	for i := 0; i < 10; i++ {
		m.Observe("GET", fmt.Sprint(i), 200, time.Second)
	}
	if len(m.requests) != 2 || !strings.Contains(m.Text(), "zinc_http_metrics_dropped_total 8") {
		t.Fatal(m.Text())
	}
}
