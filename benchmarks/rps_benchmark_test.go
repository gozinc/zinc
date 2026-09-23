// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

//go:build rps

package benchmarks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/0mjs/zinc"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v5"
)

// Run this optional network-throughput benchmark explicitly:
//
//	go test -tags rps -run=^$ -bench '^BenchmarkRequestsPerSecond$' -count=1
const rpsDuration = 1500 * time.Millisecond

var rpsConcurrencyLevels = []int{1, 8, 32, 128}

func rpsCases() []benchmarkCase {
	return []benchmarkCase{
		{name: "Zinc", build: buildZincRPSHandler},
		{name: "Chi", build: buildChiRPSHandler},
		{name: "Echo", build: buildEchoRPSHandler},
		{name: "Gin", build: buildGinRPSHandler},
	}
}

func buildZincRPSHandler() http.Handler {
	app := New()
	app.Get("/rps", func(c *Context) error {
		return c.String(benchmarkOKResponse)
	})
	return app
}

func buildChiRPSHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/rps", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, benchmarkOKResponse)
	})
	return r
}

func buildEchoRPSHandler() http.Handler {
	e := echo.New()
	e.GET("/rps", func(c *echo.Context) error {
		return c.String(http.StatusOK, benchmarkOKResponse)
	})
	return e
}

func buildGinRPSHandler() http.Handler {
	r := newGinBenchmarkRouter()
	r.GET("/rps", func(c *gin.Context) {
		c.String(http.StatusOK, benchmarkOKResponse)
	})
	return r
}

func BenchmarkRequestsPerSecond(b *testing.B) {
	for _, concurrency := range rpsConcurrencyLevels {
		concurrency := concurrency
		b.Run("Concurrency"+strconv.Itoa(concurrency), func(b *testing.B) {
			for _, bc := range rpsCases() {
				b.Run(bc.name, func(b *testing.B) {
					server := httptest.NewServer(bc.build())
					defer server.Close()
					measureRPS(b, server.URL+"/rps", concurrency, rpsDuration)
				})
			}
		})
	}
}

func measureRPS(b *testing.B, url string, concurrency int, duration time.Duration) {
	var (
		totalRequests int64
		wg            sync.WaitGroup
		client        = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        concurrency,
				MaxIdleConnsPerHost: concurrency,
				MaxConnsPerHost:     concurrency,
				DisableKeepAlives:   false,
			},
		}
	)
	baseReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		b.Fatalf("create base request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					req := baseReq.Clone(ctx)
					resp, err := client.Do(req)
					if err != nil {
						if !errors.Is(err, context.DeadlineExceeded) {
							b.Logf("request error: %v", err)
						}
						continue
					}

					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()

					if resp.StatusCode == http.StatusOK {
						atomic.AddInt64(&totalRequests, 1)
					}
				}
			}
		}()
	}

	wg.Wait()

	requestsPerSec := float64(totalRequests) / duration.Seconds()
	b.ReportMetric(requestsPerSec, "reqs/s")
}
