// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package proxy forwards requests to upstream servers.
package proxy

import (
	"cmp"
	"context"
	"errors"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Target names an absolute upstream URL.
type Target struct {
	Name string
	URL  *url.URL
}

// Balancer selects an upstream for each request.
type Balancer interface {
	Next(*zinc.Context) (*Target, error)
}

// Config controls upstream selection, rewriting, retries, and transport. One
// of Target, Targets, or Balancer is required.
type Config struct {
	// Target is one absolute upstream URL.
	Target   string
	Targets  []*Target
	Balancer Balancer
	// Director runs after hop-by-hop and inbound forwarding headers are removed.
	Director       func(*http.Request)
	ModifyResponse func(*http.Response) error
	ErrorHandler   func(http.ResponseWriter, *http.Request, error)
	Retries        int
	RetryFilter    func(*zinc.Context, error) bool
	// Rewrite maps an upstream path, after the target's own path is added,
	// to the path to send instead. A path ending in "*" matches a prefix,
	// and a "*" in the target is replaced by the rest of the path. When
	// several rules match, an exact rule wins over every "*" rule, and among
	// "*" rules the longest prefix wins. The order is fixed when New runs.
	Rewrite map[string]string
	// RegexRewrite holds regular-expression path rules, tried when no
	// Rewrite rule matches; the target may use $1-style references. A map
	// has no order, so New puts the rules in one: the longest pattern
	// source is tried first, then patterns in lexical order, and the first
	// match wins. New panics when two rules have the same pattern source.
	RegexRewrite map[*regexp.Regexp]string
	Transport    http.RoundTripper
}

// New forwards matching requests to an upstream through an
// httputil.ReverseProxy and ends the chain there. Custom directors and
// transports run with the same trust as application code.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("proxy", configs)
	proxy, balancer := newReverseProxy(config)

	return func(c *zinc.Context) error {
		if balancer != nil {
			target, err := balancer.Next(c)
			if err != nil {
				return err
			}
			c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), proxyTargetContextKey{}, target)))
		}
		if config.Retries > 0 {
			c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), proxyContextKey{}, c)))
		}
		proxy.ServeHTTP(c.Writer(), c.Request())
		return nil
	}
}

// NewRoundRobinBalancer returns a concurrency-safe round-robin balancer.
func NewRoundRobinBalancer(targets []*Target) Balancer {
	return &roundRobinProxyBalancer{targets: normalizeProxyTargets(targets)}
}

// NewRandomBalancer returns a concurrency-safe pseudo-random balancer.
func NewRandomBalancer(targets []*Target) Balancer {
	return &randomProxyBalancer{
		targets: normalizeProxyTargets(targets),
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func newReverseProxy(config Config) (*httputil.ReverseProxy, Balancer) {
	if config.Retries < 0 {
		panic("proxy: Retries must not be negative")
	}
	baseTarget, balancer := resolveProxyTargets(config)
	rewriteRules := shared.CompileRules(config.Rewrite)
	regexRewriteRules := compileRegexRewriteRules(config.RegexRewrite)

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			req := pr.Out
			target := baseTarget
			if selected, ok := req.Context().Value(proxyTargetContextKey{}).(*Target); ok && selected != nil {
				target = selected
			}
			if target == nil || target.URL == nil {
				return
			}

			rewriteProxyURL(req, target.URL, rewriteRules, regexRewriteRules)
			pr.SetXForwarded()
			if config.Director != nil {
				config.Director(req)
			}
		},
	}
	proxy.ModifyResponse = config.ModifyResponse
	if config.Transport != nil {
		proxy.Transport = config.Transport
	}
	if config.Retries > 0 {
		proxy.Transport = &proxyRetryTransport{
			base:    proxy.Transport,
			retries: config.Retries,
			filter:  config.RetryFilter,
		}
	}
	if config.ErrorHandler != nil {
		proxy.ErrorHandler = config.ErrorHandler
	}
	return proxy, balancer
}

type proxyTargetContextKey struct{}

type proxyContextKey struct{}

type roundRobinProxyBalancer struct {
	targets []*Target
	next    uint64
}

func (b *roundRobinProxyBalancer) Next(*zinc.Context) (*Target, error) {
	if len(b.targets) == 0 {
		return nil, errors.New("proxy: no proxy targets configured")
	}
	index := atomic.AddUint64(&b.next, 1) - 1
	return b.targets[index%uint64(len(b.targets))], nil
}

type randomProxyBalancer struct {
	targets []*Target
	mu      sync.Mutex
	rand    *rand.Rand
}

func (b *randomProxyBalancer) Next(*zinc.Context) (*Target, error) {
	if len(b.targets) == 0 {
		return nil, errors.New("proxy: no proxy targets configured")
	}
	b.mu.Lock()
	index := b.rand.Intn(len(b.targets))
	b.mu.Unlock()
	return b.targets[index], nil
}

