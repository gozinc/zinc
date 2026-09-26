// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package middleware_test checks middleware packages working together. Each
// package's own behavior is tested in that package.
package middleware_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/basicauth"
	"github.com/0mjs/zinc/middleware/bodydump"
	"github.com/0mjs/zinc/middleware/bodylimit"
	"github.com/0mjs/zinc/middleware/csrf"
	"github.com/0mjs/zinc/middleware/recover"
	"github.com/0mjs/zinc/middleware/timeout"
)

func TestBodyDumpSeesWhatBodyLimitAllows(t *testing.T) {
	app := zinc.New()
	var observed bodydump.Snapshot
	app.Use(bodydump.New(bodydump.Config{Observe: func(_ *zinc.Context, snapshot bodydump.Snapshot) {
		observed = snapshot
	}}))
	app.Use(bodylimit.New(bodylimit.Config{Limit: 4}))
	app.Post("/echo", func(c *zinc.Context) error {
		body, err := c.BodyString()
		if err != nil {
			return err
		}
		return c.String(body)
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("ping")))
	if rec.Code != http.StatusOK || string(observed.RequestBody) != "ping" {
		t.Fatalf("status=%d body=%q observed=%q", rec.Code, rec.Body.String(), observed.RequestBody)
	}

	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("too-large")))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestTimeoutAndCSRFTogether(t *testing.T) {
	app := zinc.New()
	app.Use(timeout.New(timeout.Config{Timeout: 100 * time.Millisecond}))
	app.Use(csrf.New())
	app.Get("/form", func(c *zinc.Context) error {
		if got := timeout.MustGet(c).Timeout; got != 100*time.Millisecond {
			t.Fatalf("timeout=%s", got)
		}
		return c.String(csrf.Token(c))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/form", nil))
	if rec.Code != http.StatusOK || rec.Body.String() == "" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderSetCookie); !strings.Contains(got, "_csrf=") {
		t.Fatalf("set-cookie=%q", got)
	}
}

func TestExtractorsAndErrorValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?csrf=one&csrf=two", nil)
	req.Header.Set("X-Basic", base64.StdEncoding.EncodeToString([]byte("joe:secret")))
	app := zinc.New()
	ctx := app.AcquireContext(httptest.NewRecorder(), req)
	defer app.ReleaseContext(ctx)

	credentials, err := basicauth.FromHeader("X-Basic")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Username != "joe" || credentials.Password != "secret" || credentials.Source != basicauth.SourceHeader {
		t.Fatalf("credentials=%+v", credentials)
	}

	values, err := csrf.FromQuery("csrf")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(values, ",") != "one,two" {
		t.Fatalf("csrf values=%v", values)
	}

	timeoutErr := &timeout.Error{Info: timeout.Info{Timeout: time.Second}, Cause: context.Canceled}
	if timeoutErr.Error() == "" || !errors.Is(timeoutErr, timeout.ErrExceeded) || !errors.Is(timeoutErr.Unwrap(), context.Canceled) {
		t.Fatalf("timeout error=%v unwrap=%v", timeoutErr, timeoutErr.Unwrap())
	}
	var nilTimeoutErr *timeout.Error
	if nilTimeoutErr.Error() == "" || !errors.Is(nilTimeoutErr.Unwrap(), context.DeadlineExceeded) {
		t.Fatalf("nil timeout error=%v unwrap=%v", nilTimeoutErr, nilTimeoutErr.Unwrap())
	}

	if got := (&recover.Error{Value: "boom"}).Error(); !strings.Contains(got, "boom") {
		t.Fatalf("recover error=%q", got)
	}
	var nilRecoverErr *recover.Error
	if nilRecoverErr.Error() == "" {
		t.Fatal("nil recover error should have a message")
	}
}
