package dataflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)


type mockTransport struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func TestFetchStockTwits(t *testing.T) {
	mockJSON := `{
		"messages": [
			{
				"body": "Bullish news on AAPL!",
				"created_at": "2026-05-22T12:00:00Z",
				"user": {"username": "trader_joe"},
				"entities": {"sentiment": {"basic": "Bullish"}}
			},
			{
				"body": "Bearish outlook.",
				"created_at": "2026-05-22T12:01:00Z",
				"user": {"username": "bear_guy"},
				"entities": {"sentiment": {"basic": "Bearish"}}
			}
		]
	}`

	httpClient := &http.Client{
		Transport: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if !strings.Contains(req.URL.Host, "api.stocktwits.com") {
					return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
				}
				resp := httptest.NewRecorder()
				resp.Header().Set("Content-Type", "application/json")
				_, _ = resp.Write([]byte(mockJSON))
				return resp.Result(), nil
			},
		},
	}

	limiter := NewTokenBucket(10, 10)
	resilientClient := NewResilientHTTPClient(httpClient, limiter, 1, 10*time.Millisecond, 50*time.Millisecond)
	provider := NewHTTPNewsSocialProvider(resilientClient)

	out, err := provider.FetchStockTwits(context.Background(), "AAPL", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Bullish: 1 (50%) · Bearish: 1 (50%)") {
		t.Errorf("expected summary not found in output: %s", out)
	}
	if !strings.Contains(out, "[2026-05-22T12:00:00Z · @trader_joe · Bullish] Bullish news on AAPL!") {
		t.Errorf("expected message body not found in output: %s", out)
	}
}

func TestFetchReddit(t *testing.T) {
	mockJSON := `{
		"data": {
			"children": [
				{
					"data": {
						"title": "AAPL to the moon",
						"score": 1500,
						"num_comments": 350,
						"created_utc": 1779542400,
						"selftext": "This is a great buying opportunity."
					}
				}
			]
		}
	}`

	httpClient := &http.Client{
		Transport: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if !strings.Contains(req.URL.Host, "www.reddit.com") {
					return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
				}
				resp := httptest.NewRecorder()
				resp.Header().Set("Content-Type", "application/json")
				_, _ = resp.Write([]byte(mockJSON))
				return resp.Result(), nil
			},
		},
	}

	limiter := NewTokenBucket(10, 10)
	resilientClient := NewResilientHTTPClient(httpClient, limiter, 1, 10*time.Millisecond, 50*time.Millisecond)
	provider := NewHTTPNewsSocialProvider(resilientClient)
	provider.interRequestDelay = 1 * time.Millisecond // Speed up test

	out, err := provider.FetchReddit(context.Background(), "AAPL", []string{"wallstreetbets"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "r/wallstreetbets — 1 recent posts mentioning AAPL:") {
		t.Errorf("expected sub block title not found: %s", out)
	}
	if !strings.Contains(out, "1500↑ · 350c") {
		t.Errorf("expected score and comments formatting not found: %s", out)
	}
	if !strings.Contains(out, "body excerpt: This is a great buying opportunity.") {
		t.Errorf("expected selftext excerpt not found: %s", out)
	}
}

func TestFetchNewsYahoo(t *testing.T) {
	// Let's mock a unix time of 2026-05-22 12:00:00 (which is 1779537600)
	mockJSON := `{
		"news": [
			{
				"uuid": "story-123",
				"title": "Apple launches new AI products",
				"publisher": "Yahoo Finance",
				"link": "https://finance.yahoo.com/news/apple-ai",
				"providerPublishTime": 1779537600,
				"summary": "Apple announced major upgrades."
			}
		]
	}`

	httpClient := &http.Client{
		Transport: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if !strings.Contains(req.URL.Host, "query2.finance.yahoo.com") {
					return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
				}
				resp := httptest.NewRecorder()
				resp.Header().Set("Content-Type", "application/json")
				_, _ = resp.Write([]byte(mockJSON))
				return resp.Result(), nil
			},
		},
	}

	limiter := NewTokenBucket(10, 10)
	resilientClient := NewResilientHTTPClient(httpClient, limiter, 1, 10*time.Millisecond, 50*time.Millisecond)
	provider := NewHTTPNewsSocialProvider(resilientClient)

	// Filter date is 2026-05-21 to 2026-05-23
	start := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)

	out, err := provider.FetchNews(context.Background(), "AAPL", start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Apple launches new AI products (source: Yahoo Finance)") {
		t.Errorf("expected news details not found: %s", out)
	}
	if !strings.Contains(out, "Apple announced major upgrades.") {
		t.Errorf("expected summary not found: %s", out)
	}

	// Test date filtering - out of bounds start date of 2026-05-24
	outFilter, err := provider.FetchNews(context.Background(), "AAPL", time.Date(2026, 5, 24, 1, 0, 0, 0, time.UTC), end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outFilter, "No news found for AAPL between") {
		t.Errorf("expected filter block to be empty: %s", outFilter)
	}
}

func TestResilientClient429Fallback(t *testing.T) {
	attempts := 0
	httpClient := &http.Client{
		Transport: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts < 3 {
					resp := httptest.NewRecorder()
					resp.WriteHeader(http.StatusTooManyRequests)
					return resp.Result(), nil
				}
				resp := httptest.NewRecorder()
				resp.Header().Set("Content-Type", "application/json")
				_, _ = resp.Write([]byte(`{"messages": []}`))
				return resp.Result(), nil
			},
		},
	}

	limiter := NewTokenBucket(10, 10)
	// Base backoff 1ms to execute quickly
	resilientClient := NewResilientHTTPClient(httpClient, limiter, 3, 1*time.Millisecond, 5*time.Millisecond)
	provider := NewHTTPNewsSocialProvider(resilientClient)

	_, err := provider.FetchStockTwits(context.Background(), "AAPL", 10)
	if err != nil {
		t.Fatalf("expected client to retry successfully after two 429 errors, but failed: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", attempts)
	}
}
