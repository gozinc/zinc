// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"container/list"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/0mjs/zinc"
)

// TokenBucket represents a concurrency-safe token bucket.
type TokenBucket struct {
	rate       float64
	capacity   float64
	tokens     float64
	lastRefill time.Time
	mutex      sync.Mutex
}

// RateLimiterConfig contains rate-limiter policy and key selection.
type RateLimiterConfig struct {
	// MaxKeys bounds keyed buckets; zero uses 10,000. New keys are denied at capacity.
	MaxKeys int
	// IdleTTL is the minimum idle time before a fully refilled bucket expires.
	// Zero uses five minutes. Active or depleted buckets are never evicted.
	IdleTTL time.Duration
	// MaxKeyBytes bounds retained key sizes; zero uses 256.
	MaxKeyBytes int
	// Now supplies the clock; nil uses time.Now.
	Now func() time.Time
	// Rate is the token refill rate per second (zero uses 10).
	Rate float64
	// Capacity is the maximum number of tokens in the bucket
	Capacity float64
	// IPLookup is the function to extract IP address for per-IP rate limiting
	// If nil, the rate limiter applies globally
	IPLookup func(*zinc.Context) string
	// KeyGenerator is a custom function to generate keys for rate limiting
	// If nil and IPLookup is set, IP is used as key
	// If both nil, global rate limiting is used
	KeyGenerator func(*zinc.Context) string
	// StatusCode is the response code when rate limit is exceeded
	StatusCode int
	// LimitReachedHandler is called when rate limit is reached
	LimitReachedHandler zinc.RouteHandler
}

// newTokenBucket creates a new token bucket rate limiter
func newTokenBucket(rate, capacity float64) *TokenBucket {
	return &TokenBucket{
		rate:       rate,
		capacity:   capacity,
		tokens:     capacity,
		lastRefill: time.Now(),
	}
}

// take attempts to take a token from the bucket
func (tb *TokenBucket) take() bool { return tb.takeAt(time.Now()) }

func (tb *TokenBucket) refill(now time.Time) {
	if now.After(tb.lastRefill) {
		tb.tokens = math.Min(tb.capacity, tb.tokens+now.Sub(tb.lastRefill).Seconds()*tb.rate)
		tb.lastRefill = now
	}
}

func (tb *TokenBucket) takeAt(now time.Time) bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()
	tb.refill(now)
	if tb.tokens < 1 {
		return false
	}
	tb.tokens--
	return true
}

func (tb *TokenBucket) fullAt(now time.Time) bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()
	tb.refill(now)
	return tb.tokens >= tb.capacity
}

type rateLimitEntry struct {
	key    string
	bucket *TokenBucket
	seen   time.Time
}

// RateLimiter returns bounded token-bucket middleware. At capacity it rejects
// new keys rather than evicting active quotas. No cleanup goroutine is required.
func RateLimiter(config ...RateLimiterConfig) zinc.Middleware {
	var cfg RateLimiterConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	if cfg.Rate == 0 {
		cfg.Rate = 10
	}
	if cfg.Capacity == 0 {
		cfg.Capacity = 10
	}
	if cfg.MaxKeys == 0 {
		cfg.MaxKeys = 10000
	}
	if cfg.MaxKeyBytes == 0 {
		cfg.MaxKeyBytes = 256
	}
	if cfg.IdleTTL == 0 {
		cfg.IdleTTL = 5 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.StatusCode == 0 {
		cfg.StatusCode = http.StatusTooManyRequests
	}
	if cfg.Rate <= 0 || math.IsNaN(cfg.Rate) || math.IsInf(cfg.Rate, 0) ||
		cfg.Capacity < 1 || math.IsNaN(cfg.Capacity) || math.IsInf(cfg.Capacity, 0) ||
		cfg.MaxKeys < 1 || cfg.MaxKeyBytes < 1 || cfg.IdleTTL < 0 || cfg.StatusCode < 400 || cfg.StatusCode > 599 {
		panic("zinc: invalid rate limiter configuration")
	}
	if cfg.LimitReachedHandler == nil {
		cfg.LimitReachedHandler = func(c *zinc.Context) error { return c.Status(cfg.StatusCode).Send("Rate limit exceeded") }
	}
	newBucket := func(now time.Time) *TokenBucket {
		return &TokenBucket{rate: cfg.Rate, capacity: cfg.Capacity, tokens: cfg.Capacity, lastRefill: now}
	}
	global := newBucket(cfg.Now())
	keyFor := cfg.KeyGenerator
	if keyFor == nil {
		keyFor = cfg.IPLookup
	}
	var mu sync.Mutex
	buckets := make(map[string]*list.Element)
	order := list.New()
	takeKey := func(key string, now time.Time) bool {
		if len(key) > cfg.MaxKeyBytes {
			return false
		}
		mu.Lock()
		defer mu.Unlock()
		// Bound cleanup work as well as retained state. Expiry never resets an
		// unrefilled quota, even when IdleTTL is shorter than the refill period.
		for n := 0; n < 16 && order.Len() > 0; n++ {
			oldest := order.Back()
			e := oldest.Value.(*rateLimitEntry)
			if now.Sub(e.seen) < cfg.IdleTTL || !e.bucket.fullAt(now) {
				break
			}
			delete(buckets, e.key)
			order.Remove(oldest)
		}
		el := buckets[key]
		if el == nil {
			if len(buckets) >= cfg.MaxKeys {
				return false
			}
			key = strings.Clone(key)
			el = order.PushFront(&rateLimitEntry{key: key, bucket: newBucket(now), seen: now})
			buckets[key] = el
		}
		e := el.Value.(*rateLimitEntry)
		e.seen = now
		order.MoveToFront(el)
		return e.bucket.takeAt(now)
	}
	return func(c *zinc.Context) error {
		now := cfg.Now()
		var allowed bool
		if keyFor == nil {
			allowed = global.takeAt(now)
		} else {
			allowed = takeKey(keyFor(c), now)
		}
		if !allowed {
			return cfg.LimitReachedHandler(c)
		}
		return c.Next()
	}
}

// DefaultRateLimiter returns a rate limiter with default settings
func DefaultRateLimiter() zinc.Middleware {
	return RateLimiter()
}

// IPRateLimiter returns a rate limiter that limits by IP address
func IPRateLimiter(rate, capacity float64) zinc.Middleware {
	return RateLimiter(RateLimiterConfig{
		Rate:     rate,
		Capacity: capacity,
		IPLookup: func(c *zinc.Context) string {
			return c.IP()
		},
	})
}
