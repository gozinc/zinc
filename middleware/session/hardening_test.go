// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestSessionDeleteValuesAndOptions(t *testing.T) {
	var cfg Config
	cfg.Secret = []byte(strings.Repeat("s", 32))
	cfg.Name = "sid"
	cfg.MaxAge = 10
	cfg.DisableHTTPOnly = true

	app := zinc.New()
	app.Use(New(cfg))
	app.Get("/", func(c *zinc.Context) error {
		session := MustGet(c)
		session.Set("one", "1")
		session.Set("two", "2")
		session.Delete("two")
		values := session.Values()
		return c.String(values["one"] + values["two"])
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "1" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v", cookies)
	}
	if cookies[0].HttpOnly {
		t.Fatalf("cookie should not be http-only: %+v", cookies[0])
	}
	if cookies[0].MaxAge != 10 || cookies[0].Expires.IsZero() {
		t.Fatalf("cookie max-age/expires=%+v", cookies[0])
	}
}

func TestSessionHelpersOnNil(t *testing.T) {
	var session *Session
	if got := session.Get("missing"); got != "" {
		t.Fatalf("nil session get=%q", got)
	}
	session.Set("ignored", "value")
	session.Delete("ignored")
	if values := session.Values(); values != nil {
		t.Fatalf("nil session values=%v", values)
	}
	if _, ok := Get(nil); ok {
		t.Fatal("nil context should not have session")
	}
}
