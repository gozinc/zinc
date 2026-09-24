// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bufio"
	"fmt"
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
	set := new(responseWriterSet)
	return set.wrap(w, nil)
}

type wrappedResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int
	written      bool
	owner        *Context
}

// WriteHeader follows net/http's first-write-wins rule.
func (w *wrappedResponseWriter) WriteHeader(code int) {
	if w.written {
		return
	}
	if code < 100 || code > 999 {
		panic(fmt.Sprintf("invalid WriteHeader code %d", code))
	}
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.status = code
	w.written = true
	if w.owner != nil {
		w.owner.written = true
		w.owner.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *wrappedResponseWriter) Write(p []byte) (int, error) {
	w.startWrite()
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += n
	return n, err
}

// WriteString avoids converting s when the underlying writer supports io.StringWriter.
func (w *wrappedResponseWriter) WriteString(s string) (int, error) {
	w.startWrite()
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

func (w *wrappedResponseWriter) defaultStatus() int {
	if w.owner != nil {
		return w.owner.responseStatus()
	}
	return http.StatusOK
}

func (w *wrappedResponseWriter) startWrite() {
	if w.written {
		return
	}
	if code := w.defaultStatus(); code != http.StatusOK {
		w.WriteHeader(code)
		return
	}
	// Let Write perform net/http's implicit 200, avoiding an extra delegation.
	w.status, w.written = http.StatusOK, true
	if w.owner != nil {
		w.owner.written = true
		w.owner.status = http.StatusOK
	}
}

func (w *wrappedResponseWriter) contextOwner() *Context { return w.owner }

// FlushError allows ResponseController to report unsupported flushing rather
// than silently pretending that buffered data reached the client.
func (w *wrappedResponseWriter) FlushError() error {
	if !writerCanFlush(w.ResponseWriter) {
		return http.ErrNotSupported
	}
	if !w.written {
		w.WriteHeader(w.defaultStatus())
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func writerCanFlush(w http.ResponseWriter) bool {
	for {
		if _, ok := w.(http.Flusher); ok {
			return true
		}
		if unwrap, ok := w.(interface{ Unwrap() http.ResponseWriter }); ok {
			w = unwrap.Unwrap()
			continue
		}
		_, ok := w.(interface{ FlushError() error })
		return ok
	}
}

func (w *wrappedResponseWriter) hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.written = true
		if w.owner != nil {
			w.owner.written = true
		}
	}
	return conn, rw, err
}

// ReadFrom preserves io.Copy's ReaderFrom fast path without bypassing counters.
func (w *wrappedResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	var total int64
	if !w.written {
		// A read failure before any bytes must leave error handling available.
		var first [1]byte
		n, err := io.ReadFull(r, first[:])
		if n == 0 {
			if err == io.EOF {
				return 0, nil
			}
			return 0, err
		}
		written, writeErr := w.Write(first[:n])
		total = int64(written)
		if writeErr != nil {
			return total, writeErr
		}
		if written != n {
			return total, io.ErrShortWrite
		}
		if err != nil && err != io.EOF {
			return total, err
		}
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		w.bytesWritten += int(n)
		return total + n, err
	}
	n, err := io.Copy(responseWriterOnly{w: w}, r)
	return total + n, err
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

// The facade's method set reflects the underlying writer's optional HTTP
// capabilities. StringWriter and ReaderFrom have correct fallbacks everywhere.
// Context embeds this set, keeping the ordinary request path allocation-free.
type responseWriterSet struct {
	base wrappedResponseWriter
	v1   responseWriter1
	v2   responseWriter2
	v3   responseWriter3
	v4   responseWriter4
	v5   responseWriter5
	v6   responseWriter6
	v7   responseWriter7
}

func (s *responseWriterSet) wrap(w http.ResponseWriter, owner *Context) ResponseWriter {
	s.base = wrappedResponseWriter{ResponseWriter: w, owner: owner}
	mask := 0
	if _, ok := w.(http.Flusher); ok {
		mask |= 1
	}
	if _, ok := w.(http.Hijacker); ok {
		mask |= 2
	}
	if _, ok := w.(http.Pusher); ok {
		mask |= 4
	}
	if owner != nil && &s.base != &owner.response.base && owner.response.base.ResponseWriter != nil {
		// Middleware cannot add transport capabilities the original writer lacks.
		original := owner.response.base.ResponseWriter
		if _, ok := original.(http.Flusher); !ok {
			mask &^= 1
		}
		if _, ok := original.(http.Hijacker); !ok {
			mask &^= 2
		}
		if _, ok := original.(http.Pusher); !ok {
			mask &^= 4
		}
	}
	switch mask {
	case 1:
		s.v1.wrappedResponseWriter = &s.base
		return &s.v1
	case 2:
		s.v2.wrappedResponseWriter = &s.base
		return &s.v2
	case 3:
		s.v3.wrappedResponseWriter = &s.base
		return &s.v3
	case 4:
		s.v4.wrappedResponseWriter = &s.base
		return &s.v4
	case 5:
		s.v5.wrappedResponseWriter = &s.base
		return &s.v5
	case 6:
		s.v6.wrappedResponseWriter = &s.base
		return &s.v6
	case 7:
		s.v7.wrappedResponseWriter = &s.base
		return &s.v7
	default:
		return &s.base
	}
}

type responseWriter1 struct{ *wrappedResponseWriter }

func (w *responseWriter1) Flush() { _ = w.FlushError() }

type responseWriter2 struct{ *wrappedResponseWriter }

func (w *responseWriter2) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }

type responseWriter3 struct{ *wrappedResponseWriter }

func (w *responseWriter3) Flush()                                       { _ = w.FlushError() }
func (w *responseWriter3) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }

type responseWriter4 struct{ *wrappedResponseWriter }

func (w *responseWriter4) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

type responseWriter5 struct{ *wrappedResponseWriter }

func (w *responseWriter5) Flush() { _ = w.FlushError() }
func (w *responseWriter5) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

type responseWriter6 struct{ *wrappedResponseWriter }

func (w *responseWriter6) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }
func (w *responseWriter6) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

type responseWriter7 struct{ *wrappedResponseWriter }

func (w *responseWriter7) Flush()                                       { _ = w.FlushError() }
func (w *responseWriter7) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }
func (w *responseWriter7) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}
