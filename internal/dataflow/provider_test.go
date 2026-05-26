package dataflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testResilientClient(transport http.RoundTripper) *ResilientHTTPClient {
	httpClient := &http.Client{Transport: transport}
	limiter := NewTokenBucket(10, 10)
	return NewResilientHTTPClient(httpClient, limiter, 2, 1*time.Millisecond, 5*time.Millisecond)
}

func ohlcvCacheFile(cacheDir, ticker string) string {
	today := time.Now()
	start5Y := today.AddDate(-5, 0, 0)
	return filepath.Join(cacheDir, fmt.Sprintf("%s-YFin-data-%s-%s.csv",
		ticker, start5Y.Format("2006-01-02"), today.Format("2006-01-02")))
}

func loadTestFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
}

func newMockSessionManager(t *testing.T) (*YahooSessionManager, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fc":
			http.SetCookie(w, &http.Cookie{
				Name:  mockCookieName,
				Value: mockCookieValue,
			})
			w.WriteHeader(http.StatusNotFound)
		case "/getcrumb":
			cookie, err := r.Cookie(mockCookieName)
			if err != nil || cookie.Value != mockCookieValue {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockCrumbValue))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	sm := &YahooSessionManager{
		fcURL:    server.URL + "/fc",
		crumbURL: server.URL + "/getcrumb",
	}
	return sm, server
}

func assertNoLookAhead(t *testing.T, candles []Candle, tradeDate time.Time) {
	t.Helper()
	for _, c := range candles {
		if c.Time.After(tradeDate) {
			t.Errorf("look-ahead candle %s is after tradeDate %s", c.Time.Format("2006-01-02"), tradeDate.Format("2006-01-02"))
		}
	}
}

func seedOHLCVCache(t *testing.T, cacheDir, ticker string, rows ...string) string {
	t.Helper()
	cacheFile := ohlcvCacheFile(cacheDir, ticker)
	var b strings.Builder
	b.WriteString("Date,Open,High,Low,Close,Adj Close,Volume\n")
	for _, row := range rows {
		b.WriteString(row)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(cacheFile, []byte(b.String()), 0600); err != nil {
		t.Fatalf("failed to seed cache: %v", err)
	}
	return cacheFile
}

func TestFetchOHLCV_CacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	seedOHLCVCache(t, cacheDir, "AAPL",
		"2024-05-18,100.00,105.00,99.00,104.00,104.00,10000",
		"2024-05-19,100.00,105.00,99.00,104.00,104.00,10000",
		"2024-05-20,100.00,105.00,99.00,104.00,104.00,10000",
		"2024-05-21,100.00,105.00,99.00,104.00,104.00,10000",
	)

	var httpHits int32
	client := testResilientClient(&mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&httpHits, 1)
			return nil, fmt.Errorf("unexpected HTTP request: %s", req.URL)
		},
	})
	reader := NewYahooFinanceCSVReader(client, cacheDir)

	candles, err := reader.FetchOHLCV(context.Background(), "AAPL", time.Time{}, time.Time{}, tradeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&httpHits) != 0 {
		t.Fatalf("expected no HTTP requests on cache hit, got %d", httpHits)
	}
	if len(candles) != 3 {
		t.Fatalf("expected 3 candles (look-ahead filtered), got %d", len(candles))
	}
	assertNoLookAhead(t, candles, tradeDate)
	if candles[len(candles)-1].Time.Format("2006-01-02") != "2024-05-20" {
		t.Errorf("expected last candle on tradeDate, got %s", candles[len(candles)-1].Time.Format("2006-01-02"))
	}
}

