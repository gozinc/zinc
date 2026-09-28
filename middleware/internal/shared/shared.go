// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package shared holds helpers used by more than one Zinc middleware package.
package shared

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/0mjs/zinc"
)

// Config returns the configuration passed to a middleware's New, or the zero
// value when there is none, so every field falls back to its default.
func Config[T any](name string, configs []T) T {
	switch len(configs) {
	case 0:
		var zero T
		return zero
	case 1:
		return configs[0]
	default:
		panic(fmt.Sprintf("%s: New takes at most one Config", name))
	}
}

// ResponseStatus is the status a request ended with: the one written, or the
// one the error handler sends for err.
func ResponseStatus(rw zinc.ResponseWriter, err error) int {
	if rw != nil && rw.Written() {
		return rw.Status()
	}
	if err != nil {
		return zinc.StatusCode(err)
	}
	return zinc.StatusOK
}

// CloneRewriteRules copies rules, dropping empty patterns.
func CloneRewriteRules(rules map[string]string) map[string]string {
	out := make(map[string]string, len(rules))
	for from, to := range rules {
		if from == "" {
			continue
		}
		out[from] = to
	}
	return out
}

// RewriteTarget applies the first matching rule to path. A pattern ending in
// "*" matches a prefix; a "*" in the target is replaced by the matched tail.
func RewriteTarget(path string, rules map[string]string) (string, bool) {
	if len(rules) == 0 {
		return "", false
	}
	if target, ok := rules[path]; ok {
		return target, true
	}
	for from, to := range rules {
		if !strings.HasSuffix(from, "*") {
			continue
		}
		prefix := strings.TrimSuffix(from, "*")
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		tail := strings.TrimPrefix(path, prefix)
		if strings.Contains(to, "*") {
			return strings.Replace(to, "*", tail, 1), true
		}
		return to + tail, true
	}
	return "", false
}

// PathWithRawQuery appends rawQuery to path.
func PathWithRawQuery(path, rawQuery string) string {
	if rawQuery == "" {
		return path
	}
	return path + "?" + rawQuery
}

// RoutingWarning warns once when middleware that changes which route a
// request takes, such as redirect or rewrite, runs after a route was already
// chosen. On a group Zinc refuses it at registration, so this catches the
// remaining case, a single route: it works for that path only, and a rewrite
// can no longer change the route.
type RoutingWarning struct {
	name   string
	warned atomic.Bool
}

// NewRoutingWarning returns a RoutingWarning for the middleware called name.
func NewRoutingWarning(name string) *RoutingWarning {
	return &RoutingWarning{name: name}
}

// Check logs the warning the first time c has a matched route.
func (w *RoutingWarning) Check(c *zinc.Context) {
	if w.warned.Load() {
		return
	}
	route := c.FullPath()
	if route == "" || !w.warned.CompareAndSwap(false, true) {
		return
	}
	slog.Warn(w.name+" middleware runs after routing, so it only sees requests that already matched a route; register it with app.Use or app.UsePrefix",
		slog.String("middleware", w.name), slog.String("route", route))
}
