// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package shared holds helpers used by more than one Zinc middleware package.
package shared

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
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

// Rules is a compiled set of path rules, as used by rewrite, redirect and
// proxy. A pattern ending in "*" matches any path starting with the text
// before the "*"; any other pattern matches one path exactly.
//
// When several rules match a path, the winner is fixed when the rules are
// compiled, not by map order: an exact rule beats every "*" rule, and among
// "*" rules the one with the longest literal prefix wins. Two different
// prefixes that both match a path can't have the same length, so there are
// no ties to break.
type Rules struct {
	exact    map[string]string
	prefixes []prefixRule
}

type prefixRule struct {
	prefix string
	to     string
}

// CompileRules copies rules into precedence order, dropping empty patterns.
func CompileRules(rules map[string]string) *Rules {
	r := &Rules{}
	for from, to := range rules {
		if from == "" {
			continue
		}
		if prefix, ok := strings.CutSuffix(from, "*"); ok {
			r.prefixes = append(r.prefixes, prefixRule{prefix: prefix, to: to})
			continue
		}
		if r.exact == nil {
			r.exact = make(map[string]string)
		}
		r.exact[from] = to
	}
	// Longest prefix first; the lexical order only makes the slice itself
	// stable, since equal-length prefixes never both match one path.
	slices.SortFunc(r.prefixes, func(a, b prefixRule) int {
		if c := cmp.Compare(len(b.prefix), len(a.prefix)); c != 0 {
			return c
		}
		return strings.Compare(a.prefix, b.prefix)
	})
	return r
}

// Len reports how many rules were compiled.
func (r *Rules) Len() int {
	return len(r.exact) + len(r.prefixes)
}

// Rewrite applies the winning rule to path. A "*" in the target is replaced
// by the part of path after the matched prefix, or that part is appended
// when the target has no "*".
func (r *Rules) Rewrite(path string) (string, bool) {
	to, tail, prefix, ok := r.Match(path)
	if !ok {
		return "", false
	}
	if !prefix {
		return to, true
	}
	return ExpandTarget(to, tail), true
}

// Match finds the winning rule for path. It returns the rule's target, the
// part of path after a prefix pattern, and whether a prefix pattern matched
// rather than an exact one.
func (r *Rules) Match(path string) (to, tail string, prefix, ok bool) {
	if to, ok := r.exact[path]; ok {
		return to, "", false, true
	}
	for _, rule := range r.prefixes {
		if rest, found := strings.CutPrefix(path, rule.prefix); found {
			return rule.to, rest, true, true
		}
	}
	return "", "", false, false
}

// ExpandTarget replaces the first "*" in to with tail, or appends tail when
// to has none.
func ExpandTarget(to, tail string) string {
	if strings.Contains(to, "*") {
		return strings.Replace(to, "*", tail, 1)
	}
	return to + tail
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
