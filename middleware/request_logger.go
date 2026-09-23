// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
)

// RequestLoggerConfig controls request logging behavior.
type RequestLoggerConfig struct {
	// Skipper skips logging when it returns true.
	Skipper func(*zinc.Context) bool
	// BeforeNextFunc runs before the rest of the middleware/handler chain.
	BeforeNextFunc func(*zinc.Context)
	// LogValuesFunc receives extracted values and should emit logs.
	// If nil, a default slog logger implementation is used.
	LogValuesFunc func(*zinc.Context, RequestLoggerValues) error
	// HandleError forwards returned errors through zinc's error handler before logging.
	HandleError bool
	// Logger is used by the default LogValuesFunc. If nil, slog.Default() is used.
	Logger *slog.Logger

	LogLatency       bool
	LogMethod        bool
	LogURI           bool
	LogRoutePath     bool
	LogStatus        bool
	LogError         bool
	LogRemoteIP      bool
	LogHost          bool
	LogUserAgent     bool
	LogRequestID     bool
	LogContentLength bool
	LogResponseSize  bool
	LogHeaders       []string
	LogQueryParams   []string

	timeNow func() time.Time
}

// RequestLoggerValues contains extracted request/response values.
type RequestLoggerValues struct {
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

// DefaultRequestLoggerConfig returns default request-logger settings.
func DefaultRequestLoggerConfig() RequestLoggerConfig {
	return RequestLoggerConfig{
		HandleError:      true,
		LogLatency:       true,
		LogMethod:        true,
		LogURI:           true,
		LogStatus:        true,
		LogError:         true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogUserAgent:     true,
		LogRequestID:     true,
		LogContentLength: true,
		LogResponseSize:  true,
	}
}

// RequestLogger creates request-logging middleware with default settings.
func RequestLogger() zinc.Middleware {
	return RequestLoggerWithConfig(DefaultRequestLoggerConfig())
}

// RequestLoggerWithConfig creates request-logging middleware with custom settings.
func RequestLoggerWithConfig(config RequestLoggerConfig) zinc.Middleware {
	if config.Skipper == nil {
		config.Skipper = defaultRequestLoggerSkipper
	}
	if config.LogValuesFunc == nil {
		config.LogValuesFunc = defaultRequestLogValuesFunc(config.Logger)
	}
	now := time.Now
	if config.timeNow != nil {
		now = config.timeNow
	}

	logHeaders := len(config.LogHeaders) > 0
	headers := append([]string(nil), config.LogHeaders...)
	for i := range headers {
		headers[i] = http.CanonicalHeaderKey(headers[i])
	}

	logQueryParams := len(config.LogQueryParams) > 0

	return func(c *zinc.Context) error {
		if config.Skipper(c) {
			return c.Next()
		}

		req := c.Request()
		start := now()

		if config.BeforeNextFunc != nil {
			config.BeforeNextFunc(c)
		}

		var rw zinc.ResponseWriter
		if config.LogStatus || config.LogResponseSize {
			rw = zinc.WrapResponseWriter(c.Writer())
			c.SetWriter(rw)
		}

		err := c.Next()
		if err != nil && config.HandleError {
			c.Error(err)
		}

		logErr := c.LastError()
		if logErr == nil {
			logErr = err
		}

		v := RequestLoggerValues{
			StartTime: start,
		}
		if config.LogLatency {
			v.Latency = now().Sub(start)
		}

		if req != nil {
			if config.LogMethod {
				v.Method = req.Method
			}
			if config.LogURI {
				v.URI = req.RequestURI
			}
			if config.LogRemoteIP {
				v.RemoteIP = c.IP()
			}
			if config.LogHost {
				v.Host = req.Host
			}
			if config.LogUserAgent {
				v.UserAgent = req.UserAgent()
			}
			if config.LogContentLength {
				v.ContentLength = req.Header.Get(zinc.HeaderContentLength)
			}
			if logHeaders {
				v.Headers = make(map[string][]string, len(headers))
				for _, header := range headers {
					if values, ok := req.Header[header]; ok {
						v.Headers[header] = append([]string(nil), values...)
					}
				}
			}
			if logQueryParams {
				queryValues := c.QueryValues()
				v.QueryParams = make(map[string][]string, len(config.LogQueryParams))
				for _, key := range config.LogQueryParams {
					if values, ok := queryValues[key]; ok {
						v.QueryParams[key] = append([]string(nil), values...)
					}
				}
			}
		}

		if config.LogRoutePath {
			v.RoutePath = c.FullPath()
		}
		if config.LogRequestID {
			requestID := c.RequestID()
			if requestID == "" && rw != nil {
				requestID = rw.Header().Get(zinc.HeaderXRequestID)
			}
			v.RequestID = requestID
		}
		if config.LogStatus {
			v.Status = resolveRequestLogStatus(rw, logErr)
		}
		if config.LogError {
			v.Error = logErr
		}
		if config.LogResponseSize {
			if rw != nil {
				v.ResponseSize = int64(rw.BytesWritten())
			} else {
				v.ResponseSize = -1
			}
		}

		if errOnLog := config.LogValuesFunc(c, v); errOnLog != nil {
			return errOnLog
		}

		return err
	}
}

// Logger is an alias for RequestLogger.
func Logger() zinc.Middleware {
	return RequestLogger()
}

// LoggerWithConfig is an alias for RequestLoggerWithConfig.
func LoggerWithConfig(config RequestLoggerConfig) zinc.Middleware {
	return RequestLoggerWithConfig(config)
}

func defaultRequestLoggerSkipper(*zinc.Context) bool {
	return false
}

func defaultRequestLogValuesFunc(logger *slog.Logger) func(*zinc.Context, RequestLoggerValues) error {
	if logger == nil {
		logger = slog.Default()
	}
	return func(_ *zinc.Context, v RequestLoggerValues) error {
		if v.Error == nil {
			if v.RoutePath != "" {
				logger.LogAttrs(context.Background(), slog.LevelInfo, "REQUEST",
					slog.String("method", v.Method),
					slog.String("uri", v.URI),
					slog.String("route", v.RoutePath),
					slog.Int("status", v.Status),
					slog.Duration("latency", v.Latency),
					slog.String("host", v.Host),
					slog.String("bytes_in", v.ContentLength),
					slog.Int64("bytes_out", v.ResponseSize),
					slog.String("user_agent", v.UserAgent),
					slog.String("remote_ip", v.RemoteIP),
					slog.String("request_id", v.RequestID),
				)
				return nil
			}
			logger.LogAttrs(context.Background(), slog.LevelInfo, "REQUEST",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
				slog.String("host", v.Host),
				slog.String("bytes_in", v.ContentLength),
				slog.Int64("bytes_out", v.ResponseSize),
				slog.String("user_agent", v.UserAgent),
				slog.String("remote_ip", v.RemoteIP),
				slog.String("request_id", v.RequestID),
			)
			return nil
		}

		if v.RoutePath != "" {
			logger.LogAttrs(context.Background(), slog.LevelError, "REQUEST_ERROR",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.String("route", v.RoutePath),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
				slog.String("host", v.Host),
				slog.String("bytes_in", v.ContentLength),
				slog.Int64("bytes_out", v.ResponseSize),
				slog.String("user_agent", v.UserAgent),
				slog.String("remote_ip", v.RemoteIP),
				slog.String("request_id", v.RequestID),
				slog.String("error", v.Error.Error()),
			)
			return nil
		}
		logger.LogAttrs(context.Background(), slog.LevelError, "REQUEST_ERROR",
			slog.String("method", v.Method),
			slog.String("uri", v.URI),
			slog.Int("status", v.Status),
			slog.Duration("latency", v.Latency),
			slog.String("host", v.Host),
			slog.String("bytes_in", v.ContentLength),
			slog.Int64("bytes_out", v.ResponseSize),
			slog.String("user_agent", v.UserAgent),
			slog.String("remote_ip", v.RemoteIP),
			slog.String("request_id", v.RequestID),
			slog.String("error", v.Error.Error()),
		)
		return nil
	}
}

func resolveRequestLogStatus(rw zinc.ResponseWriter, err error) int {
	if rw != nil && rw.Written() {
		return rw.Status()
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
