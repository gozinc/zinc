// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package compress compresses responses for clients that accept gzip.
package compress

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config controls the gzip compression level and the smallest body worth
// compressing.
type Config struct {
	Level     int
	MinLength int
}

// New compresses responses for clients that accept gzip and preserves the
// optional ResponseWriter interfaces used by streaming and connection upgrades.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("compress", configs)
	level := config.Level
	if level == 0 {
		level = gzip.DefaultCompression
	}
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		panic("compress: Level must be a gzip compression level")
	}
	if config.MinLength < 0 {
		panic("compress: MinLength must be greater than or equal to zero")
	}

	// A gzip.Writer holds about a megabyte of compression state, so each
	// instance keeps its writers, all at its one level, for reuse.
	pool := &sync.Pool{}

	return func(c *zinc.Context) error {
		if !requestAcceptsGzip(c.Header(zinc.HeaderAcceptEncoding)) {
			appendVary(c.Writer().Header(), zinc.HeaderAcceptEncoding)
			return c.Next()
		}

		baseWriter := c.Writer()
		writer := &gzipResponseWriter{
			ResponseWriter: baseWriter,
			method:         c.Method(),
			level:          level,
			minLength:      config.MinLength,
			pool:           pool,
		}
		c.SetWriter(writer)
		finished := false
		defer func() {
			if finished {
				return
			}
			finishOnPanic(c, baseWriter, writer, recover())
		}()

		err := c.Next()
		if err != nil {
			c.HandleError(err)
		}
		finished = true
		closeErr := writer.Close()
		writer.release()
		c.SetWriter(baseWriter)
		if err != nil {
			return err
		}
		return closeErr
	}
}

// finishOnPanic runs when the downstream chain panics. An ordinary panic
// completes the response written so far, as returning normally would: a gzip
// stream gets its trailer, and a buffered body under MinLength is sent as
// identity. The client then reads a well-formed body rather than a truncated
// gzip stream, and the status already committed stays as it is. The original
// value is re-panicked so outer recovery still runs. http.ErrAbortHandler
// asks net/http to abort the response, so it restores the writer and
// re-panics the sentinel without completing any output.
func finishOnPanic(c *zinc.Context, baseWriter http.ResponseWriter, writer *gzipResponseWriter, value any) {
	c.SetWriter(baseWriter)
	if value == nil {
		// runtime.Goexit: there is no panic to propagate.
		return
	}
	if value != http.ErrAbortHandler {
		_ = writer.Close()
		writer.release()
	}
	panic(value)
}

func requestAcceptsGzip(header string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(value), "gzip") {
			continue
		}
		if params == "" {
			return true
		}
		if gzipQValueAllows(params) {
			return true
		}
	}
	return false
}

func gzipQValueAllows(params string) bool {
	for _, param := range strings.Split(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return err != nil || parsed > 0
	}
	return true
}

