// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package prometheus records request counts and durations and serves them in
// the Prometheus text format, without depending on the Prometheus client.
package prometheus

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config controls request metric collection.
type Config struct {
	// Metrics is the registry to record into. Nil creates an isolated one,
	// which Handler finds through the request context.
	Metrics *Metrics
	// Now supplies the clock; nil uses time.Now.
	Now func() time.Time
}

// Metrics stores in-process counters and duration sums. Route labels
// use registered patterns to avoid cardinality growth from path parameters.
type Metrics struct {
	mu        sync.Mutex
	maxSeries int
	dropped   uint64
	requests  map[prometheusRequestKey]*prometheusRequestValue
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

type prometheusContextKey struct{}

// NewMetrics creates an isolated registry holding at most maxSeries series;
// zero means 10,000. Further new series are dropped and counted.
func NewMetrics(maxSeries int) *Metrics {
	limit := maxSeries
	if limit == 0 {
		limit = 10000
	}
	if limit < 1 {
		panic("prometheus: maxSeries must not be negative")
	}
	return &Metrics{
		requests:  make(map[prometheusRequestKey]*prometheusRequestValue),
		maxSeries: limit,
	}
}

// New records each request's method, route pattern, status, and duration.
// Route labels use registered patterns, so path parameters cannot grow the
// number of series.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("prometheus", configs)
	cfg := resolvePrometheusConfig(config)
	now := cfg.Now

	return func(c *zinc.Context) error {
		c.Set(prometheusContextKey{}, cfg.Metrics)
		baseWriter := c.Writer()
		writer := zinc.WrapResponseWriter(baseWriter)
		c.SetWriter(writer)
		defer c.SetWriter(baseWriter)

		start := now()
		err := c.Next()
		if err != nil {
			c.HandleError(err)
		}
		c.SetWriter(baseWriter)

		status := shared.ResponseStatus(writer, c.LastError())
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Method()
		switch method {
		case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		default:
			method = "OTHER"
		}
		cfg.Metrics.Observe(method, route, status, now().Sub(start))

		return err
	}
}

// Handler serves the registry in Prometheus text format.
func Handler(metrics ...*Metrics) zinc.HandlerFunc {
	var m *Metrics
	if len(metrics) > 0 && metrics[0] != nil {
		m = metrics[0]
	}
	return func(c *zinc.Context) error {
		registry := m
		if registry == nil {
			value, _ := c.Get(prometheusContextKey{})
			registry, _ = value.(*Metrics)
		}
		if registry == nil {
			return c.Status(http.StatusServiceUnavailable).Send("Prometheus middleware is not configured")
		}
		return c.Data("text/plain; version=0.0.4; charset=utf-8", []byte(registry.Text()))
	}
}

// Observe records one completed request.
func (m *Metrics) Observe(method, route string, status int, duration time.Duration) {
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
		if m.maxSeries == 0 {
			m.maxSeries = 10000
		}
		if len(m.requests) >= m.maxSeries {
			m.dropped++
			return
		}
		if m.requests == nil {
			m.requests = make(map[prometheusRequestKey]*prometheusRequestValue)
		}
		key.Method = strings.Clone(key.Method)
		key.Route = strings.Clone(key.Route)
		value = &prometheusRequestValue{}
		m.requests[key] = value
	}
	value.Count++
	value.DurationSum += duration.Seconds()
}

// Text returns a consistent snapshot in Prometheus exposition format.
func (m *Metrics) Text() string {
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
	dropped := m.dropped
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
	out.WriteString("# TYPE zinc_http_metrics_dropped_total counter\nzinc_http_metrics_dropped_total ")
	out.WriteString(strconv.FormatUint(dropped, 10))
	out.WriteByte('\n')
	return out.String()
}

func resolvePrometheusConfig(config Config) Config {
	if config.Metrics == nil {
		config.Metrics = NewMetrics(0)
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
