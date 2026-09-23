// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

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

	"github.com/0mjs/zinc"
)

// GzipConfig controls response compression level and minimum body size.
type GzipConfig struct {
	Skipper   func(*zinc.Context) bool
	Level     int
	MinLength int
}

// Gzip compresses eligible responses at the default level.
func Gzip() zinc.Middleware {
	return GzipWithConfig(GzipConfig{})
}

// GzipWithConfig negotiates gzip and preserves optional ResponseWriter
// interfaces used by streaming and connection upgrades.
func GzipWithConfig(config GzipConfig) zinc.Middleware {
	level := config.Level
	if level == 0 {
		level = gzip.DefaultCompression
	}
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		panic("zincgzip: Level must be a gzip compression level")
	}
	if config.MinLength < 0 {
		panic("zincgzip: MinLength must be greater than or equal to zero")
	}

	return func(c *zinc.Context) error {
		if config.Skipper != nil && config.Skipper(c) {
			return c.Next()
		}

		if !requestAcceptsGzip(c.GetHeader(zinc.HeaderAcceptEncoding)) {
			return c.Next()
		}

		baseWriter := c.Writer()
		writer := &gzipResponseWriter{
			ResponseWriter: baseWriter,
			method:         c.Method(),
			level:          level,
			minLength:      config.MinLength,
		}
		c.SetWriter(writer)

		err := c.Next()
		if err != nil {
			c.Error(err)
		}
		closeErr := writer.Close()
		c.SetWriter(baseWriter)
		if err != nil {
			return err
		}
		return closeErr
	}
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
	buffer      bytes.Buffer
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader || w.status != 0 {
		return
	}
	w.status = code
	if !gzipBodyAllowed(w.method, code) {
		w.writeRawHeader()
	}
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !gzipBodyAllowed(w.method, w.status) || w.Header().Get(zinc.HeaderContentEncoding) != "" {
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

func (w *gzipResponseWriter) Flush() {
	if w.writer != nil {
		_ = w.writer.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
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
	w.writeRawHeader()

	writer, err := gzip.NewWriterLevel(w.ResponseWriter, w.level)
	if err != nil {
		return err
	}
	w.writer = writer
	return nil
}

func (w *gzipResponseWriter) writeRawHeader() {
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
