// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package methodoverride

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestMethodOverrideQueryAndFirstGetter(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{
		Getter: FromFirst(
			FromHeader("X-Missing"),
			FromQuery("_method"),
		),
		SourceMethods: []string{"post"},
		Methods:       []string{"patch"},
	}))
	app.Patch("/resource", func(c *zinc.Context) error {
		return c.String(c.Method())
	})

	req := httptest.NewRequest(http.MethodPost, "/resource?_method=PATCH", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != http.MethodPatch {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
