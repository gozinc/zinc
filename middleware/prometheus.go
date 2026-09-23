// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0mjs/zinc"
)

// PrometheusConfig controls request metric collection.
type PrometheusConfig struct {
	Skipper func(*zinc.Context) bool
	Metrics *PrometheusMetrics
	Now     func() time.Time
}

// PrometheusMetrics stores in-process counters and duration sums. Route labels
// use registered patterns to avoid cardinality growth from path parameters.
type PrometheusMetrics struct {
	mu       sync.Mutex
	requests map[prometheusRequestKey]*prometheusRequestValue
}

type prometheusRequestKey struct {
	Method string
	Route  string
	Status int
}

type prometheusRequestValue struct {
	Count       uint64
	DurationSum float64
}

var defaultPrometheusMetrics = NewPrometheusMetrics()

// NewPrometheusMetrics creates an isolated metrics registry.
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		requests: make(map[prometheusRequestKey]*prometheusRequestValue),
	}
}

// Prometheus records requests in the supplied registry or the package default.
func Prometheus(metrics ...*PrometheusMetrics) zinc.Middleware {
	cfg := PrometheusConfig{Metrics: defaultPrometheusMetrics}
	if len(metrics) > 0 {
		cfg.Metrics = metrics[0]
	}
	return PrometheusWithConfig(cfg)
}

// PrometheusWithConfig records request counts and durations after downstream work.
func PrometheusWithConfig(config PrometheusConfig) zinc.Middleware {
	cfg := resolvePrometheusConfig(config)
	now := cfg.Now

	return func(c *zinc.Context) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		baseWriter := c.Writer()
		writer := zinc.WrapResponseWriter(baseWriter)
		c.SetWriter(writer)

		start := now()
		err := c.Next()
		if err != nil {
			c.Error(err)
		}
		c.SetWriter(baseWriter)

		status := resolveRequestLogStatus(writer, c.LastError())
		route := c.FullPath()
		if route == "" {
			route = c.Path()
		}
		cfg.Metrics.Observe(c.Method(), route, status, now().Sub(start))

		return err
	}
}

// PrometheusHandler serves the registry in Prometheus text format.
func PrometheusHandler(metrics ...*PrometheusMetrics) zinc.HandlerFunc {
	m := defaultPrometheusMetrics
	if len(metrics) > 0 && metrics[0] != nil {
		m = metrics[0]
	}
	return func(c *zinc.Context) error {
		return c.Data("text/plain; version=0.0.4; charset=utf-8", []byte(m.Text()))
	}
}

// Observe records one completed request.
func (m *PrometheusMetrics) Observe(method, route string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	if route == "" {
		route = "unknown"
	}
	key := prometheusRequestKey{
		Method: method,
		Route:  route,
		Status: status,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	value := m.requests[key]
	if value == nil {
		value = &prometheusRequestValue{}
		m.requests[key] = value
	}
	value.Count++
	value.DurationSum += duration.Seconds()
}

// Text returns a consistent snapshot in Prometheus exposition format.
func (m *PrometheusMetrics) Text() string {
	if m == nil {
		return ""
	}

	type row struct {
		key   prometheusRequestKey
		value prometheusRequestValue
	}

	m.mu.Lock()
	rows := make([]row, 0, len(m.requests))
	for key, value := range m.requests {
		rows = append(rows, row{key: key, value: *value})
	}
	m.mu.Unlock()

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].key.Method != rows[j].key.Method {
			return rows[i].key.Method < rows[j].key.Method
		}
		if rows[i].key.Route != rows[j].key.Route {
			return rows[i].key.Route < rows[j].key.Route
		}
		return rows[i].key.Status < rows[j].key.Status
	})

	var out strings.Builder
	out.WriteString("# TYPE zinc_http_requests_total counter\n")
	for _, row := range rows {
		out.WriteString("zinc_http_requests_total")
		writePrometheusLabels(&out, row.key)
		out.WriteByte(' ')
		out.WriteString(strconv.FormatUint(row.value.Count, 10))
		out.WriteByte('\n')
	}
	out.WriteString("# TYPE zinc_http_request_duration_seconds summary\n")
	for _, row := range rows {
		out.WriteString("zinc_http_request_duration_seconds_sum")
		writePrometheusLabels(&out, row.key)
		out.WriteByte(' ')
		out.WriteString(strconv.FormatFloat(row.value.DurationSum, 'f', -1, 64))
		out.WriteByte('\n')
	}
	for _, row := range rows {
		out.WriteString("zinc_http_request_duration_seconds_count")
		writePrometheusLabels(&out, row.key)
		out.WriteByte(' ')
		out.WriteString(strconv.FormatUint(row.value.Count, 10))
		out.WriteByte('\n')
	}
	return out.String()
}

func resolvePrometheusConfig(config PrometheusConfig) PrometheusConfig {
	if config.Metrics == nil {
		config.Metrics = defaultPrometheusMetrics
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return config
}

func writePrometheusLabels(out *strings.Builder, key prometheusRequestKey) {
	out.WriteString(`{method="`)
	out.WriteString(escapePrometheusLabel(key.Method))
	out.WriteString(`",route="`)
	out.WriteString(escapePrometheusLabel(key.Route))
	out.WriteString(`",status="`)
	out.WriteString(strconv.Itoa(key.Status))
	out.WriteString(`"}`)
}

func escapePrometheusLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}
