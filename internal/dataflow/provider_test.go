package dataflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFetchAndStreamOHLCV_LocalFile(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "ohlcv.csv")
	tradeDate := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	csv := strings.Join([]string{
		"Date,Open,High,Low,Close,Adj Close,Volume",
		"2026-05-18,100.00,105.00,99.00,104.00,104.00,10000",
		"2026-05-19,101.00,106.00,100.00,105.00,105.00,11000",
		"2026-05-20,102.00,107.00,101.00,106.00,106.00,12000",
		"2026-05-21,103.00,108.00,102.00,107.00,107.00,13000", // after tradeDate — filtered
	}, "\n") + "\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	reader := NewYahooFinanceCSVReader(nil, "")
	candles, err := reader.FetchAndStreamOHLCV(context.Background(), csvPath, tradeDate)
	if err != nil {
		t.Fatalf("FetchAndStreamOHLCV: %v", err)
	}
	if len(candles) != 3 {
		t.Fatalf("expected 3 candles on/before trade date, got %d", len(candles))
	}
	if candles[2].Close != 106.0 {
		t.Errorf("last candle close = %f, want 106", candles[2].Close)
	}
}

func TestFetchOHLCV_CacheHit(t *testing.T) {
	dir := t.TempDir()
	tradeDate := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	today := time.Now()
	start5Y := today.AddDate(-5, 0, 0)
	cacheFile := filepath.Join(dir, fmt.Sprintf("AAPL-YFin-data-%s-%s.csv",
		start5Y.Format("2006-01-02"), today.Format("2006-01-02")))

	csv := "Date,Open,High,Low,Close,Adj Close,Volume\n" +
		tradeDate.Format("2006-01-02") + ",100.00,105.00,99.00,104.00,104.00,10000\n"
	if err := os.WriteFile(cacheFile, []byte(csv), 0600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	reader := NewYahooFinanceCSVReader(nil, dir)
	candles, err := reader.FetchOHLCV(context.Background(), "AAPL", start5Y, today, tradeDate)
	if err != nil {
		t.Fatalf("FetchOHLCV cache hit: %v", err)
	}
	if len(candles) != 1 || candles[0].Close != 104.0 {
		t.Fatalf("unexpected candles from cache: %+v", candles)
	}
}

func TestFetchOHLCV_ChartAPI(t *testing.T) {
	chartBody := `{
		"chart": {
			"result": [{
				"meta": {"gmtoffset": 0},
				"timestamp": [1716163200, 1716249600],
				"indicators": {
					"quote": [{
						"open": [150.0, 151.0],
						"high": [155.0, 156.0],
						"low": [149.0, 150.0],
						"close": [152.0, 153.0],
						"volume": [1000000.0, 1100000.0]
					}],
					"adjclose": [{"adjclose": [152.0, 153.0]}]
				}
			}]
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v8/finance/chart/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chartBody))
	}))
	defer server.Close()

	// Redirect chart fetch to test server by replacing transport — chart URL is hardcoded to query2.finance.yahoo.com.
	// Use mock transport that rewrites host.
	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/v8/finance/chart/") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
				return http.DefaultTransport.RoundTrip(req)
			}
			return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
		},
	}

	client := NewResilientHTTPClient(&http.Client{Transport: transport}, NewTokenBucket(10, 10), 0, time.Millisecond, time.Millisecond)
	dir := t.TempDir()
	reader := NewYahooFinanceCSVReader(client, dir)

	tradeDate := time.Date(2024, 5, 21, 0, 0, 0, 0, time.UTC)
	start := tradeDate.AddDate(0, 0, -5)
	candles, err := reader.FetchOHLCV(context.Background(), "AAPL", start, tradeDate, tradeDate)
	if err != nil {
		t.Fatalf("FetchOHLCV chart API: %v", err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles from chart API, got %d", len(candles))
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "AAPL-YFin-data-*.csv"))
	if len(matches) == 0 {
		t.Fatal("expected chart response to be written to cache")
	}
}

func TestFetchOHLCV_GlobFallback(t *testing.T) {
	dir := t.TempDir()
	tradeDate := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	fallbackFile := filepath.Join(dir, "AAPL-YFin-data-2020-01-01-2026-05-26.csv")
	csv := "Date,Open,High,Low,Close,Adj Close,Volume\n" +
		tradeDate.Format("2006-01-02") + ",100.00,105.00,99.00,104.00,104.00,10000\n"
	if err := os.WriteFile(fallbackFile, []byte(csv), 0600); err != nil {
		t.Fatalf("write fallback cache: %v", err)
	}

	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/v8/finance/chart/") {
				resp := httptest.NewRecorder()
				resp.WriteHeader(http.StatusInternalServerError)
				return resp.Result(), nil
			}
			return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
		},
	}
	client := NewResilientHTTPClient(&http.Client{Transport: transport}, NewTokenBucket(10, 10), 0, time.Millisecond, time.Millisecond)
	reader := NewYahooFinanceCSVReader(client, dir)

	candles, err := reader.FetchOHLCV(context.Background(), "AAPL", tradeDate.AddDate(-1, 0, 0), tradeDate, tradeDate)
	if err != nil {
		t.Fatalf("FetchOHLCV glob fallback: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle from glob fallback, got %d", len(candles))
	}
}

func TestFetchFundamentals_CacheHit(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "MSFT-fundamentals.txt")
	want := "# Company Fundamentals for MSFT\nMarket Cap: 3000000.00\n"
	if err := os.WriteFile(cacheFile, []byte(want), 0600); err != nil {
		t.Fatalf("write fundamentals cache: %v", err)
	}

	reader := NewYahooFinanceCSVReader(nil, dir)
	got, err := reader.FetchFundamentals(context.Background(), "MSFT", time.Now())
	if err != nil {
		t.Fatalf("FetchFundamentals cache: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFetchFundamentals_Online(t *testing.T) {
	quoteBody := `{
		"quoteSummary": {
			"result": [{
				"assetProfile": {"sector": "Technology", "industry": "Software"},
				"price": {"longName": "Microsoft Corporation"},
				"financialData": {
					"totalRevenue": {"raw": 200000},
					"grossProfits": {"raw": 140000},
					"ebitda": {"raw": 100000},
					"netIncomeToCommon": {"raw": 80000},
					"profitMargins": {"raw": 0.35},
					"operatingMargins": {"raw": 0.42},
					"returnOnEquity": {"raw": 0.40},
					"returnOnAssets": {"raw": 0.18},
					"debtToEquity": {"raw": 0.35},
					"currentRatio": {"raw": 1.8},
					"freeCashflow": {"raw": 60000}
				},
				"defaultKeyStatistics": {
					"forwardPE": {"raw": 28.5},
					"pegRatio": {"raw": 1.2},
					"priceToBook": {"raw": 12.0},
					"trailingEps": {"raw": 11.5},
					"forwardEps": {"raw": 12.0},
					"bookValue": {"raw": 25.0}
				},
				"summaryDetail": {
					"marketCap": {"raw": 3000000000000},
					"trailingPE": {"raw": 32.0},
					"dividendYield": {"raw": 0.008},
					"beta": {"raw": 0.9},
					"fiftyTwoWeekHigh": {"raw": 450.0},
					"fiftyTwoWeekLow": {"raw": 350.0},
					"fiftyDayAverage": {"raw": 410.0},
					"twoHundredDayAverage": {"raw": 400.0}
				}
			}]
		}
	}`

	var sessionHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/fc"):
			http.SetCookie(w, &http.Cookie{Name: "A3", Value: "sess"})
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/getcrumb"):
			_, _ = w.Write([]byte("testcrumb"))
		case strings.Contains(r.URL.Path, "/quoteSummary/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(quoteBody))
		default:
			http.NotFound(w, r)
		}
		sessionHits++
	}))
	defer server.Close()

	sm := &YahooSessionManager{
		fcURL:    server.URL + "/fc",
		crumbURL: server.URL + "/getcrumb",
	}
	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/quoteSummary/") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
				return http.DefaultTransport.RoundTrip(req)
			}
			return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
		},
	}
	client := NewResilientHTTPClient(&http.Client{Transport: transport}, NewTokenBucket(10, 10), 0, time.Millisecond, time.Millisecond)
	dir := t.TempDir()
	reader := &YahooFinanceCSVReader{client: client, cacheDir: dir, sessionMgr: sm}

	got, err := reader.FetchFundamentals(context.Background(), "MSFT", time.Now())
	if err != nil {
		t.Fatalf("FetchFundamentals online: %v", err)
	}
	for _, want := range []string{
		"Microsoft Corporation",
		"Technology",
		"Market Cap: 3000000000000.00",
		"PE Ratio (TTM): 32.00",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fundamentals missing %q:\n%s", want, got)
		}
	}

	cacheFile := filepath.Join(dir, "MSFT-fundamentals.txt")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("expected fundamentals cache file: %v", err)
	}
}

func TestResilientClient503Retry(t *testing.T) {
	attempts := 0
	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts < 2 {
				resp := httptest.NewRecorder()
				resp.WriteHeader(http.StatusServiceUnavailable)
				return resp.Result(), nil
			}
			resp := httptest.NewRecorder()
			resp.WriteHeader(http.StatusOK)
			_, _ = resp.Write([]byte("ok"))
			return resp.Result(), nil
		},
	}
	client := NewResilientHTTPClient(&http.Client{Transport: transport}, NewTokenBucket(10, 10), 2, time.Millisecond, 5*time.Millisecond)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected retry success: %v", err)
	}
	_ = resp.Body.Close()
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestResilientClientContextCancel(t *testing.T) {
	// Zero-capacity bucket forces the rate-limit wait loop, which respects context cancellation.
	client := NewResilientHTTPClient(http.DefaultClient, NewTokenBucket(0, 0), 0, time.Millisecond, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled, got: %v", err)
	}
}
