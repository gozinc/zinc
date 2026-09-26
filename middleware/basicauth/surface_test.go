// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package basicauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestStaticValidator(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Validator: Static("joe", "secret")}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String(MustGet(c).Username)
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.SetBasicAuth("joe", "secret")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "joe" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
