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

	"github.com/0mjs/zinc"
)

const (
	// BodyDumpDefaultMaxRequestBytes and BodyDumpDefaultMaxResponseBytes bound
	// captured payloads while preserving total byte counts.
	BodyDumpDefaultMaxRequestBytes  int64 = 1 << 20
	BodyDumpDefaultMaxResponseBytes int64 = 1 << 20
)

// BodyDumpObserver receives one completed request snapshot.
type BodyDumpObserver func(*zinc.Context, BodyDumpSnapshot)

// BodyDumpRedactor removes sensitive data before observation.
type BodyDumpRedactor func(*zinc.Context, *BodyDumpSnapshot)

// BodyDumpConfig controls capture limits, redaction, and observation.
type BodyDumpConfig struct {
	Skipper          func(*zinc.Context) bool
	Observe          BodyDumpObserver
	Redact           BodyDumpRedactor
	MaxRequestBytes  int64
	MaxResponseBytes int64
}

// BodyDumpSnapshot contains bounded request and response captures.
type BodyDumpSnapshot struct {
	Method            string
	Path              string
	RoutePath         string
	Status            int
	RequestBody       []byte
	ResponseBody      []byte
	RequestBytes      int64
	ResponseBytes     int64
	RequestTruncated  bool
	ResponseTruncated bool
	Error             error
}

// DefaultBodyDumpConfig returns bounded one-megabyte capture defaults.
func DefaultBodyDumpConfig() BodyDumpConfig {
	return BodyDumpConfig{
		MaxRequestBytes:  BodyDumpDefaultMaxRequestBytes,
		MaxResponseBytes: BodyDumpDefaultMaxResponseBytes,
	}
}

// BodyDump captures request and response bodies for observer.
func BodyDump(observer BodyDumpObserver) zinc.Middleware {
	return BodyDumpWithConfig(BodyDumpConfig{Observe: observer})
}

// BodyDumpWithConfig captures bodies without replacing the bytes visible to
// downstream handlers. Configure Redact before exporting snapshots.
func BodyDumpWithConfig(config BodyDumpConfig) zinc.Middleware {
	cfg := resolveBodyDumpConfig(config)

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		requestBody, err := c.BodyBytes()
		if err != nil {
			snapshot := BodyDumpSnapshot{
				Method:        c.Method(),
				Path:          c.Path(),
				RoutePath:     c.FullPath(),
				Status:        inferBodyDumpStatus(nil, err),
				RequestBytes:  0,
				ResponseBytes: 0,
				Error:         err,
			}
			if cfg.Redact != nil {
				cfg.Redact(c, &snapshot)
			}
			cfg.Observe(c, snapshot)
			return err
		}

		requestBytes := int64(len(requestBody))
		requestBody, requestTruncated := limitBodyDumpCopy(requestBody, cfg.MaxRequestBytes)
		baseWriter := c.Writer()
		writer := newBodyDumpCaptureResponseWriter(baseWriter, cfg.MaxResponseBytes)
		c.SetWriter(writer)

		err = c.Next()

		c.SetWriter(baseWriter)

		logErr := c.LastError()
		if logErr == nil {
			logErr = err
		}

		snapshot := BodyDumpSnapshot{
			Method:            c.Method(),
			Path:              c.Path(),
			RoutePath:         c.FullPath(),
			Status:            inferBodyDumpStatus(writer, logErr),
			RequestBody:       requestBody,
			ResponseBody:      writer.Bytes(),
			RequestBytes:      requestBytes,
			ResponseBytes:     writer.Size(),
			RequestTruncated:  requestTruncated,
			ResponseTruncated: writer.Truncated(),
			Error:             logErr,
		}

		if cfg.Redact != nil {
			cfg.Redact(c, &snapshot)
		}
		cfg.Observe(c, snapshot)

		return err
	}
}

