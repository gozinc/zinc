// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package decompress

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestDecompressClose(t *testing.T) {
	app := zinc.New()
	app.Use(New())
	app.Post("/", func(c *zinc.Context) error {
		if err := c.Request().Body.Close(); err != nil {
			return err
		}
		return c.String("closed")
	})

	req := httptest.NewRequest(http.MethodPost, "/", gzipBody(t, "close me"))
	req.Header.Set(zinc.HeaderContentEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}