func TestFetchOHLCV_OnlineChartMock(t *testing.T) {
	cacheDir := t.TempDir()
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	chartJSON := loadTestFixture(t, "chart_aapl.json")

	var chartHits int32
	client := testResilientClient(&mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/v8/finance/chart/") {
				atomic.AddInt32(&chartHits, 1)
				rec := httptest.NewRecorder()
				rec.Header().Set("Content-Type", "application/json")
				_, _ = rec.Write(chartJSON)
				return rec.Result(), nil
			}
			return nil, fmt.Errorf("unexpected request: %s", req.URL)
		},
	})
	reader := NewYahooFinanceCSVReader(client, cacheDir)

	candles, err := reader.FetchOHLCV(context.Background(), "AAPL", time.Time{}, time.Time{}, tradeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&chartHits) != 1 {
		t.Fatalf("expected exactly 1 chart request, got %d", chartHits)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles after look-ahead filter, got %d", len(candles))
	}
	assertNoLookAhead(t, candles, tradeDate)

	cacheFile := ohlcvCacheFile(cacheDir, "AAPL")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("expected CSV cache file written: %v", err)
	}
	cacheData, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("failed to read cache: %v", err)
	}
	if !strings.Contains(string(cacheData), "2024-05-21") {
		t.Error("expected cache CSV to include future date row")
	}
	if strings.Count(string(cacheData), "2024-05-21") != 1 {
		t.Errorf("expected one 2024-05-21 row in cache, got data:\n%s", cacheData)
	}
}

func TestFetchOHLCV_OnlineChartError(t *testing.T) {
	cacheDir := t.TempDir()
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)

	client := testResilientClient(&mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusInternalServerError)
			return rec.Result(), nil
		},
	})
	reader := NewYahooFinanceCSVReader(client, cacheDir)

	_, err := reader.FetchOHLCV(context.Background(), "AAPL", time.Time{}, time.Time{}, tradeDate)
	if err == nil {
		t.Fatal("expected error when chart API returns 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected status 500 in error, got: %v", err)
	}
}

func TestFetchAndStreamOHLCV_LocalFile(t *testing.T) {
	cacheDir := t.TempDir()
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	csvPath := filepath.Join(cacheDir, "stream.csv")
	csvContent := "Date,Open,High,Low,Close,Adj Close,Volume\n" +
		"2024-05-19,100.00,105.00,99.00,104.00,104.00,10000\n" +
		"2024-05-20,101.00,106.00,100.00,105.00,105.00,11000\r\n" +
		"\n" +
		"2024-05-21,102.00,107.00,101.00,106.00,106.00,12000\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0600); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	reader := NewYahooFinanceCSVReader(nil, cacheDir)
	candles, err := reader.FetchAndStreamOHLCV(context.Background(), csvPath, tradeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles (CRLF row + empty line skipped, future excluded), got %d", len(candles))
	}
	assertNoLookAhead(t, candles, tradeDate)
	if candles[1].Close != 105.0 {
		t.Errorf("expected second candle close 105.0, got %f", candles[1].Close)
	}
}

func TestFetchAndStreamOHLCV_LocalFileError(t *testing.T) {
	reader := NewYahooFinanceCSVReader(nil, t.TempDir())
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)

	_, err := reader.FetchAndStreamOHLCV(context.Background(), filepath.Join(t.TempDir(), "missing.csv"), tradeDate)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFetchAndStreamOHLCV_HTTP(t *testing.T) {
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	csvBody := "Date,Open,High,Low,Close,Adj Close,Volume\n" +
		"2024-05-19,100.00,105.00,99.00,104.00,104.00,10000\n" +
		"2024-05-21,102.00,107.00,101.00,106.00,106.00,12000\n"

	sm, _ := newMockSessionManager(t)
	csvServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("crumb") != mockCrumbValue {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(csvBody))
	}))
	t.Cleanup(csvServer.Close)

	client := testResilientClient(http.DefaultTransport)
	reader := &YahooFinanceCSVReader{
		client:     client,
		cacheDir:   t.TempDir(),
		sessionMgr: sm,
	}

	candles, err := reader.FetchAndStreamOHLCV(context.Background(), csvServer.URL, tradeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle from HTTP stream, got %d", len(candles))
	}
	assertNoLookAhead(t, candles, tradeDate)
}

