// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package compress

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

// benchJSON is a JSON body of about 4KB.
var benchJSON = func() []byte {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; b.Len() < 4000; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":12345,"name":"widget","price":19.99,"tags":["a","b","c"]}`)
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}()

func benchmarkCompress(b *testing.B, config Config, body []byte) {
	app := zinc.New()
	app.Use(New(config))
	app.Get("/", func(c *zinc.Context) error {
		return c.Data("application/json", body)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(zinc.HeaderAcceptEncoding, "gzip")

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status=%d", rec.Code)
		}
	}
}

// BenchmarkCompress gzips a 4KB JSON body.
func BenchmarkCompress(b *testing.B) {
	benchmarkCompress(b, Config{}, benchJSON)
}

// BenchmarkCompressBelowMinLength sends a small body uncompressed because it
// is under MinLength: the cost of the middleware without gzip.
func BenchmarkCompressBelowMinLength(b *testing.B) {
	benchmarkCompress(b, Config{MinLength: 1024}, []byte(`{"ok":true}`))
}
