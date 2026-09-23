// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestBodyDumpCapturesRequestAndResponse(t *testing.T) {
	app := zinc.New()

	var got BodyDumpSnapshot
	app.Use(BodyDumpWithConfig(BodyDumpConfig{
		Observe: func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
			got = snapshot
		},
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
	if string(got.RequestBody) != "hello" {
		t.Fatalf("request body=%q", string(got.RequestBody))
	}
	if string(got.ResponseBody) != "hello" {
		t.Fatalf("response body=%q", string(got.ResponseBody))
	}
	if got.RequestBytes != 5 {
		t.Fatalf("request bytes=%d", got.RequestBytes)
	}
	if got.ResponseBytes != 5 {
		t.Fatalf("response bytes=%d", got.ResponseBytes)
	}
	if got.Status != http.StatusOK {
		t.Fatalf("snapshot status=%d", got.Status)
	}
	if got.RoutePath != "/echo" {
		t.Fatalf("route path=%q", got.RoutePath)
	}
	if got.Path != "/echo" {
		t.Fatalf("path=%q", got.Path)
	}
	if got.Method != http.MethodPost {
		t.Fatalf("method=%q", got.Method)
	}
	if got.Error != nil {
		t.Fatalf("snapshot error=%v", got.Error)
	}
}

func TestBodyDumpRequestSnapshotCanTruncateWithoutChangingHandlerBody(t *testing.T) {
	app := zinc.New()

	var got BodyDumpSnapshot
	app.Use(BodyDumpWithConfig(BodyDumpConfig{
		MaxRequestBytes: 4,
		Observe: func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
			got = snapshot
		},
	}))
	app.Post("/upload", func(c *zinc.Context) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(string(body))
	})

	full := "abcdefghij"
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(full))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != full {
		t.Fatalf("handler body=%q", rec.Body.String())
	}
	if string(got.RequestBody) != "abcd" {
		t.Fatalf("dumped request body=%q", string(got.RequestBody))
	}
	if !got.RequestTruncated {
		t.Fatal("request should be marked truncated")
	}
	if got.RequestBytes != int64(len(full)) {
		t.Fatalf("request bytes=%d", got.RequestBytes)
	}
}

