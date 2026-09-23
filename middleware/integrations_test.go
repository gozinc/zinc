// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/0mjs/zinc"
)

type casbinStub struct {
	allow bool
	args  []any
}

func (s *casbinStub) Enforce(args ...any) (bool, error) {
	s.args = append([]any(nil), args...)
	return s.allow, nil
}

func TestCasbinAuthAllowsRequest(t *testing.T) {
	enforcer := &casbinStub{allow: true}
	app := zinc.New()
	app.Use(CasbinAuth(enforcer, func(*zinc.Context) any {
		return "alice"
	}))
	app.Get("/docs", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(enforcer.args) != 3 || enforcer.args[0] != "alice" || enforcer.args[1] != "/docs" || enforcer.args[2] != http.MethodGet {
		t.Fatalf("args=%v", enforcer.args)
	}
}

func TestCasbinAuthRejectsRequest(t *testing.T) {
	app := zinc.New()
	app.Use(CasbinAuth(&casbinStub{allow: false}, func(*zinc.Context) any {
		return "alice"
	}))
	app.Get("/docs", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestPrometheusRecordsRequestsAndServesMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()
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
	app.Use(PrometheusWithConfig(PrometheusConfig{Metrics: metrics, Now: now}))
	app.Get("/users/{id}", func(c *zinc.Context) error {
		return c.String("ok")
	})
	app.Get("/metrics", PrometheusHandler(metrics))

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

func TestProxyForwardsRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "upstream:"+r.URL.Path)
	}))
	defer upstream.Close()

	app := zinc.New()
	app.Use(Proxy(upstream.URL))

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "upstream:/api/users" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestStaticServesFilesAndFallsThrough(t *testing.T) {
	fsys := fstest.MapFS{
		"hello.txt": &fstest.MapFile{Data: []byte("hello")},
	}

	app := zinc.New()
	app.Use(StaticWithConfig(StaticConfig{
		Filesystem:     fsys,
		Prefix:         "/assets",
		NextOnNotFound: true,
	}))
	app.Get("/assets/missing.txt", func(c *zinc.Context) error {
		return c.String("fallback")
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/hello.txt", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "hello" {
		t.Fatalf("body=%q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/missing.txt", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "fallback" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestSessionPersistsSignedCookie(t *testing.T) {
	app := zinc.New()
	app.Use(SessionCookie("sid", "secret"))
	app.Get("/", func(c *zinc.Context) error {
		session := MustSession(c)
		visits := session.Get("visits")
		if visits == "" {
			visits = "1"
		} else {
			visits = "2"
		}
		session.Set("visits", visits)
		return c.String(visits)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "1" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "sid" {
		t.Fatalf("cookies=%v", cookies)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "2" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestSessionRejectsInvalidCookie(t *testing.T) {
	app := zinc.New()
	app.Use(SessionCookie("sid", "secret"))
	app.Get("/", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: "bad"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestJaegerPropagatesUberTraceID(t *testing.T) {
	var observed JaegerSpan
	app := zinc.New()
	app.Use(Jaeger(func(_ *zinc.Context, span JaegerSpan) error {
		observed = span
		return nil
	}))
	app.Get("/trace", func(c *zinc.Context) error {
		span, ok := JaegerCurrent(c)
		if !ok {
			t.Fatal("missing jaeger span")
		}
		return c.String(span.TraceID)
	})

	req := httptest.NewRequest(http.MethodGet, "/trace", nil)
	req.Header.Set(HeaderUberTraceID, "abc123:def456:0:1")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "abc123" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if observed.TraceID != "abc123" || observed.ParentSpanID != "def456" {
		t.Fatalf("span=%+v", observed)
	}
	if got := rec.Header().Get(HeaderUberTraceID); !strings.HasPrefix(got, "abc123:") {
		t.Fatalf("trace header=%q", got)
	}
}

func mustNoErrIntegrations(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
