// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package logger logs one line per request.
package logger

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config controls request logging. The zero value logs every request to
// slog.Default().
type Config struct {
	// Logger receives the default log line. If nil, slog.Default() is used.
	Logger *slog.Logger
	// Log replaces the default log line. It receives every request's values.
	Log func(*zinc.Context, Values) error
	// Headers lists request headers to copy into Values.Headers.
	Headers []string
	// QueryParams lists query parameters to copy into Values.QueryParams.
	QueryParams []string

	timeNow func() time.Time
}

// Values contains extracted request/response values.
type Values struct {
	StartTime     time.Time
	Latency       time.Duration
	Method        string
	URI           string
	RoutePath     string
	Status        int
	Error         error
	RemoteIP      string
	Host          string
	UserAgent     string
	RequestID     string
	ContentLength string
	ResponseSize  int64
	Headers       map[string][]string
	QueryParams   map[string][]string
}

// New logs each request after the rest of the chain has run. A returned
// error is sent through the application's error handler first, so the logged
// status is the one the client received.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("logger", configs)
	log := config.Log
	if log == nil {
		log = defaultLog(config.Logger)
	}
	now := time.Now
	if config.timeNow != nil {
		now = config.timeNow
	}
	headers := make([]string, len(config.Headers))
	for i, header := range config.Headers {
		headers[i] = http.CanonicalHeaderKey(header)
	}
	queryParams := append([]string(nil), config.QueryParams...)

	return func(c *zinc.Context) error {
		start := now()
		rw := zinc.WrapResponseWriter(c.Writer())
		c.SetWriter(rw)

		err := c.Next()
		if err != nil {
			c.HandleError(err)
		}
		logErr := c.LastError()
		if logErr == nil {
			logErr = err
		}

		v := Values{
			StartTime:    start,
			Latency:      now().Sub(start),
			RoutePath:    c.FullPath(),
			Status:       shared.ResponseStatus(rw, logErr),
			Error:        logErr,
			ResponseSize: int64(rw.BytesWritten()),
		}
		if req := c.Request(); req != nil {
			v.Method = req.Method
			v.URI = req.RequestURI
			v.RemoteIP = c.IP()
			v.Host = req.Host
			v.UserAgent = req.UserAgent()
			v.ContentLength = req.Header.Get(zinc.HeaderContentLength)
			v.RequestID = req.Header.Get(zinc.HeaderXRequestID)
			if len(headers) > 0 {
				v.Headers = make(map[string][]string, len(headers))
				for _, header := range headers {
					if values, ok := req.Header[header]; ok {
						v.Headers[header] = append([]string(nil), values...)
					}
				}
			}
			if len(queryParams) > 0 {
				query := c.QueryValues()
				v.QueryParams = make(map[string][]string, len(queryParams))
				for _, key := range queryParams {
					if values, ok := query[key]; ok {
						v.QueryParams[key] = append([]string(nil), values...)
					}
				}
			}
		}
		if v.RequestID == "" {
			v.RequestID = rw.Header().Get(zinc.HeaderXRequestID)
		}

		if logErr := log(c, v); logErr != nil {
			return logErr
		}
		return err
	}
}

func defaultLog(logger *slog.Logger) func(*zinc.Context, Values) error {
	if logger == nil {
		logger = slog.Default()
	}
	return func(_ *zinc.Context, v Values) error {
		attrs := make([]slog.Attr, 0, 14)
		attrs = append(attrs, slog.String("method", v.Method), slog.String("uri", v.URI))
		if v.RoutePath != "" {
			attrs = append(attrs, slog.String("route", v.RoutePath))
		}
		attrs = append(attrs,
			slog.Int("status", v.Status),
			slog.Duration("latency", v.Latency),
			slog.String("host", v.Host),
			slog.String("bytes_in", v.ContentLength),
			slog.Int64("bytes_out", v.ResponseSize),
			slog.String("user_agent", v.UserAgent),
			slog.String("remote_ip", v.RemoteIP),
			slog.String("request_id", v.RequestID),
		)
		if len(v.Headers) > 0 {
			attrs = append(attrs, slog.Any("headers", v.Headers))
		}
		if len(v.QueryParams) > 0 {
			attrs = append(attrs, slog.Any("query", v.QueryParams))
		}
		if v.Error != nil {
			attrs = append(attrs, slog.String("error", v.Error.Error()))
			logger.LogAttrs(context.Background(), slog.LevelError, "REQUEST_ERROR", attrs...)
			return nil
		}
		logger.LogAttrs(context.Background(), slog.LevelInfo, "REQUEST", attrs...)
		return nil
	}
}