func TestBodyDumpResponseTruncationStillSendsFullResponse(t *testing.T) {
	app := zinc.New()

	var got BodyDumpSnapshot
	app.Use(BodyDumpWithConfig(BodyDumpConfig{
		MaxResponseBytes: 4,
		Observe: func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
			got = snapshot
		},
	}))
	app.Get("/stream", func(c *zinc.Context) error {
		for _, chunk := range []string{"ab", "cd", "ef", "gh"} {
			if _, err := io.WriteString(c.Writer(), chunk); err != nil {
				return err
			}
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "abcdefgh" {
		t.Fatalf("response body=%q", rec.Body.String())
	}
	if string(got.ResponseBody) != "abcd" {
		t.Fatalf("dumped response body=%q", string(got.ResponseBody))
	}
	if !got.ResponseTruncated {
		t.Fatal("response should be marked truncated")
	}
	if got.ResponseBytes != 8 {
		t.Fatalf("response bytes=%d", got.ResponseBytes)
	}
}

func TestBodyDumpRedactor(t *testing.T) {
	app := zinc.New()

	var got BodyDumpSnapshot
	app.Use(BodyDumpWithConfig(BodyDumpConfig{
		Redact: func(_ *zinc.Context, snapshot *BodyDumpSnapshot) {
			snapshot.RequestBody = []byte("[redacted]")
			snapshot.ResponseBody = []byte("[redacted]")
		},
		Observe: func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
			got = snapshot
		},
	}))
	app.Post("/secret", func(c *zinc.Context) error {
		return c.String("top-secret")
	})

	req := httptest.NewRequest(http.MethodPost, "/secret", strings.NewReader("password=123"))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if string(got.RequestBody) != "[redacted]" {
		t.Fatalf("request body=%q", string(got.RequestBody))
	}
	if string(got.ResponseBody) != "[redacted]" {
		t.Fatalf("response body=%q", string(got.ResponseBody))
	}
}

func TestBodyDumpSkipper(t *testing.T) {
	app := zinc.New()

	called := false
	app.Use(BodyDumpWithConfig(BodyDumpConfig{
		Skipper: func(*zinc.Context) bool { return true },
		Observe: func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
			called = true
		},
	}))
	app.Get("/x", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if called {
		t.Fatal("observer should not have been called")
	}
}

func TestBodyDumpInfersErrorStatus(t *testing.T) {
	app := zinc.New()

	var got BodyDumpSnapshot
	app.Use(BodyDumpWithConfig(BodyDumpConfig{
		Observe: func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
			got = snapshot
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return zinc.ErrUnauthorized
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got.Status != http.StatusUnauthorized {
		t.Fatalf("snapshot status=%d", got.Status)
	}
	if !errors.Is(got.Error, zinc.ErrUnauthorized) {
		t.Fatalf("snapshot error=%v", got.Error)
	}
	if got.ResponseBytes != 0 {
		t.Fatalf("response bytes=%d", got.ResponseBytes)
	}
	if string(got.ResponseBody) != "" {
		t.Fatalf("response body=%q", string(got.ResponseBody))
	}
}

func TestBodyDumpObservesBodyReadFailure(t *testing.T) {
	app := zinc.New()

	var got BodyDumpSnapshot
	app.Use(BodyDumpWithConfig(BodyDumpConfig{
		Observe: func(_ *zinc.Context, snapshot BodyDumpSnapshot) {
			got = snapshot
		},
	}))
	app.Post("/upload", func(c *zinc.Context) error {
		t.Fatal("handler should not run")
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", &failingReadCloser{
		data:     []byte("partial"),
		failAt:   4,
		failWith: errors.New("connection reset"),
	})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got.Status != http.StatusInternalServerError {
		t.Fatalf("snapshot status=%d", got.Status)
	}
	if got.Error == nil || !strings.Contains(got.Error.Error(), "connection reset") {
		t.Fatalf("snapshot error=%v", got.Error)
	}
}

func TestCaptureResponseWriterWritePaths(t *testing.T) {
	base := &bodyDumpBasicResponseWriter{}
	rw := newBodyDumpCaptureResponseWriter(base, 4)

	if rw.Written() {
		t.Fatal("writer should not be marked written initially")
	}
	if got := rw.Status(); got != http.StatusOK {
		t.Fatalf("default status=%d", got)
	}

	n, err := rw.Write([]byte("abc"))
	if err != nil {
		t.Fatalf("write err=%v", err)
	}
	if n != 3 {
		t.Fatalf("write bytes=%d", n)
	}
	if got := rw.Size(); got != 3 {
		t.Fatalf("size=%d", got)
	}
	if string(rw.Bytes()) != "abc" {
		t.Fatalf("dump=%q", string(rw.Bytes()))
	}

	stringBase := &bodyDumpStringResponseWriter{}
	rwString := newBodyDumpCaptureResponseWriter(stringBase, 2)
	n, err = rwString.WriteString("xyzz")
	if err != nil {
		t.Fatalf("write string err=%v", err)
	}
	if n != 4 {
		t.Fatalf("write string bytes=%d", n)
	}
	if string(rwString.Bytes()) != "xy" {
		t.Fatalf("dump=%q", string(rwString.Bytes()))
	}
	if !rwString.Truncated() {
		t.Fatal("writer should be marked truncated")
	}

	headerOnly := newBodyDumpCaptureResponseWriter(&bodyDumpBasicResponseWriter{}, -1)
	headerOnly.WriteHeader(http.StatusCreated)
	headerOnly.WriteHeader(http.StatusInternalServerError)
	if got := headerOnly.Status(); got != http.StatusCreated {
		t.Fatalf("status=%d", got)
	}
}

func TestCaptureResponseWriterReadFromAndOptionalInterfaces(t *testing.T) {
	rfBase := &bodyDumpReaderFromResponseWriter{}
	rw := newBodyDumpCaptureResponseWriter(rfBase, -1)
	n, err := rw.ReadFrom(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("readfrom err=%v", err)
	}
	if n != 5 {
		t.Fatalf("readfrom bytes=%d", n)
	}
	if string(rw.Bytes()) != "hello" {
		t.Fatalf("dump=%q", string(rw.Bytes()))
	}

	flushBase := &bodyDumpFlushResponseWriter{}
	rwFlush := newBodyDumpCaptureResponseWriter(flushBase, -1)
	rwFlush.Flush()
	if !flushBase.flushed {
		t.Fatal("flush should be delegated")
	}

	plain := newBodyDumpCaptureResponseWriter(&bodyDumpBasicResponseWriter{}, -1)
	plain.Flush()
	if err := plain.Push("/asset.js", nil); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("push err=%v", err)
	}
	_, _, err = plain.Hijack()
	if err == nil {
		t.Fatal("hijack should fail when unsupported")
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	hijackBase := &bodyDumpHijackResponseWriter{
		conn: serverConn,
		rw: bufio.NewReadWriter(
			bufio.NewReader(strings.NewReader("")),
			bufio.NewWriter(io.Discard),
		),
	}
	rwHijack := newBodyDumpCaptureResponseWriter(hijackBase, -1)
	conn, brw, err := rwHijack.Hijack()
	if err != nil {
		t.Fatalf("hijack err=%v", err)
	}
	if conn != serverConn {
		t.Fatal("unexpected conn")
	}
	if brw == nil {
		t.Fatal("expected readwriter")
	}

	pushBase := &bodyDumpPushResponseWriter{}
	rwPush := newBodyDumpCaptureResponseWriter(pushBase, -1)
	opts := &http.PushOptions{Method: http.MethodGet}
	if err := rwPush.Push("/asset.js", opts); err != nil {
		t.Fatalf("push err=%v", err)
	}
	if pushBase.target != "/asset.js" {
		t.Fatalf("target=%q", pushBase.target)
	}
	if rwPush.Unwrap() != pushBase {
		t.Fatal("unwrap should return original writer")
	}
}

func mustNoErrBodyDump(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type failingReadCloser struct {
	data     []byte
	pos      int
	failAt   int
	failWith error
}

func (f *failingReadCloser) Read(p []byte) (int, error) {
	if f.pos >= f.failAt {
		return 0, f.failWith
	}

	n := copy(p, f.data[f.pos:])
	f.pos += n
	if f.pos >= f.failAt {
		return n, f.failWith
	}
	return n, nil
}

func (f *failingReadCloser) Close() error {
	return nil
}

type bodyDumpBasicResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *bodyDumpBasicResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *bodyDumpBasicResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func (w *bodyDumpBasicResponseWriter) WriteHeader(code int) {
	w.status = code
}

type bodyDumpStringResponseWriter struct {
	bodyDumpBasicResponseWriter
}

func (w *bodyDumpStringResponseWriter) WriteString(s string) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.WriteString(s)
}

type bodyDumpReaderFromResponseWriter struct {
	bodyDumpBasicResponseWriter
}

func (w *bodyDumpReaderFromResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.ReadFrom(r)
}

type bodyDumpFlushResponseWriter struct {
	bodyDumpBasicResponseWriter
	flushed bool
}

func (w *bodyDumpFlushResponseWriter) Flush() {
	w.flushed = true
}

type bodyDumpHijackResponseWriter struct {
	bodyDumpBasicResponseWriter
	conn net.Conn
	rw   *bufio.ReadWriter
	err  error
}

func (w *bodyDumpHijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.rw, w.err
}

type bodyDumpPushResponseWriter struct {
	bodyDumpBasicResponseWriter
	target string
	opts   *http.PushOptions
	err    error
}

func (w *bodyDumpPushResponseWriter) Push(target string, opts *http.PushOptions) error {
	w.target = target
	w.opts = opts
	return w.err
}
