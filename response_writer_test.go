// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

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
)

func TestWrapResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := WrapResponseWriter(rec)

	if writer.Written() {
		t.Fatal("writer should start unwritten")
	}
	if writer.Status() != http.StatusOK {
		t.Fatalf("initial status=%d", writer.Status())
	}

	n, err := writer.Write([]byte("hello"))
	mustDo(t, err)
	if n != 5 || writer.BytesWritten() != 5 || !writer.Written() || writer.Status() != http.StatusOK {
		t.Fatalf("write state n=%d bytes=%d written=%v status=%d", n, writer.BytesWritten(), writer.Written(), writer.Status())
	}
	writer.WriteHeader(http.StatusCreated)
	if writer.Status() != http.StatusOK {
		t.Fatalf("late status=%d", writer.Status())
	}

	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	} else {
		t.Fatal("wrapped recorder should expose flusher")
	}

	readFrom, ok := writer.(io.ReaderFrom)
	if !ok {
		t.Fatal("wrapped writer should expose reader from")
	}
	copied, err := readFrom.ReadFrom(strings.NewReader(" zinc"))
	mustDo(t, err)
	if copied != 5 || writer.BytesWritten() != 10 || rec.Body.String() != "hello zinc" {
		t.Fatalf("readfrom copied=%d bytes=%d body=%q", copied, writer.BytesWritten(), rec.Body.String())
	}

	if WrapResponseWriter(writer) != writer {
		t.Fatal("wrapping an existing ResponseWriter should return it")
	}
}

func TestWrapResponseWriterWriteString(t *testing.T) {
	stringBase := &wrappedTestStringResponseWriter{}
	writer := WrapResponseWriter(stringBase)
	sw, ok := writer.(io.StringWriter)
	if !ok {
		t.Fatal("wrapped writer should expose string writer")
	}
	n, err := sw.WriteString("zinc")
	mustDo(t, err)
	if n != 4 || writer.BytesWritten() != 4 || stringBase.body.String() != "zinc" {
		t.Fatalf("write string n=%d bytes=%d body=%q", n, writer.BytesWritten(), stringBase.body.String())
	}

	fallbackBase := &wrappedTestResponseWriter{}
	fallback := WrapResponseWriter(fallbackBase)
	sw, ok = fallback.(io.StringWriter)
	if !ok {
		t.Fatal("wrapped writer should expose fallback string writer")
	}
	n, err = sw.WriteString("go")
	mustDo(t, err)
	if n != 2 || fallback.BytesWritten() != 2 || fallbackBase.body.String() != "go" {
		t.Fatalf("fallback write string n=%d bytes=%d body=%q", n, fallback.BytesWritten(), fallbackBase.body.String())
	}
}

func TestWrapResponseWriterReadFromUsesUnderlying(t *testing.T) {
	base := &wrappedTestReaderFromResponseWriter{}
	writer := WrapResponseWriter(base)
	readFrom, ok := writer.(io.ReaderFrom)
	if !ok {
		t.Fatal("wrapped writer should expose reader from")
	}

	n, err := readFrom.ReadFrom(strings.NewReader("hello"))
	mustDo(t, err)
	if n != 5 || writer.BytesWritten() != 5 || !base.usedReaderFrom || base.body.String() != "hello" {
		t.Fatalf("readfrom n=%d bytes=%d used=%v body=%q", n, writer.BytesWritten(), base.usedReaderFrom, base.body.String())
	}
}

func TestWrapResponseWriterOptionalInterfaces(t *testing.T) {
	flushBase := &wrappedTestFlushResponseWriter{}
	flusher := WrapResponseWriter(flushBase).(http.Flusher)
	flusher.Flush()
	if !flushBase.flushed {
		t.Fatal("flush should be delegated")
	}

	plain := WrapResponseWriter(&wrappedTestResponseWriter{})
	plain.(http.Flusher).Flush()
	if _, _, err := plain.(http.Hijacker).Hijack(); err == nil {
		t.Fatal("hijack should fail when unsupported")
	}
	if err := plain.(http.Pusher).Push("/asset.js", nil); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("push err=%v", err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	hijackBase := &wrappedTestHijackResponseWriter{
		conn: serverConn,
		rw: bufio.NewReadWriter(
			bufio.NewReader(strings.NewReader("")),
			bufio.NewWriter(io.Discard),
		),
	}
	conn, brw, err := WrapResponseWriter(hijackBase).(http.Hijacker).Hijack()
	mustDo(t, err)
	if conn != serverConn || brw == nil {
		t.Fatal("hijack should return the underlying connection and readwriter")
	}

	pushBase := &wrappedTestPushResponseWriter{}
	opts := &http.PushOptions{Method: http.MethodGet}
	if err := WrapResponseWriter(pushBase).(http.Pusher).Push("/asset.js", opts); err != nil {
		t.Fatalf("push err=%v", err)
	}
	if pushBase.target != "/asset.js" || pushBase.opts != opts {
		t.Fatalf("push target=%q opts=%v", pushBase.target, pushBase.opts)
	}

	if WrapResponseWriter(pushBase).(interface{ Unwrap() http.ResponseWriter }).Unwrap() != pushBase {
		t.Fatal("unwrap should return original writer")
	}
}

type wrappedTestResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *wrappedTestResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *wrappedTestResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func (w *wrappedTestResponseWriter) WriteHeader(code int) {
	w.status = code
}

type wrappedTestStringResponseWriter struct {
	wrappedTestResponseWriter
}

func (w *wrappedTestStringResponseWriter) WriteString(s string) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.WriteString(s)
}

type wrappedTestReaderFromResponseWriter struct {
	wrappedTestResponseWriter
	usedReaderFrom bool
}

func (w *wrappedTestReaderFromResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	w.usedReaderFrom = true
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.ReadFrom(r)
}

type wrappedTestFlushResponseWriter struct {
	wrappedTestResponseWriter
	flushed bool
}

func (w *wrappedTestFlushResponseWriter) Flush() {
	w.flushed = true
}

type wrappedTestHijackResponseWriter struct {
	wrappedTestResponseWriter
	conn net.Conn
	rw   *bufio.ReadWriter
	err  error
}

func (w *wrappedTestHijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.rw, w.err
}

type wrappedTestPushResponseWriter struct {
	wrappedTestResponseWriter
	target string
	opts   *http.PushOptions
	err    error
}

func (w *wrappedTestPushResponseWriter) Push(target string, opts *http.PushOptions) error {
	w.target = target
	w.opts = opts
	return w.err
}
