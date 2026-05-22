package dataflow

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// TokenBucket represents a concurrency-safe rate limiter using lazy refilling.
type TokenBucket struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	refillRate float64 // Tokens generated per second
	lastRefill time.Time
}

// NewTokenBucket instantiates an active bucket.
func NewTokenBucket(capacity, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow evaluates and consumes a token if available.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// refill calculates incremental tokens based on time elapsed since last check.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	tb.tokens = math.Min(tb.capacity, tb.tokens+(elapsed*tb.refillRate))
}

// ResilientHTTPClient is a robust decorator wrapper around a standard net/http client.
type ResilientHTTPClient struct {
	client      *http.Client
	limiter     *TokenBucket
	maxRetries  int
	baseBackoff time.Duration
	maxBackoff  time.Duration
	rng         *rand.Rand
	rngMu       sync.Mutex
}

// NewResilientHTTPClient creates a resilient HTTP client wrapper.
func NewResilientHTTPClient(client *http.Client, limiter *TokenBucket, maxRetries int, base, max time.Duration) *ResilientHTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &ResilientHTTPClient{
		client:      client,
		limiter:     limiter,
		maxRetries:  maxRetries,
		baseBackoff: base,
		maxBackoff:  max,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Do executes request execution under token restriction and handles HTTP 429/503 limits dynamically.
func (c *ResilientHTTPClient) Do(req *http.Request) (*http.Response, error) {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// Acquire rate limiter permission
		for {
			if c.limiter.Allow() {
				break
			}
			// Active wait block
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(100 * time.Millisecond):
			}
		}

		resp, err := c.client.Do(req)
		if err != nil {
			if attempt == c.maxRetries {
				return nil, err
			}
			c.sleepJitter(attempt, req.Context())
			continue
		}

		// Handle server throttling or service drops
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close() // Immediately close body to conserve connections
			if attempt == c.maxRetries {
				return nil, fmt.Errorf("rate limits exceeded after %d retries: status %d", c.maxRetries, resp.StatusCode)
			}
			c.sleepJitter(attempt, req.Context())
			continue
		}

		return resp, nil
	}
	return nil, errors.New("unexpected error in HTTP retry matrix")
}

// sleepJitter sleeps for a duration defined by Full Jitter algorithm:
// Sleep = Uniform(0, Min(maxBackoff, baseBackoff * 2^attempt))
func (c *ResilientHTTPClient) sleepJitter(attempt int, ctx context.Context) {
	// Exponential scaling factor: 2^attempt
	factor := int64(1) << uint(attempt)
	temp := float64(c.baseBackoff) * float64(factor)
	if temp > float64(c.maxBackoff) {
		temp = float64(c.maxBackoff)
	}

	c.rngMu.Lock()
	jitter := c.rng.Float64() * temp
	c.rngMu.Unlock()

	sleepDur := time.Duration(jitter)

	select {
	case <-ctx.Done():
	case <-time.After(sleepDur):
	}
}
