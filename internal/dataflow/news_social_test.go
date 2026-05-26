package dataflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)


type mockTransport struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func newTestNewsSocialProvider(roundTrip func(req *http.Request) (*http.Response, error)) *HTTPNewsSocialProvider {
	httpClient := &http.Client{Transport: &mockTransport{roundTrip: roundTrip}}
	limiter := NewTokenBucket(10, 10)
	resilientClient := NewResilientHTTPClient(httpClient, limiter, 1, 10*time.Millisecond, 50*time.Millisecond)
	return NewHTTPNewsSocialProvider(resilientClient)
}

func httpJSONResponse(status int, body string) *http.Response {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	if status > 0 {
		rec.WriteHeader(status)
	}
	_, _ = rec.WriteString(body)
	return rec.Result()
}

func countMarkdownH3(out string) int {
	return strings.Count(out, "### ")
}

func extractMarkdownTitles(out string) []string {
	var titles []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "### ") {
			continue
		}
		title := strings.TrimPrefix(line, "### ")
		if idx := strings.Index(title, " (source:"); idx >= 0 {
			title = title[:idx]
		}
		titles = append(titles, title)
	}
	return titles
}

var stockTwitsSummaryRE = regexp.MustCompile(
	`^Bullish: (\d+) \((\d+)%\) · Bearish: (\d+) \((\d+)%\) · Unlabeled: (\d+) · Total: (\d+) most-recent messages$`,
)

type parsedStockTwitsSummary struct {
	Bullish, BullPct, Bearish, BearPct, Unlabeled, Total int
}

func parseStockTwitsSummary(out string) (parsedStockTwitsSummary, bool) {
	firstLine, _, ok := strings.Cut(out, "\n")
	if !ok {
		firstLine = out
	}
	m := stockTwitsSummaryRE.FindStringSubmatch(strings.TrimSpace(firstLine))
	if m == nil {
		return parsedStockTwitsSummary{}, false
	}
	parse := func(s string) int {
		n, _ := strconv.Atoi(s)
		return n
	}
	return parsedStockTwitsSummary{
		Bullish:   parse(m[1]),
		BullPct:   parse(m[2]),
		Bearish:   parse(m[3]),
		BearPct:   parse(m[4]),
		Unlabeled: parse(m[5]),
		Total:     parse(m[6]),
	}, true
}