func resolveBodyDumpConfig(config BodyDumpConfig) BodyDumpConfig {
	cfg := DefaultBodyDumpConfig()
	cfg.Skipper = config.Skipper
	if config.Observe == nil {
		panic("zincbodydump: Observe is required")
	}
	cfg.Observe = config.Observe
	cfg.Redact = config.Redact
	if config.MaxRequestBytes != 0 {
		cfg.MaxRequestBytes = config.MaxRequestBytes
	}
	if config.MaxResponseBytes != 0 {
		cfg.MaxResponseBytes = config.MaxResponseBytes
	}
	return cfg
}

func limitBodyDumpCopy(body []byte, limit int64) ([]byte, bool) {
	if len(body) == 0 {
		return nil, false
	}
	if limit < 0 || int64(len(body)) <= limit {
		return append([]byte(nil), body...), false
	}
	return append([]byte(nil), body[:limit]...), true
}

func inferBodyDumpStatus(writer *bodyDumpCaptureResponseWriter, err error) int {
	if writer != nil && writer.Written() {
		return writer.Status()
	}
	if err != nil {
		var httpErr *zinc.HTTPError
		if errors.As(err, &httpErr) {
			return httpErr.Code
		}
		return http.StatusInternalServerError
	}
	return http.StatusOK
}

type bodyDumpCaptureResponseWriter struct {
	http.ResponseWriter
	status    int
	size      int64
	limit     int64
	buf       bytes.Buffer
	truncated bool
}

func newBodyDumpCaptureResponseWriter(w http.ResponseWriter, limit int64) *bodyDumpCaptureResponseWriter {
	return &bodyDumpCaptureResponseWriter{
		ResponseWriter: w,
		limit:          limit,
	}
}

func (w *bodyDumpCaptureResponseWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *bodyDumpCaptureResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	// Account only for bytes accepted by the underlying writer; observers must
	// see transport reality rather than the attempted payload length.
	n, err := w.ResponseWriter.Write(p)
	w.size += int64(n)
	w.capture(p[:n])
	return n, err
}

func (w *bodyDumpCaptureResponseWriter) WriteString(s string) (int, error) {
	if sw, ok := w.ResponseWriter.(io.StringWriter); ok {
		if w.status == 0 {
			w.WriteHeader(http.StatusOK)
		}
		n, err := sw.WriteString(s)
		w.size += int64(n)
		if n > 0 {
			w.capture([]byte(s[:n]))
		}
		return n, err
	}
	return w.Write([]byte(s))
}

func (w *bodyDumpCaptureResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(bodyDumpWriterOnly{w: w}, r)
}

func (w *bodyDumpCaptureResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *bodyDumpCaptureResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hj.Hijack()
}

func (w *bodyDumpCaptureResponseWriter) Push(target string, opts *http.PushOptions) error {
	p, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return p.Push(target, opts)
}

func (w *bodyDumpCaptureResponseWriter) Bytes() []byte {
	return append([]byte(nil), w.buf.Bytes()...)
}

func (w *bodyDumpCaptureResponseWriter) Size() int64 {
	return w.size
}

func (w *bodyDumpCaptureResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *bodyDumpCaptureResponseWriter) Truncated() bool {
	return w.truncated
}

func (w *bodyDumpCaptureResponseWriter) Written() bool {
	return w.status != 0
}

func (w *bodyDumpCaptureResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *bodyDumpCaptureResponseWriter) capture(p []byte) {
	if len(p) == 0 {
		return
	}
	if w.limit >= 0 {
		remaining := int(w.limit) - w.buf.Len()
		if remaining <= 0 {
			w.truncated = true
			return
		}
		if len(p) > remaining {
			_, _ = w.buf.Write(p[:remaining])
			w.truncated = true
			return
		}
	}
	_, _ = w.buf.Write(p)
}

type bodyDumpWriterOnly struct {
	w io.Writer
}

func (w bodyDumpWriterOnly) Write(p []byte) (int, error) {
	return w.w.Write(p)
}
