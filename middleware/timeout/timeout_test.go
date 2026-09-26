// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package timeout

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc/middleware/internal/testutil"

	"github.com/0mjs/zinc"
)

func TestContextTimeoutSuccessfulRequest(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Timeout: 100 * time.Millisecond}))
	app.Get("/ok", func(c *zinc.Context) error {
		info := MustGet(c)
		if info.Timeout != 100*time.Millisecond {
			t.Fatalf("timeout=%s", info.Timeout)
		}
		if info.Deadline.IsZero() {
			t.Fatal("deadline should be set")
		}
		if remaining, ok := Remaining(c); !ok || remaining <= 0 {
			t.Fatalf("remaining=%s ok=%t", remaining, ok)
		}
		if c.Context() == context.Background() {
			t.Fatal("request context should be replaced")
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok?x=1", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestContextTimeoutDefaultErrorHandler(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Timeout: 20 * time.Millisecond}))
	app.Get("/slow", func(c *zinc.Context) error {
		return sleepWithContext(c.Context(), 60*time.Millisecond)
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != testutil.JSONErrorBody(http.StatusServiceUnavailable, "Service Unavailable") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestContextTimeoutCustomErrorHandler(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Timeout: 20 * time.Millisecond,
		ErrorHandler: func(c *zinc.Context, err *Error) error {
			if err.Info.Timeout != 20*time.Millisecond {
				t.Fatalf("timeout=%s", err.Info.Timeout)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error=%v", err)
			}
			return zinc.NewError(http.StatusGatewayTimeout, "deadline hit")
		},
	}))
	app.Get("/slow", func(c *zinc.Context) error {
		return sleepWithContext(c.Context(), 60*time.Millisecond)
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != testutil.JSONErrorBody(http.StatusGatewayTimeout, "deadline hit") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestContextTimeoutPassesThroughNonTimeoutErrors(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Timeout: 50 * time.Millisecond}))
	app.Get("/boom", func(c *zinc.Context) error {
		return zinc.ErrForbidden
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestContextTimeoutPreservesRequestData(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Timeout: time.Second}))
	app.Post("/submit", func(c *zinc.Context) error {
		cookie, err := c.Cookie("session")
		if err != nil {
			return err
		}
		if cookie.Value != "abc" {
			t.Fatalf("cookie=%q", cookie.Value)
		}
		if c.Query("query") != "value" {
			t.Fatalf("query=%q", c.Query("query"))
		}
		if got := c.FormValue("name"); got != "matt" {
			t.Fatalf("form=%q", got)
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/submit?query=value", strings.NewReader("name=matt"))
	req.Header.Set(zinc.HeaderContentType, "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestTimeoutErrorMatching(t *testing.T) {
	timeoutErr := &Error{
		Info:  Info{Timeout: 50 * time.Millisecond},
		Cause: context.DeadlineExceeded,
	}

	if !errors.Is(timeoutErr, ErrExceeded) {
		t.Fatal("expected match on ErrExceeded")
	}
	if !errors.Is(timeoutErr, context.DeadlineExceeded) {
		t.Fatal("expected match on context deadline exceeded")
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return context.DeadlineExceeded
	case <-timer.C:
		return nil
	}
}