func stockTwitsMessageLines(out string) []string {
	_, rest, ok := strings.Cut(out, "\n\n")
	if !ok {
		return nil
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	return strings.Split(rest, "\n")
}

var globalNewsHeaderRE = regexp.MustCompile(
	`^## Global Market News, from (\d{4}-\d{2}-\d{2}) to (\d{4}-\d{2}-\d{2}):$`,
)

func parseGlobalNewsHeader(out string) (start, end string, ok bool) {
	firstLine, _, found := strings.Cut(out, "\n")
	if !found {
		return "", "", false
	}
	m := globalNewsHeaderRE.FindStringSubmatch(strings.TrimSpace(firstLine))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
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

func TestFetchGlobalNews(t *testing.T) {
	currDate := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	inWindow := currDate.Unix()
	future := currDate.AddDate(0, 0, 2).Unix()

	t.Run("happy path formats header and articles", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Host, "query2.finance.yahoo.com") {
				return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
			}
			body := fmt.Sprintf(`{"news":[{"uuid":"1","title":"Fed holds rates steady","publisher":"Reuters","link":"https://example.com/1","providerPublishTime":%d,"summary":"No change."}]}`, inWindow)
			return httpJSONResponse(http.StatusOK, body), nil
		})

		out, err := provider.FetchGlobalNews(context.Background(), currDate, 7, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		start, end, ok := parseGlobalNewsHeader(out)
		if !ok {
			t.Fatalf("expected structured global news header, got: %q", out)
		}
		if start != "2026-05-15" || end != "2026-05-22" {
			t.Errorf("header dates: start=%s end=%s", start, end)
		}
		titles := extractMarkdownTitles(out)
		if len(titles) != 1 || titles[0] != "Fed holds rates steady" {
			t.Errorf("unexpected titles: %v", titles)
		}
	})

	t.Run("deduplicates duplicate titles across queries", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			body := fmt.Sprintf(`{"news":[{"uuid":"dup","title":"Same headline","publisher":"A","providerPublishTime":%d}]}`, inWindow)
			return httpJSONResponse(http.StatusOK, body), nil
		})

		out, err := provider.FetchGlobalNews(context.Background(), currDate, 7, 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		titles := extractMarkdownTitles(out)
		if len(titles) != 1 {
			t.Errorf("expected 1 deduplicated title, got %d: %v", len(titles), titles)
		}
		if countMarkdownH3(out) != 1 {
			t.Errorf("expected exactly one article block, h3 count=%d", countMarkdownH3(out))
		}
	})

	t.Run("respects limit cap", func(t *testing.T) {
		var items []string
		for i := 0; i < 8; i++ {
			items = append(items, fmt.Sprintf(
				`{"uuid":"%d","title":"Article %d","publisher":"Wire","providerPublishTime":%d}`,
				i, i, inWindow,
			))
		}
		payload := `{"news":[` + strings.Join(items, ",") + `]}`

		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			return httpJSONResponse(http.StatusOK, payload), nil
		})

		out, err := provider.FetchGlobalNews(context.Background(), currDate, 7, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if countMarkdownH3(out) != 3 {
			t.Errorf("expected limit cap of 3 articles, got %d", countMarkdownH3(out))
		}
	})

	t.Run("lookbackDays sets header start date", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			body := fmt.Sprintf(`{"news":[{"uuid":"1","title":"Macro update","publisher":"Bloomberg","providerPublishTime":%d}]}`, inWindow)
			return httpJSONResponse(http.StatusOK, body), nil
		})

		out, err := provider.FetchGlobalNews(context.Background(), currDate, 14, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		start, end, ok := parseGlobalNewsHeader(out)
		if !ok {
			t.Fatalf("missing header: %q", out)
		}
		if start != "2026-05-08" || end != "2026-05-22" {
			t.Errorf("lookback header: start=%s end=%s", start, end)
		}
	})

	t.Run("excludes future dated articles", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			body := fmt.Sprintf(`{"news":[
				{"uuid":"f","title":"Future leak","publisher":"X","providerPublishTime":%d},
				{"uuid":"p","title":"Present story","publisher":"Y","providerPublishTime":%d}
			]}`, future, inWindow)
			return httpJSONResponse(http.StatusOK, body), nil
		})

		out, err := provider.FetchGlobalNews(context.Background(), currDate, 7, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		titles := extractMarkdownTitles(out)
		if len(titles) != 1 || titles[0] != "Present story" {
			t.Errorf("future article should be excluded, titles=%v", titles)
		}
	})

	t.Run("empty fallback message", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			return httpJSONResponse(http.StatusOK, `{"news":[]}`), nil
		})

		out, err := provider.FetchGlobalNews(context.Background(), currDate, 7, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "No global news found for 2026-05-22"
		if out != want {
			t.Errorf("empty fallback: got %q want %q", out, want)
		}
		if countMarkdownH3(out) != 0 {
			t.Errorf("empty result should have no articles")
		}
	})
}

func TestFetchNewsHTTPErrors(t *testing.T) {
	start := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantPrefix string
	}{
		{
			name:       "200 empty body",
			status:     http.StatusOK,
			body:       "",
			wantErr:    true,
			wantPrefix: "<news unavailable: JSON decode error",
		},
		{
			name:       "404",
			status:     http.StatusNotFound,
			body:       `{"news":[]}`,
			wantErr:    true,
			wantPrefix: "<news unavailable: status 404>",
		},
		{
			name:       "500",
			status:     http.StatusInternalServerError,
			body:       `error`,
			wantErr:    true,
			wantPrefix: "<news unavailable: status 500>",
		},
		{
			name:       "malformed JSON",
			status:     http.StatusOK,
			body:       `{not-json`,
			wantErr:    true,
			wantPrefix: "<news unavailable: JSON decode error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
				if !strings.Contains(req.URL.Host, "query2.finance.yahoo.com") {
					return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
				}
				return httpJSONResponse(tc.status, tc.body), nil
			})

			out, err := provider.FetchNews(context.Background(), "AAPL", start, end)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !strings.HasPrefix(out, tc.wantPrefix) {
				t.Errorf("output prefix: got %q want prefix %q", out, tc.wantPrefix)
			}
		})
	}

	t.Run("200 empty news array", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			return httpJSONResponse(http.StatusOK, `{"news":[]}`), nil
		})
		out, err := provider.FetchNews(context.Background(), "AAPL", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "No news found for AAPL" {
			t.Errorf("empty feed message: got %q", out)
		}
	})
}

func TestFetchStockTwitsHTTPErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantPrefix string
	}{
		{name: "200 empty body", status: http.StatusOK, body: "", wantErr: true, wantPrefix: "<stocktwits unavailable: JSON decode error"},
		{name: "404", status: http.StatusNotFound, body: `{}`, wantErr: true, wantPrefix: "<stocktwits unavailable: status 404>"},
		{name: "500", status: http.StatusInternalServerError, body: `err`, wantErr: true, wantPrefix: "<stocktwits unavailable: status 500>"},
		{name: "malformed JSON", status: http.StatusOK, body: `{bad`, wantErr: true, wantPrefix: "<stocktwits unavailable: JSON decode error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
				if !strings.Contains(req.URL.Host, "api.stocktwits.com") {
					return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
				}
				return httpJSONResponse(tc.status, tc.body), nil
			})

			out, err := provider.FetchStockTwits(context.Background(), "AAPL", 10)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !strings.HasPrefix(out, tc.wantPrefix) {
				t.Errorf("output prefix: got %q want prefix %q", out, tc.wantPrefix)
			}
		})
	}
}

func TestFetchStockTwitsEmptyAndTruncation(t *testing.T) {
	t.Run("empty messages array", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			return httpJSONResponse(http.StatusOK, `{"messages":[]}`), nil
		})

		out, err := provider.FetchStockTwits(context.Background(), "AAPL", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "<no StockTwits messages found for $AAPL>" {
			t.Errorf("empty messages output: %q", out)
		}
		if _, ok := parseStockTwitsSummary(out); ok {
			t.Error("empty output should not parse as summary")
		}
	})

	t.Run("truncates long body to 280 runes", func(t *testing.T) {
		longBody := strings.Repeat("字", 300)
		payload := fmt.Sprintf(`{"messages":[{"body":%q,"created_at":"2026-05-22T12:00:00Z","user":{"username":"long"},"entities":{}}]}`, longBody)

		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			return httpJSONResponse(http.StatusOK, payload), nil
		})

		out, err := provider.FetchStockTwits(context.Background(), "AAPL", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		summary, ok := parseStockTwitsSummary(out)
		if !ok {
			t.Fatalf("expected parsed summary line: %q", out)
		}
		if summary.Total != 1 || summary.Unlabeled != 1 {
			t.Errorf("summary counts: %+v", summary)
		}
		lines := stockTwitsMessageLines(out)
		if len(lines) != 1 {
			t.Fatalf("expected 1 message line, got %d", len(lines))
		}
		msgBody := strings.TrimPrefix(lines[0], "[2026-05-22T12:00:00Z · @long · no-label] ")
		if !strings.HasSuffix(msgBody, "…") {
			t.Errorf("expected truncation ellipsis in body")
		}
		runes := utf8.RuneCountInString(strings.TrimSuffix(msgBody, "…"))
		if runes != 280 {
			t.Errorf("truncated body rune length: got %d want 280", runes)
		}
	})
}

