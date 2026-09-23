// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
)

// ResponseWriter records status and body size while retaining http.ResponseWriter.
type ResponseWriter interface {
	http.ResponseWriter
	Status() int
	BytesWritten() int
	Written() bool
}

// WrapResponseWriter wraps w or returns it unchanged when already instrumented.
func WrapResponseWriter(w http.ResponseWriter) ResponseWriter {
	if rw, ok := w.(ResponseWriter); ok {
		return rw
	}
	return &wrappedResponseWriter{ResponseWriter: w}
}

type wrappedResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int
	written      bool
}

// WriteHeader follows net/http's first-write-wins rule.
func (w *wrappedResponseWriter) WriteHeader(code int) {
	if w.written {
		return
	}
	w.status = code
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *wrappedResponseWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += n
	return n, err
}

// WriteString avoids converting s when the underlying writer supports io.StringWriter.
func (w *wrappedResponseWriter) WriteString(s string) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	if sw, ok := w.ResponseWriter.(io.StringWriter); ok {
		n, err := sw.WriteString(s)
		w.bytesWritten += n
		return n, err
	}
	return w.Write([]byte(s))
}

func (w *wrappedResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *wrappedResponseWriter) BytesWritten() int {
	return w.bytesWritten
}

func (w *wrappedResponseWriter) Written() bool {
	return w.written
}

func (w *wrappedResponseWriter) Flush() {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *wrappedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

// ReadFrom preserves io.Copy's ReaderFrom fast path without bypassing counters.
func (w *wrappedResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		w.bytesWritten += int(n)
		return n, err
	}
	return io.Copy(responseWriterOnly{w: w}, r)
}

func (w *wrappedResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *wrappedResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

type responseWriterOnly struct {
	w *wrappedResponseWriter
}

func (w responseWriterOnly) Write(p []byte) (int, error) {
	return w.w.Write(p)
}