func TestFetchAndStreamOHLCV_HTTPUnauthorized(t *testing.T) {
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	sm, sessionServer := newMockSessionManager(t)

	var quoteHits int32
	csvServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&quoteHits, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(csvServer.Close)

	client := testResilientClient(http.DefaultTransport)
	reader := &YahooFinanceCSVReader{
		client:     client,
		cacheDir:   t.TempDir(),
		sessionMgr: sm,
	}

	_, err := reader.FetchAndStreamOHLCV(context.Background(), csvServer.URL, tradeDate)
	if err == nil {
		t.Fatal("expected error on 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}

	// Session should be invalidated; next credential fetch hits session server again.
	_, _, err = sm.GetCredentials(context.Background())
	if err != nil {
		t.Fatalf("expected session refresh after invalidation: %v", err)
	}
	_ = sessionServer
}

func TestFetchFundamentals_CacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	cached := "# Company Fundamentals for AAPL\nName: Apple Inc.\nSector: Technology\n"
	cacheFile := filepath.Join(cacheDir, "AAPL-fundamentals.txt")
	if err := os.WriteFile(cacheFile, []byte(cached), 0600); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	var httpHits int32
	client := testResilientClient(&mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&httpHits, 1)
			return nil, fmt.Errorf("unexpected HTTP request")
		},
	})
	reader := NewYahooFinanceCSVReader(client, cacheDir)
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)

	out, err := reader.FetchFundamentals(context.Background(), "AAPL", tradeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != cached {
		t.Errorf("expected cached fundamentals, got: %s", out)
	}
	if atomic.LoadInt32(&httpHits) != 0 {
		t.Fatalf("expected no HTTP on cache hit, got %d hits", httpHits)
	}
}

func TestFetchFundamentals_OnlineQuoteSummary(t *testing.T) {
	cacheDir := t.TempDir()
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	quoteJSON := loadTestFixture(t, "quote_summary_aapl.json")

	sm, _ := newMockSessionManager(t)
	var quoteHits int32
	client := testResilientClient(&mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/v10/finance/quoteSummary/") {
				atomic.AddInt32(&quoteHits, 1)
				if req.URL.Query().Get("crumb") != mockCrumbValue {
					rec := httptest.NewRecorder()
					rec.WriteHeader(http.StatusUnauthorized)
					return rec.Result(), nil
				}
				rec := httptest.NewRecorder()
				rec.Header().Set("Content-Type", "application/json")
				_, _ = rec.Write(quoteJSON)
				return rec.Result(), nil
			}
			return nil, fmt.Errorf("unexpected request: %s", req.URL)
		},
	})
	reader := &YahooFinanceCSVReader{
		client:     client,
		cacheDir:   cacheDir,
		sessionMgr: sm,
	}

	out, err := reader.FetchFundamentals(context.Background(), "AAPL", tradeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&quoteHits) != 1 {
		t.Fatalf("expected 1 quoteSummary request, got %d", quoteHits)
	}
	if !strings.Contains(out, "Apple Inc.") {
		t.Errorf("expected company name in output: %s", out)
	}
	if !strings.Contains(out, "Sector: Technology") {
		t.Errorf("expected sector in output: %s", out)
	}

	cacheFile := filepath.Join(cacheDir, "AAPL-fundamentals.txt")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("expected fundamentals cache written: %v", err)
	}
}