type gzipResponseWriter struct {
	http.ResponseWriter
	method      string
	level       int
	minLength   int
	status      int
	wroteHeader bool
	writer      *gzip.Writer
	pool        *sync.Pool
	buffer      bytes.Buffer
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader || w.status != 0 {
		return
	}
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.status = code
	if !gzipBodyAllowed(w.method, code) {
		w.writeRawHeader()
	}
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	// Once compression starts, every later write belongs to the gzip stream.
	// The Content-Encoding check below must not see the header startGzip set.
	if w.writer != nil {
		return w.writer.Write(p)
	}
	if w.wroteHeader {
		return w.ResponseWriter.Write(p)
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !gzipBodyAllowed(w.method, w.status) || !w.compressible() {
		w.writeRawHeader()
		return w.ResponseWriter.Write(p)
	}
	if w.minLength > 0 && w.writer == nil {
		// Delay the header decision until the response crosses MinLength. Once
		// committed as gzip, net/http cannot safely switch representation.
		if w.buffer.Len()+len(p) < w.minLength {
			return w.buffer.Write(p)
		}
		if err := w.startGzip(); err != nil {
			return 0, err
		}
		if w.buffer.Len() > 0 {
			if _, err := w.writer.Write(w.buffer.Bytes()); err != nil {
				return 0, err
			}
			w.buffer.Reset()
		}
		return w.writer.Write(p)
	}
	if err := w.startGzip(); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}

func (w *gzipResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *gzipResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(gzipWriterOnly{w: w}, r)
}

func (w *gzipResponseWriter) Flush() { _ = w.FlushError() }

func (w *gzipResponseWriter) FlushError() error {
	if w.writer != nil {
		if err := w.writer.Flush(); err != nil {
			return err
		}
	} else {
		// Flush commits the currently buffered short response as identity. Later
		// writes must keep that representation instead of switching to gzip.
		w.writeRawHeader()
		if w.buffer.Len() > 0 {
			if _, err := w.ResponseWriter.Write(w.buffer.Bytes()); err != nil {
				return err
			}
			w.buffer.Reset()
		}
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hj.Hijack()
}

func (w *gzipResponseWriter) Push(target string, opts *http.PushOptions) error {
	p, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return p.Push(target, opts)
}

func (w *gzipResponseWriter) Close() error {
	if w.writer != nil {
		return w.writer.Close()
	}
	if w.buffer.Len() > 0 {
		w.writeRawHeader()
		_, err := w.ResponseWriter.Write(w.buffer.Bytes())
		w.buffer.Reset()
		return err
	}
	if w.status != 0 && !w.wroteHeader {
		w.writeRawHeader()
	}
	return nil
}

func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *gzipResponseWriter) startGzip() error {
	if w.writer != nil {
		return nil
	}
	appendVary(w.Header(), zinc.HeaderAcceptEncoding)
	w.Header().Set(zinc.HeaderContentEncoding, "gzip")
	w.Header().Del(zinc.HeaderContentLength)
	weakenETag(w.Header())
	w.writeRawHeader()

	if w.pool == nil {
		// Built without New, as a test does: no pool to draw from.
	} else if writer, _ := w.pool.Get().(*gzip.Writer); writer != nil {
		writer.Reset(w.ResponseWriter)
		w.writer = writer
		return nil
	}
	writer, err := gzip.NewWriterLevel(w.ResponseWriter, w.level)
	if err != nil {
		return err
	}
	w.writer = writer
	return nil
}

// release returns a closed gzip writer to the pool, pointed at nothing so
// it keeps no reference to this response.
func (w *gzipResponseWriter) release() {
	if w.writer == nil || w.pool == nil {
		return
	}
	w.writer.Reset(io.Discard)
	w.pool.Put(w.writer)
	w.writer = nil
}

// compressible reports whether the response may be gzip-encoded: it is not
// already encoded, and it is not a partial response. A 206 or a Content-Range
// gives byte offsets in the identity representation, which a gzip body would
// no longer match.
func (w *gzipResponseWriter) compressible() bool {
	header := w.Header()
	return w.status != http.StatusPartialContent &&
		header.Get(zinc.HeaderContentEncoding) == "" &&
		header.Get(zinc.HeaderContentRange) == ""
}

// weakenETag marks a strong ETag weak once the body is gzip-encoded. RFC 9110
// section 8.8.3 requires a strong validator to differ between content
// codings. A weak one claims only semantic equivalence, which the gzip and
// identity bodies share, and still matches If-None-Match's weak comparison.
// A weak ETag is left as it is.
func weakenETag(header http.Header) {
	etag := header.Get(zinc.HeaderETag)
	if etag == "" || strings.HasPrefix(etag, "W/") {
		return
	}
	header.Set(zinc.HeaderETag, "W/"+etag)
}

func (w *gzipResponseWriter) writeRawHeader() {
	appendVary(w.Header(), zinc.HeaderAcceptEncoding)
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)
}

func gzipBodyAllowed(method string, status int) bool {
	if method == http.MethodHead {
		return false
	}
	if status >= 100 && status < 200 {
		return false
	}
	return status != http.StatusNoContent && status != http.StatusNotModified
}

type gzipWriterOnly struct {
	w *gzipResponseWriter
}

func (w gzipWriterOnly) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

func appendVary(header http.Header, value string) {
	for _, existing := range header.Values(zinc.HeaderVary) {
		for _, part := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add(zinc.HeaderVary, value)
}
