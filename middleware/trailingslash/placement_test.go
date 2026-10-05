// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package trailingslash

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

// trailingslash changes the routed path, so like rewrite it only works
// before routing: registering it on a group panics at startup.

func groupPanic(t *testing.T, register func()) string {
	t.Helper()
	var msg string
	func() {
		defer func() {
			if v := recover(); v != nil {
				msg = fmt.Sprint(v)
			}
		}()
		register()
	}()
	if msg == "" {
		t.Fatal("expected a registration panic")
	}
	return msg
}

func TestTrailingSlashOnGroupPanics(t *testing.T) {
	never := func(*zinc.Context) bool { return false }
	for name, cfg := range map[string]Config{
		"remove":   {},
		"redirect": {Redirect: true},
		"add":      {Add: true, Redirect: true},
	} {
		t.Run(name, func(t *testing.T) {
			app := zinc.New()
			msg := groupPanic(t, func() { app.Group("/api").Use(New(cfg)) })
			if !strings.Contains(msg, "trailingslash middleware on group") || !strings.Contains(msg, "app.Use") {
				t.Fatalf("panic=%q", msg)
			}
			groupPanic(t, func() { app.Group("/v1", New(cfg)) })
			groupPanic(t, func() { app.Group("/v2").Use(zinc.Skip(never, New(cfg))) })
		})
	}
}

func TestTrailingSlashOnAppStillRoutes(t *testing.T) {
	app := zinc.New(zinc.Config{StrictRouting: true})
	app.Use(zinc.Skip(func(*zinc.Context) bool { return false }, New()))
	app.Group("/api").Get("/users", func(c *zinc.Context) error { return c.String(c.Path()) })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "/api/users" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}
