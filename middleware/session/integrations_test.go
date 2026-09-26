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

func TestSessionPersistsSignedCookie(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Name: "sid", Secret: []byte(strings.Repeat("s", 32))}))
	app.Get("/", func(c *zinc.Context) error {
		session := MustGet(c)
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
	app.Use(New(Config{Name: "sid", Secret: []byte(strings.Repeat("s", 32))}))
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