func TestFetchRedditHTTPErrors(t *testing.T) {
	okJSON := `{"data":{"children":[{"data":{"title":"AAPL mention","score":5,"num_comments":1,"created_utc":1779542400,"selftext":""}}]}}`

	t.Run("malformed JSON on failing subreddit", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/r/wallstreetbets/") {
				return httpJSONResponse(http.StatusOK, okJSON), nil
			}
			if strings.Contains(req.URL.Path, "/r/stocks/") {
				return httpJSONResponse(http.StatusOK, `{bad-json`), nil
			}
			return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
		})
		provider.interRequestDelay = time.Millisecond

		out, err := provider.FetchReddit(context.Background(), "AAPL", []string{"wallstreetbets", "stocks"}, 5)
		if err != nil {
			t.Fatalf("unexpected top-level error: %v", err)
		}
		if !strings.Contains(out, "r/stocks: <reddit unavailable:") {
			t.Errorf("expected subreddit error block, got: %s", out)
		}
		if !strings.Contains(out, "r/wallstreetbets — 1 recent posts") {
			t.Errorf("expected successful subreddit block, got: %s", out)
		}
	})

	t.Run("404 on failing subreddit", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/r/wallstreetbets/") {
				return httpJSONResponse(http.StatusOK, okJSON), nil
			}
			return httpJSONResponse(http.StatusNotFound, `{"error":true}`), nil
		})
		provider.interRequestDelay = time.Millisecond

		out, err := provider.FetchReddit(context.Background(), "AAPL", []string{"wallstreetbets", "investing"}, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "server responded with status 404") {
			t.Errorf("expected 404 in unavailable block: %s", out)
		}
	})

	t.Run("all subreddits fail returns aggregate empty message", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			return httpJSONResponse(http.StatusInternalServerError, `{}`), nil
		})
		provider.interRequestDelay = time.Millisecond

		out, err := provider.FetchReddit(context.Background(), "AAPL", []string{"stocks"}, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "<no Reddit posts found mentioning AAPL across stocks in the past 7 days>"
		if out != want {
			t.Errorf("aggregate empty message: got %q want %q", out, want)
		}
	})
}

func TestFetchRedditPartialFailureAndDefaults(t *testing.T) {
	okJSON := `{"data":{"children":[{"data":{"title":"AAPL win","score":10,"num_comments":2,"created_utc":1779542400,"selftext":"ok"}}]}}`

	t.Run("partial subreddit failure aggregates successes", func(t *testing.T) {
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/r/wallstreetbets/") {
				return httpJSONResponse(http.StatusOK, okJSON), nil
			}
			if strings.Contains(req.URL.Path, "/r/stocks/") {
				return httpJSONResponse(http.StatusInternalServerError, `{}`), nil
			}
			return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
		})
		provider.interRequestDelay = time.Millisecond

		out, err := provider.FetchReddit(context.Background(), "AAPL", []string{"wallstreetbets", "stocks"}, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "r/wallstreetbets — 1 recent posts mentioning AAPL:") {
			t.Errorf("missing success block: %s", out)
		}
		if !strings.Contains(out, "r/stocks: <reddit unavailable:") {
			t.Errorf("missing failure block: %s", out)
		}
		if !strings.Contains(out, "AAPL win") {
			t.Errorf("missing post title in success block")
		}
	})

	t.Run("nil subreddits uses default list", func(t *testing.T) {
		var paths []string
		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			paths = append(paths, req.URL.Path)
			return httpJSONResponse(http.StatusOK, `{"data":{"children":[]}}`), nil
		})
		provider.interRequestDelay = time.Millisecond

		out, err := provider.FetchReddit(context.Background(), "AAPL", nil, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "<no Reddit posts found mentioning AAPL across wallstreetbets, stocks, investing in the past 7 days>"
		if out != want {
			t.Errorf("nil subreddits aggregate message: got %q want %q", out, want)
		}
		if len(paths) != 3 {
			t.Errorf("expected 3 subreddit requests, got %d: %v", len(paths), paths)
		}
	})

	t.Run("truncates selftext excerpt to 240 runes", func(t *testing.T) {
		longText := strings.Repeat("a", 300)
		payload := fmt.Sprintf(`{"data":{"children":[{"data":{"title":"Long post","score":1,"num_comments":0,"created_utc":1779542400,"selftext":%q}}]}}`, longText)

		provider := newTestNewsSocialProvider(func(req *http.Request) (*http.Response, error) {
			return httpJSONResponse(http.StatusOK, payload), nil
		})
		provider.interRequestDelay = time.Millisecond

		out, err := provider.FetchReddit(context.Background(), "AAPL", []string{"stocks"}, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		const prefix = "    body excerpt: "
		idx := strings.Index(out, prefix)
		if idx < 0 {
			t.Fatalf("missing body excerpt in: %s", out)
		}
		excerpt := strings.TrimSpace(out[idx+len(prefix):])
		if !strings.HasSuffix(excerpt, "…") {
			t.Errorf("expected ellipsis truncation, got len=%d", len(excerpt))
		}
		runes := utf8.RuneCountInString(strings.TrimSuffix(excerpt, "…"))
		if runes != 240 {
			t.Errorf("excerpt rune length: got %d want 240", runes)
		}
	})
}
