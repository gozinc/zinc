// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"net/http"
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
	// Rate is the token refill rate per second
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
func (tb *TokenBucket) take() bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	// Refill tokens based on elapsed time
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	// Check if we can take a token
	if tb.tokens < 1 {
		return false
	}

	// Take a token
	tb.tokens--
	return true
}

// RateLimiter returns token-bucket middleware. Keyed buckets currently remain
// for the middleware lifetime; use a bounded key source for public traffic.
func RateLimiter(config ...RateLimiterConfig) zinc.Middleware {
	// Set default config
	cfg := RateLimiterConfig{
		Rate:       10,
		Capacity:   10,
		StatusCode: http.StatusTooManyRequests,
		LimitReachedHandler: func(c *zinc.Context) error {
			return c.Status(http.StatusTooManyRequests).Send("Rate limit exceeded")
		},
	}

	if len(config) > 0 {
		cfg = config[0]
		if cfg.StatusCode == 0 {
			cfg.StatusCode = http.StatusTooManyRequests
		}
		if cfg.LimitReachedHandler == nil {
			cfg.LimitReachedHandler = func(c *zinc.Context) error {
				return c.Status(http.StatusTooManyRequests).Send("Rate limit exceeded")
			}
		}
	}

	// Store buckets by key
	var buckets sync.Map

	// Global bucket for global rate limiting
	var globalBucket = newTokenBucket(cfg.Rate, cfg.Capacity)

	// Return middleware
	return func(c *zinc.Context) error {
		var bucket *TokenBucket

		// A custom key takes precedence over IP lookup; with neither, all
		// requests share the global bucket.
		if cfg.KeyGenerator != nil {
			// Use custom key generator
			key := cfg.KeyGenerator(c)
			bucketInterface, _ := buckets.LoadOrStore(key, newTokenBucket(cfg.Rate, cfg.Capacity))
			bucket = bucketInterface.(*TokenBucket)
		} else if cfg.IPLookup != nil {
			// Use IP-based rate limiting
			ip := cfg.IPLookup(c)
			bucketInterface, _ := buckets.LoadOrStore(ip, newTokenBucket(cfg.Rate, cfg.Capacity))
			bucket = bucketInterface.(*TokenBucket)
		} else {
			// Use global rate limiting
			bucket = globalBucket
		}

		// Try to take a token
		if !bucket.take() {
			return cfg.LimitReachedHandler(c)
		}

		// Continue with the next middleware
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
