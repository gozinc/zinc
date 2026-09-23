// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0mjs/zinc"
)

// ProxyTarget names an absolute upstream URL.
type ProxyTarget struct {
	Name string
	URL  *url.URL
}

// ProxyBalancer selects an upstream for each request.
type ProxyBalancer interface {
	Next(*zinc.Context) (*ProxyTarget, error)
}

// ProxyConfig controls upstream selection, rewriting, retries, and transport.
type ProxyConfig struct {
	Skipper        func(*zinc.Context) bool
	Target         string
	Targets        []*ProxyTarget
	Balancer       ProxyBalancer
	Director       func(*http.Request)
	Modify         func(*http.Response) error
	ModifyResponse func(*http.Response) error
	ErrorHandler   func(http.ResponseWriter, *http.Request, error)
	Retries        int
	RetryFilter    func(*zinc.Context, error) bool
	Rewrite        map[string]string
	RegexRewrite   map[*regexp.Regexp]string
	Transport      http.RoundTripper
}

// Proxy forwards requests to one absolute target URL.
func Proxy(target string) zinc.Middleware {
	return ProxyWithConfig(ProxyConfig{Target: target})
}

// ProxyWithConfig terminates matching requests at an httputil.ReverseProxy.
// Custom directors and transports run with the same trust as application code.
func ProxyWithConfig(config ProxyConfig) zinc.Middleware {
	proxy, balancer := newReverseProxy(config)

	return func(c *zinc.Context) error {
		if config.Skipper != nil && config.Skipper(c) {
			return c.Next()
		}
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
func NewRoundRobinBalancer(targets []*ProxyTarget) ProxyBalancer {
	return &roundRobinProxyBalancer{targets: normalizeProxyTargets(targets)}
}

// NewRandomBalancer returns a concurrency-safe pseudo-random balancer.
func NewRandomBalancer(targets []*ProxyTarget) ProxyBalancer {
	return &randomProxyBalancer{
		targets: normalizeProxyTargets(targets),
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func newReverseProxy(config ProxyConfig) (*httputil.ReverseProxy, ProxyBalancer) {
	baseTarget, balancer := resolveProxyTargets(config)
	rewriteRules := cloneRewriteRules(config.Rewrite)
	regexRewriteRules := cloneRegexRewriteRules(config.RegexRewrite)

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			target := baseTarget
			if selected, ok := req.Context().Value(proxyTargetContextKey{}).(*ProxyTarget); ok && selected != nil {
				target = selected
			}
			if target == nil || target.URL == nil {
				return
			}

			rewriteProxyURL(req, target.URL, rewriteRules, regexRewriteRules)
		},
	}
	if config.Director != nil {
		defaultDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			defaultDirector(req)
			config.Director(req)
		}
	}
	proxy.ModifyResponse = chainProxyModifyResponse(config.Modify, config.ModifyResponse)
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
	targets []*ProxyTarget
	next    uint64
}

func (b *roundRobinProxyBalancer) Next(*zinc.Context) (*ProxyTarget, error) {
	if len(b.targets) == 0 {
		return nil, errors.New("zincproxy: no proxy targets configured")
	}
	index := atomic.AddUint64(&b.next, 1) - 1
	return b.targets[index%uint64(len(b.targets))], nil
}

type randomProxyBalancer struct {
	targets []*ProxyTarget
	mu      sync.Mutex
	rand    *rand.Rand
}

func (b *randomProxyBalancer) Next(*zinc.Context) (*ProxyTarget, error) {
	if len(b.targets) == 0 {
		return nil, errors.New("zincproxy: no proxy targets configured")
	}
	b.mu.Lock()
	index := b.rand.Intn(len(b.targets))
	b.mu.Unlock()
	return b.targets[index], nil
}

func resolveProxyTargets(config ProxyConfig) (*ProxyTarget, ProxyBalancer) {
	var base *ProxyTarget
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
		panic("zincproxy: Target or Targets is required")
	}
	return base, nil
}

func parseProxyTarget(name, rawURL string) *ProxyTarget {
	target, err := url.Parse(rawURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		panic("zincproxy: Target must be a valid absolute URL")
	}
	return &ProxyTarget{Name: name, URL: target}
}

func normalizeProxyTargets(targets []*ProxyTarget) []*ProxyTarget {
	out := normalizeOptionalProxyTargets(targets)
	if len(out) == 0 {
		panic("zincproxy: at least one proxy target is required")
	}
	return out
}

func normalizeOptionalProxyTargets(targets []*ProxyTarget) []*ProxyTarget {
	// Clone caller-owned URLs: reverse proxy directors mutate requests and must
	// not make target configuration vulnerable to later external mutation.
	out := make([]*ProxyTarget, 0, len(targets))
	for _, target := range targets {
		if target == nil || target.URL == nil || target.URL.Scheme == "" || target.URL.Host == "" {
			panic("zincproxy: proxy targets must have absolute URLs")
		}
		targetCopy := *target
		urlCopy := *target.URL
		targetCopy.URL = &urlCopy
		out = append(out, &targetCopy)
	}
	return out
}

func cloneRegexRewriteRules(rules map[*regexp.Regexp]string) map[*regexp.Regexp]string {
	out := make(map[*regexp.Regexp]string, len(rules))
	for from, to := range rules {
		if from == nil {
			continue
		}
		out[from] = to
	}
	return out
}

func chainProxyModifyResponse(first, second func(*http.Response) error) func(*http.Response) error {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func(resp *http.Response) error {
		if err := first(resp); err != nil {
			return err
		}
		return second(resp)
	}
}

func rewriteProxyURL(req *http.Request, target *url.URL, rewriteRules map[string]string, regexRewriteRules map[*regexp.Regexp]string) {
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

func applyProxyRewrite(path string, rewriteRules map[string]string, regexRewriteRules map[*regexp.Regexp]string) string {
	if target, ok := rewriteTarget(path, rewriteRules); ok {
		return target
	}
	for from, to := range regexRewriteRules {
		if from.MatchString(path) {
			return from.ReplaceAllString(path, to)
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
		if attempt == t.retries || !canReplayBody || (t.filter != nil && !t.filter(c, err)) {
			break
		}
	}
	return nil, lastErr
}
