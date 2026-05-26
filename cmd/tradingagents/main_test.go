package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockRoundTripper struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func captureOutput(f func()) (string, string) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()

	os.Stdout = wOut
	os.Stderr = wErr

	outChan := make(chan string)
	errChan := make(chan string)

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		outChan <- buf.String()
	}()

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rErr)
		errChan <- buf.String()
	}()

	f()

	_ = wOut.Close()
	_ = wErr.Close()

	return <-outChan, <-errChan
}

func TestMainHelp(t *testing.T) {
	// Reset flags to avoid contamination from test args
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "-help"}

	stdout, stderr := captureOutput(func() {
		code := run()
		if code != 0 {
			t.Errorf("expected exit code 0 for help, got %d", code)
		}
	})

	if !strings.Contains(stdout, "TradingAgents Go Orchestrator - Usage Guide") {
		t.Errorf("expected usage guide in output, got: %s", stdout)
	}
	_ = stderr
}

func TestMainInvalidFlag(t *testing.T) {
	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "-invalid-flag-name"}

	// Capture outputs and prevent panics from flag errors (flag.ContinueOnError allows this)
	stdout, stderr := captureOutput(func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("recovered panic: %v", r)
			}
		}()
		_ = run()
	})
	_ = stdout
	_ = stderr
}

func TestMainWorkflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tradingagents-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	cacheDir := filepath.Join(tempDir, "cache")
	resultsDir := filepath.Join(tempDir, "results")
	localReportsDir := filepath.Join(tempDir, "reports")
	dbPath := filepath.Join(tempDir, "test-checkpoints.db")
	memoryPath := filepath.Join(tempDir, "test-memory.md")

	_ = os.MkdirAll(cacheDir, 0750)
	_ = os.MkdirAll(resultsDir, 0750)
	_ = os.MkdirAll(localReportsDir, 0750)

	// Pre-populate fundamental cache for AAPL to prevent outbound HTTP requests
	fundamentalsFile := filepath.Join(cacheDir, "AAPL-fundamentals.txt")
	fundamentalsContent := `# Company Fundamentals for AAPL
# Data retrieved on: 2026-05-25 12:00:00
Name: Apple Inc.
Sector: Technology
Industry: Consumer Electronics
Market Cap: 3000000.00
PE Ratio (TTM): 30.00
Forward PE: 28.00
PEG Ratio: 1.50
Price to Book: 40.00
EPS (TTM): 6.00
Forward EPS: 6.50
Dividend Yield: 0.005
Beta: 1.20
52 Week High: 198.00
52 Week Low: 165.00
50 Day Average: 180.00
200 Day Average: 175.00
Revenue (TTM): 380000.00
Gross Profit: 160000.00
EBITDA: 120000.00
Net Income: 95000.00
Profit Margin: 0.25
Operating Margin: 0.30
Return on Equity: 1.50
Return on Assets: 0.20
Debt to Equity: 1.40
Current Ratio: 1.20
Book Value: 4.50
Free Cash Flow: 100000.00
`
	if errWrite := os.WriteFile(fundamentalsFile, []byte(fundamentalsContent), 0600); errWrite != nil {
		t.Fatalf("failed to write fundamentals cache: %v", errWrite)
	}

	// Pre-populate candle cache (at least 210 candles so indicators compute)
	tradeDateStr := "2026-05-25"
	tradeDate, err := time.Parse("2006-01-02", tradeDateStr)
	if err != nil {
		t.Fatalf("invalid trade date: %v", err)
	}

	today := time.Now()
	start5Y := today.AddDate(-5, 0, 0)
	cacheFileName := fmt.Sprintf("AAPL-YFin-data-%s-%s.csv", start5Y.Format("2006-01-02"), today.Format("2006-01-02"))
	cacheFilePath := filepath.Join(cacheDir, cacheFileName)

	var csvContent strings.Builder
	csvContent.WriteString("Date,Open,High,Low,Close,Adj Close,Volume\n")
	for i := 250; i >= 0; i-- {
		d := tradeDate.AddDate(0, 0, -i)
		fmt.Fprintf(&csvContent, "%s,150.00,155.00,149.00,152.00,152.00,1000000.00\n", d.Format("2006-01-02"))
	}
	if errWrite := os.WriteFile(cacheFilePath, []byte(csvContent.String()), 0600); errWrite != nil {
		t.Fatalf("failed to write candle cache: %v", errWrite)
	}

	// Mock all HTTP calls (StockTwits, Reddit, Yahoo news)
	mockRT := &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			urlStr := req.URL.String()
			var body string
			switch {
			case strings.Contains(urlStr, "api.stocktwits.com"):
				body = `{
					"messages": [
						{
							"body": "Bullish AAPL!",
							"created_at": "2026-05-25T12:00:00Z",
							"user": {"username": "bull_trader"},
							"entities": {"sentiment": {"basic": "Bullish"}}
						}
					]
				}`
			case strings.Contains(urlStr, "reddit.com"):
				body = `{
					"data": {
						"children": [
							{
								"data": {
									"title": "AAPL analysis",
									"score": 100,
									"num_comments": 10,
									"created_utc": 1779537600,
									"selftext": "Long term hold"
								}
							}
						]
					}
				}`
			case strings.Contains(urlStr, "finance.yahoo.com") || strings.Contains(urlStr, "news"):
				body = `{
					"news": [
						{
							"uuid": "news-123",
							"title": "Apple releases new update",
							"publisher": "Yahoo News",
							"link": "http://example.com",
							"providerPublishTime": 1779537600,
							"summary": "Great update."
						}
					]
				}`
			default:
				body = "{}"
			}

			resp := httptest.NewRecorder()
			resp.Header().Set("Content-Type", "application/json")
			_, _ = resp.Write([]byte(body))
			return resp.Result(), nil
		},
	}

	oldTransport := http.DefaultTransport
	oldDefaultClientTransport := http.DefaultClient.Transport
	defer func() {
		http.DefaultTransport = oldTransport
		http.DefaultClient.Transport = oldDefaultClientTransport
	}()
	http.DefaultTransport = mockRT
	http.DefaultClient.Transport = mockRT

	// Set CLI Arguments
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"cmd",
		"-provider=mock",
		"-ticker=AAPL",
		"-trade-date=" + tradeDateStr,
		"-db-path=" + dbPath,
		"-results-dir=" + resultsDir,
		"-cache-dir=" + cacheDir,
		"-memory-path=" + memoryPath,
		"-enable-local-reports=true",
		"-local-reports-dir=" + localReportsDir,
		"-timeout=10",
	}

	// Capture output and run workflow
	stdout, stderr := captureOutput(func() {
		code := run()
		if code != 0 {
			t.Errorf("expected workflow run to succeed with 0, got %d", code)
		}
	})

	if !strings.Contains(strings.ToLower(stdout), "workflow complete for aapl") {
		t.Errorf("expected workflow completion summary in stdout, got: %s", stdout)
	}

	_ = stderr

	// Verify files were generated correctly
	if _, errStatMemory := os.Stat(memoryPath); os.IsNotExist(errStatMemory) {
		t.Errorf("expected memory log to be written to %s", memoryPath)
	}

	if _, errStatDB := os.Stat(dbPath); errStatDB != nil {
		t.Errorf("expected db file to exist, but got: %v", errStatDB)
	}

	reports, errReadDir := os.ReadDir(localReportsDir)
	if errReadDir != nil || len(reports) == 0 {
		t.Errorf("expected reports to be generated in %s, got err: %v, count: %d", localReportsDir, errReadDir, len(reports))
	}
}
