// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package casbin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/basicauth"
	"github.com/0mjs/zinc/middleware/keyauth"
)

func TestSubjectFromBasicAuth(t *testing.T) {
	app := zinc.New()
	app.Use(basicauth.New(basicauth.Config{Validator: basicauth.Static("alice", "secret")}))
	app.Use(func(c *zinc.Context) error {
		c.Set("object", "doc")
		return c.Next()
	})
	enforcer := &casbinStub{allow: true}
	app.Use(New(Config{
		Enforcer: enforcer,
		Subject:  SubjectFromBasicAuth(),
		Object:   SubjectFromContext("object"),
		Action: func(*zinc.Context) any {
			return "read"
		},
	}))
	app.Get("/docs", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	req.SetBasicAuth("alice", "secret")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(enforcer.args) != 3 || enforcer.args[0] != "alice" || enforcer.args[1] != "doc" || enforcer.args[2] != "read" {
		t.Fatalf("args=%v", enforcer.args)
	}
}

type casbinStub struct {
	allow bool
	args  []any
}

func (s *casbinStub) Enforce(args ...any) (bool, error) {
	s.args = append([]any(nil), args...)
	return s.allow, nil
}

func TestSubjectFromKeyAuth(t *testing.T) {
	app := zinc.New()
	app.Use(keyauth.New(keyauth.Config{Validator: keyauth.Static("subject-key")}))
	app.Get("/subject", func(c *zinc.Context) error {
		return c.String(SubjectFromKeyAuth()(c).(string))
	})
	req := httptest.NewRequest(http.MethodGet, "/subject", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer subject-key")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != "subject-key" {
		t.Fatalf("subject body=%q", rec.Body.String())
	}
}
