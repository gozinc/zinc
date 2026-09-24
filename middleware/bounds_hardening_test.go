package middleware

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
)

func TestRateLimiterBoundedAdmissionAndExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	app := zinc.New()
	app.Use(RateLimiter(RateLimiterConfig{Rate: 0.1, Capacity: 1, MaxKeys: 2, MaxKeyBytes: 4, IdleTTL: time.Second, Now: func() time.Time { return now }, StatusCode: 503, KeyGenerator: func(c *zinc.Context) string { return c.Query("key") }}))
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

func TestPrometheusBoundsAndIsolation(t *testing.T) {
	a, b := zinc.New(), zinc.New()
	for _, app := range []*zinc.App{a, b} {
		app.Use(PrometheusWithConfig(PrometheusConfig{Skipper: func(c *zinc.Context) bool { return c.Path() == "/metrics" }}))
		app.Get("/metrics", PrometheusHandler())
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
	m := NewPrometheusMetrics(2)
	for i := 0; i < 10; i++ {
		m.Observe("GET", fmt.Sprint(i), 200, time.Second)
	}
	if len(m.requests) != 2 || !strings.Contains(m.Text(), "zinc_http_metrics_dropped_total 8") {
		t.Fatal(m.Text())
	}
}

func TestCORSPartialAndCredentialPolicy(t *testing.T) {
	for _, cfg := range []CORSConfig{{}, {AllowCredentials: true}} {
		app := zinc.New()
		app.Use(CORSWithConfig(cfg))
		app.Get("/", func(c *zinc.Context) error { return c.Send("ok") })
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		want := "*"
		if cfg.AllowCredentials {
			want = ""
		}
		if w.Header().Get("Access-Control-Allow-Origin") != want {
			t.Fatal(w.Header())
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("wildcard credentials must be rejected")
		}
	}()
	CORSWithConfig(CORSConfig{AllowOrigins: []string{"*"}, AllowCredentials: true})
}
