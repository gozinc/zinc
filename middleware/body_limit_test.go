// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestBodyLimitWithinLimit(t *testing.T) {
	app := zinc.New()
	app.Use(BodyLimit(8))
	app.Post("/echo", func(c *zinc.Context) error {
		body, err := c.BodyString()
		if err != nil {
			return err
		}
		return c.String(body)
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBodyLimitRejectsByContentLength(t *testing.T) {
	app := zinc.New()

	called := false
	app.Use(BodyLimit(4))
	app.Post("/echo", func(c *zinc.Context) error {
		called = true
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("handler should not have run")
	}
}

func TestBodyLimitRejectsByReadAndOnlyExposesAllowedBytes(t *testing.T) {
	app := zinc.New()

	var seen string
	var seenErr error

	app.Use(BodyLimit(4))
	app.Post("/echo", func(c *zinc.Context) error {
		body, err := io.ReadAll(c.Request().Body)
		seen = string(body)
		seenErr = err
		return err
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("hello world"))
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if seen != "hell" {
		t.Fatalf("seen=%q", seen)
	}
	var limitErr *BodyLimitError
	if !errors.As(seenErr, &limitErr) {
		t.Fatalf("expected BodyLimitError, got %v", seenErr)
	}
	if limitErr.Limit != 4 || limitErr.Source != BodyLimitSourceBodyRead {
		t.Fatalf("limit error=%+v", limitErr)
	}
	if !errors.Is(seenErr, ErrBodyLimitExceeded) {
		t.Fatalf("errors.Is limit sentinel failed: %v", seenErr)
	}
	if !errors.Is(seenErr, zinc.ErrRequestEntityTooLarge) {
		t.Fatalf("errors.Is 413 failed: %v", seenErr)
	}
}

func TestBodyLimitWrappedMultipartErrorStillReturns413(t *testing.T) {
	app := zinc.New()

	var captured error
	app.Use(BodyLimit(64))
	app.Post("/upload", func(c *zinc.Context) error {
		_, err := c.MultipartForm()
		if err != nil {
			captured = err
			return fmt.Errorf("parse multipart: %w", err)
		}
		return c.String("ok")
	})

	req := newMultipartRequestBodyLimit(t, http.MethodPost, "/upload", "file", "payload", strings.Repeat("x", 256))
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if captured == nil {
		t.Fatal("expected multipart parse error")
	}
	if !errors.Is(captured, ErrBodyLimitExceeded) {
		t.Fatalf("captured error=%v", captured)
	}
	if !errors.Is(captured, zinc.ErrRequestEntityTooLarge) {
		t.Fatalf("captured error should unwrap to 413: %v", captured)
	}
}

func TestBodyLimitSkipper(t *testing.T) {
	app := zinc.New()
	app.Use(BodyLimitWithConfig(BodyLimitConfig{
		Skipper: func(*zinc.Context) bool { return true },
		Limit:   2,
	}))
	app.Post("/echo", func(c *zinc.Context) error {
		body, err := c.BodyString()
		if err != nil {
			return err
		}
		return c.String(body)
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestBodyLimitErrorMatching(t *testing.T) {
	err := &BodyLimitError{
		Limit:    10,
		Observed: 12,
		Source:   BodyLimitSourceContentLength,
	}

	if !errors.Is(err, ErrBodyLimitExceeded) {
		t.Fatal("expected match on ErrBodyLimitExceeded")
	}
	if !errors.Is(err, zinc.ErrRequestEntityTooLarge) {
		t.Fatal("expected match on zinc.ErrRequestEntityTooLarge")
	}

	var httpErr *zinc.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("expected BodyLimitError to unwrap to HTTPError")
	}
	if httpErr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("http code=%d", httpErr.Code)
	}
}

func mustNoErrBodyLimit(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newMultipartRequestBodyLimit(t *testing.T, method, target, field, filename, content string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(body.Bytes()))
	req.Header.Set(zinc.HeaderContentType, writer.FormDataContentType())
	return req
}
