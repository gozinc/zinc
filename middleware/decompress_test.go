// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestDecompressGzipRequestBody(t *testing.T) {
	app := zinc.New()
	app.Use(Decompress())
	app.Post("/echo", func(c *zinc.Context) error {
		if got := c.GetHeader(zinc.HeaderContentEncoding); got != "" {
			t.Fatalf("content-encoding=%q", got)
		}
		if c.Request().ContentLength != -1 {
			t.Fatalf("content length=%d", c.Request().ContentLength)
		}
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", gzipBody(t, "compressed"))
	req.Header.Set(zinc.HeaderContentEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "compressed" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestDecompressRejectsUnsupportedEncoding(t *testing.T) {
	app := zinc.New()
	app.Use(Decompress())
	app.Post("/echo", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("body"))
	req.Header.Set(zinc.HeaderContentEncoding, "br")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDecompressRejectsInvalidGzipBody(t *testing.T) {
	app := zinc.New()
	app.Use(Decompress())
	app.Post("/echo", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("not gzip"))
	req.Header.Set(zinc.HeaderContentEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDecompressRejectsOversizedDecompressedBody(t *testing.T) {
	app := zinc.New()
	app.Use(DecompressWithConfig(DecompressConfig{MaxDecompressedSize: 4}))
	app.Post("/echo", func(c *zinc.Context) error {
		_, err := io.ReadAll(c.Request().Body)
		return err
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", gzipBody(t, "compressed"))
	req.Header.Set(zinc.HeaderContentEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDecompressSkipper(t *testing.T) {
	app := zinc.New()
	app.Use(DecompressWithConfig(DecompressConfig{
		Skipper: func(*zinc.Context) bool { return true },
	}))
	app.Post("/echo", func(c *zinc.Context) error {
		if got := c.GetHeader(zinc.HeaderContentEncoding); got != "gzip" {
			t.Fatalf("content-encoding=%q", got)
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", gzipBody(t, "compressed"))
	req.Header.Set(zinc.HeaderContentEncoding, "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Body.String() != "ok" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestDecompressErrorMatching(t *testing.T) {
	err := errors.Join(zinc.ErrUnsupportedMediaType, ErrDecompressUnsupportedEncoding)
	if !errors.Is(err, zinc.ErrUnsupportedMediaType) {
		t.Fatal("expected zinc.ErrUnsupportedMediaType match")
	}
	if !errors.Is(err, ErrDecompressUnsupportedEncoding) {
		t.Fatal("expected ErrDecompressUnsupportedEncoding match")
	}
}

func gzipBody(t *testing.T, body string) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatalf("write gzip body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip body: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func mustNoErrDecompress(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