func TestFetchFundamentals_OnlineUnauthorizedRetry(t *testing.T) {
	cacheDir := t.TempDir()
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	quoteJSON := loadTestFixture(t, "quote_summary_aapl.json")

	sm, _ := newMockSessionManager(t)
	attempts := 0
	client := testResilientClient(&mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/v10/finance/quoteSummary/") {
				return nil, fmt.Errorf("unexpected request: %s", req.URL)
			}
			attempts++
			rec := httptest.NewRecorder()
			if attempts == 1 {
				rec.WriteHeader(http.StatusUnauthorized)
				return rec.Result(), nil
			}
			rec.Header().Set("Content-Type", "application/json")
			_, _ = rec.Write(quoteJSON)
			return rec.Result(), nil
		},
	})
	reader := &YahooFinanceCSVReader{
		client:     client,
		cacheDir:   cacheDir,
		sessionMgr: sm,
	}

	_, err := reader.FetchFundamentals(context.Background(), "AAPL", tradeDate)
	if err == nil {
		t.Fatal("expected error on first 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}

	// After invalidation, a second fetch should succeed with refreshed credentials.
	out, err := reader.FetchFundamentals(context.Background(), "AAPL", tradeDate)
	if err != nil {
		t.Fatalf("expected success after session refresh: %v", err)
	}
	if !strings.Contains(out, "Apple Inc.") {
		t.Errorf("expected fundamentals after retry, got: %s", out)
	}
	if attempts < 2 {
		t.Errorf("expected at least 2 quoteSummary attempts, got %d", attempts)
	}
}

func TestFetchFundamentals_OnlineError(t *testing.T) {
	cacheDir := t.TempDir()
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)

	sm, _ := newMockSessionManager(t)
	client := testResilientClient(&mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.Header().Set("Content-Type", "application/json")
			_, _ = rec.Write([]byte(`{"quoteSummary":{"result":[]}}`))
			return rec.Result(), nil
		},
	})
	reader := &YahooFinanceCSVReader{
		client:     client,
		cacheDir:   cacheDir,
		sessionMgr: sm,
	}

	_, err := reader.FetchFundamentals(context.Background(), "AAPL", tradeDate)
	if err == nil {
		t.Fatal("expected error for empty quoteSummary result")
	}
	if !strings.Contains(err.Error(), "no fundamentals data") {
		t.Errorf("expected no data error, got: %v", err)
	}
}

func TestParseCSVRowFastEdgeCases(t *testing.T) {
	tradeDate := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)

	t.Run("invalid date length", func(t *testing.T) {
		_, valid, err := parseCSVRowFast([]byte("2024-5-20,100,105,99,104,104,10000"), tradeDate)
		if err == nil {
			t.Fatal("expected invalid date error")
		}
		if valid {
			t.Error("expected valid=false for invalid date")
		}
	})

	t.Run("bad float", func(t *testing.T) {
		_, valid, err := parseCSVRowFast([]byte("2024-05-20,not-a-float,105,99,104,104,10000"), tradeDate)
		if err == nil {
			t.Fatal("expected float parse error")
		}
		if valid {
			t.Error("expected valid=false for bad float")
		}
	})

	t.Run("short row", func(t *testing.T) {
		_, valid, err := parseCSVRowFast([]byte("2024-05-20,100,105"), tradeDate)
		if err == nil {
			t.Fatal("expected incomplete columns error")
		}
		if valid {
			t.Error("expected valid=false for short row")
		}
	})

	t.Run("CRLF trailing carriage return", func(t *testing.T) {
		row := []byte("2024-05-20,100.00,105.00,99.00,104.00,104.00,10000\r")
		_, valid, err := parseCSVRowFast(row, tradeDate)
		if err == nil {
			t.Fatal("expected volume parse error when CR is not stripped")
		}
		if valid {
			t.Error("expected valid=false when CR remains in row")
		}
		if !strings.Contains(err.Error(), "volume parse error") {
			t.Errorf("expected volume parse error, got: %v", err)
		}
	})

	t.Run("empty line", func(t *testing.T) {
		_, valid, err := parseCSVRowFast([]byte(""), tradeDate)
		if err == nil {
			t.Fatal("expected error for empty line")
		}
		if valid {
			t.Error("expected valid=false for empty line")
		}
	})
}
