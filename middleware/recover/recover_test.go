// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package recover

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestRecoverCatchesPanic(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Get("/panic", func(*zinc.Context) error {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRecoverCustomHandler(t *testing.T) {
	app := zinc.New()

	var recovered *Error
	app.Use(New(Config{
		Handler: func(c *zinc.Context, err *Error) error {
			recovered = err
			return c.Status(http.StatusTeapot).String("recovered")
		},
	}))
	app.Get("/panic", func(*zinc.Context) error {
		panic(errors.New("boom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "recovered" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if recovered == nil || recovered.Value == nil {
		t.Fatalf("recovered=%+v", recovered)
	}
	if len(recovered.Stack) == 0 {
		t.Fatal("stack should be captured by default")
	}
}
