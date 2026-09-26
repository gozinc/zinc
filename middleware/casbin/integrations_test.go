// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package casbin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestCasbinAuthAllowsRequest(t *testing.T) {
	enforcer := &casbinStub{allow: true}
	app := zinc.New()
	app.Use(New(Config{Enforcer: enforcer, Subject: func(*zinc.Context) any {
		return "alice"
	}}))
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
	app.Use(New(Config{Enforcer: &casbinStub{allow: false}, Subject: func(*zinc.Context) any {
		return "alice"
	}}))
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
