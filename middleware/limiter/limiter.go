// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package limiter bounds how fast, or how many at once, requests reach
// downstream handlers.
package limiter

import (
	"container/list"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// bucket is a concurrency-safe token bucket.
type bucket struct {
	rate       float64
	capacity   float64
	tokens     float64
	lastRefill time.Time
	mutex      sync.Mutex
}

// Config sets the token-bucket policy. The zero value allows 10 requests per
// second with bursts of 10, shared by all clients.
type Config struct {
	// Rate is the token refill rate per second. Zero uses 10.
	Rate float64
	// Capacity is the largest burst allowed. Zero uses 10.
	Capacity float64
	// Key buckets requests by the string it returns, such as c.IP(). Nil
	// applies one bucket to every request.
	Key func(*zinc.Context) string
	// LimitReached answers a request over the limit. Nil returns a 429 error.
	LimitReached zinc.HandlerFunc
	// MaxKeys bounds keyed buckets; zero uses 10,000. New keys are denied at capacity.
	MaxKeys int
	// MaxKeyBytes bounds retained key sizes; zero uses 256.
	MaxKeyBytes int
	// IdleTTL is the minimum idle time before a fully refilled bucket expires.
	// Zero uses five minutes. Active or depleted buckets are never evicted.
	IdleTTL time.Duration
	// Now supplies the clock; nil uses time.Now.
	Now func() time.Time
}

func (tb *bucket) refill(now time.Time) {
	if now.After(tb.lastRefill) {
		tb.tokens = math.Min(tb.capacity, tb.tokens+now.Sub(tb.lastRefill).Seconds()*tb.rate)
		tb.lastRefill = now
	}
}

func (tb *bucket) takeAt(now time.Time) bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()
	tb.refill(now)
	if tb.tokens < 1 {
		return false
	}
	tb.tokens--
	return true
}

func (tb *bucket) fullAt(now time.Time) bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()
	tb.refill(now)
	return tb.tokens >= tb.capacity
}

type rateLimitEntry struct {
	key    string
	bucket *bucket
	seen   time.Time
}

// New returns token-bucket rate limiting. With Key set, it keeps a bounded
// set of buckets; at MaxKeys it rejects new keys rather than evicting active
// quotas. No cleanup goroutine is required.
func New(configs ...Config) zinc.Middleware {
	cfg := shared.Config("limiter", configs)
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
	if cfg.Rate <= 0 || math.IsNaN(cfg.Rate) || math.IsInf(cfg.Rate, 0) ||
		cfg.Capacity < 1 || math.IsNaN(cfg.Capacity) || math.IsInf(cfg.Capacity, 0) ||
		cfg.MaxKeys < 1 || cfg.MaxKeyBytes < 1 || cfg.IdleTTL < 0 {
		panic("limiter: invalid configuration")
	}
	if cfg.LimitReached == nil {
		cfg.LimitReached = func(*zinc.Context) error { return zinc.TooManyRequests("rate limit exceeded") }
	}
	newBucket := func(now time.Time) *bucket {
		return &bucket{rate: cfg.Rate, capacity: cfg.Capacity, tokens: cfg.Capacity, lastRefill: now}
	}
	global := newBucket(cfg.Now())
	keyFor := cfg.Key
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
			return cfg.LimitReached(c)
		}
		return c.Next()
	}
}

// Concurrency bounds the requests running downstream at once to limit and
// answers 429 to any beyond it, rather than queueing them.
func Concurrency(limit int) zinc.Middleware {
	if limit <= 0 {
		panic("limiter: Concurrency limit must be greater than zero")
	}
	sem := make(chan struct{}, limit)
	return func(c *zinc.Context) error {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			return c.Next()
		default:
			return zinc.ErrTooManyRequests
		}
	}
}