func resolveProxyTargets(config Config) (*Target, Balancer) {
	var base *Target
	if config.Target != "" {
		base = parseProxyTarget("", config.Target)
	}

	targets := normalizeOptionalProxyTargets(config.Targets)
	if len(targets) > 0 && base == nil {
		base = targets[0]
	}

	if config.Balancer != nil {
		return base, config.Balancer
	}
	if len(targets) > 0 {
		return base, &roundRobinProxyBalancer{targets: targets}
	}
	if base == nil {
		panic("proxy: Target or Targets is required")
	}
	return base, nil
}

func parseProxyTarget(name, rawURL string) *Target {
	target, err := url.Parse(rawURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		panic("proxy: Target must be a valid absolute URL")
	}
	return &Target{Name: name, URL: target}
}

func normalizeProxyTargets(targets []*Target) []*Target {
	out := normalizeOptionalProxyTargets(targets)
	if len(out) == 0 {
		panic("proxy: at least one proxy target is required")
	}
	return out
}

func normalizeOptionalProxyTargets(targets []*Target) []*Target {
	// Clone caller-owned URLs: reverse proxy directors mutate requests and must
	// not make target configuration vulnerable to later external mutation.
	out := make([]*Target, 0, len(targets))
	for _, target := range targets {
		if target == nil || target.URL == nil || target.URL.Scheme == "" || target.URL.Host == "" {
			panic("proxy: proxy targets must have absolute URLs")
		}
		targetCopy := *target
		urlCopy := *target.URL
		targetCopy.URL = &urlCopy
		out = append(out, &targetCopy)
	}
	return out
}

type regexRule struct {
	from *regexp.Regexp
	to   string
}

// compileRegexRewriteRules puts regular-expression rules in a fixed order:
// longest pattern source first, then lexical. Two rules with the same source
// would leave the winner to map order, so they are refused.
func compileRegexRewriteRules(rules map[*regexp.Regexp]string) []regexRule {
	out := make([]regexRule, 0, len(rules))
	for from, to := range rules {
		if from == nil {
			continue
		}
		out = append(out, regexRule{from: from, to: to})
	}
	slices.SortFunc(out, func(a, b regexRule) int {
		as, bs := a.from.String(), b.from.String()
		if c := cmp.Compare(len(bs), len(as)); c != 0 {
			return c
		}
		return strings.Compare(as, bs)
	})
	for i := 1; i < len(out); i++ {
		if out[i].from.String() == out[i-1].from.String() {
			panic("proxy: RegexRewrite has two rules with the pattern " + strconv.Quote(out[i].from.String()))
		}
	}
	return out
}

func rewriteProxyURL(req *http.Request, target *url.URL, rewriteRules *shared.Rules, regexRewriteRules []regexRule) {
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.URL.Path, req.URL.RawPath = joinProxyPaths(target, req.URL)
	req.URL.Path = applyProxyRewrite(req.URL.Path, rewriteRules, regexRewriteRules)
	req.URL.RawPath = ""
	if target.RawQuery == "" || req.URL.RawQuery == "" {
		req.URL.RawQuery = target.RawQuery + req.URL.RawQuery
	} else {
		req.URL.RawQuery = target.RawQuery + "&" + req.URL.RawQuery
	}
}

func applyProxyRewrite(path string, rewriteRules *shared.Rules, regexRewriteRules []regexRule) string {
	if target, ok := rewriteRules.Rewrite(path); ok {
		return target
	}
	for _, rule := range regexRewriteRules {
		if rule.from.MatchString(path) {
			return rule.from.ReplaceAllString(path, rule.to)
		}
	}
	return path
}

func joinProxyPaths(target, request *url.URL) (string, string) {
	if target.Path == "" {
		return request.Path, ""
	}
	if request.Path == "" {
		return target.Path, ""
	}
	return singleJoiningSlash(target.Path, request.Path), ""
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}

type proxyRetryTransport struct {
	base    http.RoundTripper
	retries int
	filter  func(*zinc.Context, error) bool
}

func (t *proxyRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	var c *zinc.Context
	if value, ok := req.Context().Value(proxyContextKey{}).(*zinc.Context); ok {
		c = value
	}
	canReplayBody := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil

	var lastErr error
	// Never retry a consumed body without GetBody. Replaying a partial request
	// would silently change application semantics at the upstream.
	for attempt := 0; attempt <= t.retries; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}

		resp, err := base.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		retry := false
		if t.filter != nil {
			retry = t.filter(c, err)
		} else {
			switch req.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace, http.MethodPut, http.MethodDelete:
				retry = true
			}
		}
		if attempt == t.retries || !canReplayBody || !retry {
			break
		}
	}
	return nil, lastErr
}
