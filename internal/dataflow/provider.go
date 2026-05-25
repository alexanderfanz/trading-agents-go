package dataflow

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Candle represents an individual OHLCV candlestick bar.
type Candle struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

// DataProvider represents the concurrency-safe contract for historical and fundamental data.
type DataProvider interface {
	// FetchOHLCV retrieves historical price bars for the given ticker,
	// filtering out any records with timestamps strictly greater than the tradeDate.
	FetchOHLCV(ctx context.Context, ticker string, start, end time.Time, tradeDate time.Time) ([]Candle, error)

	// FetchFundamentals retrieves structured company overview statements.
	FetchFundamentals(ctx context.Context, ticker string, tradeDate time.Time) (string, error)
}

// YahooFinanceCSVReader handles zero-allocation-on-hot-path CSV ingestion.
type YahooFinanceCSVReader struct {
	client     *ResilientHTTPClient
	cacheDir   string
	sessionMgr *YahooSessionManager
}

// NewYahooFinanceCSVReader instantiates a new reader.
func NewYahooFinanceCSVReader(client *ResilientHTTPClient, cacheDir string) *YahooFinanceCSVReader {
	return &YahooFinanceCSVReader{
		client:     client,
		cacheDir:   cacheDir,
		sessionMgr: NewYahooSessionManager(),
	}
}

// FetchOHLCV retrieves prices, utilizing caching and strict look-ahead filtering.
func (r *YahooFinanceCSVReader) FetchOHLCV(ctx context.Context, ticker string, start, end time.Time, tradeDate time.Time) ([]Candle, error) {
	safeTicker := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, ticker)

	// Build exact 5-year start/end stamps like Python's load_ohlcv cache
	today := time.Now()
	start5Y := today.AddDate(-5, 0, 0)
	startStr := start5Y.Format("2006-01-02")
	endStr := today.Format("2006-01-02")

	var dataFile string
	if r.cacheDir != "" {
		_ = os.MkdirAll(r.cacheDir, 0755)
		dataFile = filepath.Join(r.cacheDir, fmt.Sprintf("%s-YFin-data-%s-%s.csv", safeTicker, startStr, endStr))
	}

	// 1. Try reading from cache first
	if dataFile != "" {
		if _, err := os.Stat(dataFile); err == nil {
			return r.FetchAndStreamOHLCV(ctx, dataFile, tradeDate)
		}
	}

	// 2. Fetch online if not cached
	cookie, crumb, err := r.sessionMgr.GetCredentials(ctx)
	if err != nil {
		// Fallback: search for any matching cache file if we are offline or query fails
		if r.cacheDir != "" {
			matches, globErr := filepath.Glob(filepath.Join(r.cacheDir, fmt.Sprintf("%s-YFin-data-*.csv", safeTicker)))
			if globErr == nil && len(matches) > 0 {
				return r.FetchAndStreamOHLCV(ctx, matches[0], tradeDate)
			}
		}
		return nil, fmt.Errorf("failed to fetch yahoo credentials: %w", err)
	}

	startUnix := start5Y.Unix()
	endUnix := today.Unix()
	downloadURL := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/download/%s?period1=%d&period2=%d&interval=1d&events=history&includeAdjustedClose=true&crumb=%s", ticker, startUnix, endUnix, crumb)

	candles, err := r.fetchOnlineAndCache(ctx, downloadURL, cookie, dataFile, tradeDate)
	if err != nil {
		// Fallback: search for any matching cache file if we are offline or query fails
		if r.cacheDir != "" {
			matches, globErr := filepath.Glob(filepath.Join(r.cacheDir, fmt.Sprintf("%s-YFin-data-*.csv", safeTicker)))
			if globErr == nil && len(matches) > 0 {
				return r.FetchAndStreamOHLCV(ctx, matches[0], tradeDate)
			}
		}
		return nil, err
	}

	return candles, nil
}

func (r *YahooFinanceCSVReader) fetchOnlineAndCache(ctx context.Context, url string, cookie string, dataFile string, tradeDate time.Time) ([]Candle, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	var resp *http.Response
	if r.client != nil {
		resp, err = r.client.Do(req)
	} else {
		resp, err = http.DefaultClient.Do(req)
	}

	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusTooManyRequests {
		r.sessionMgr.Invalidate()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status %d", resp.StatusCode)
	}

	// Write response body to temp file, then rename to cache to ensure atomic write
	var tempFile *os.File
	if dataFile != "" {
		tempFile, err = os.CreateTemp(filepath.Dir(dataFile), "yfin-*.tmp")
	}

	// Read and parse
	candles := make([]Candle, 0, 512)
	reader := bufio.NewReaderSize(resp.Body, 64*1024)

	// Stream tokenizer processing
	header, err := reader.ReadSlice('\n')
	if err != nil {
		if tempFile != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempFile.Name())
		}
		return nil, fmt.Errorf("failed to parse CSV header: %w", err)
	}

	if tempFile != nil {
		_, _ = tempFile.Write(header)
	}

	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) == 0 {
					break
				}
			} else {
				if tempFile != nil {
					_ = tempFile.Close()
					_ = os.Remove(tempFile.Name())
				}
				return nil, fmt.Errorf("failed reading stream line: %w", err)
			}
		}

		if tempFile != nil {
			_, _ = tempFile.Write(line)
		}

		// Clean carriage returns
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) == 0 {
			continue
		}

		candle, valid, parseErr := parseCSVRowFast(line, tradeDate)
		if parseErr != nil {
			continue
		}
		if !valid {
			continue
		}

		candles = append(candles, candle)
	}

	if tempFile != nil {
		_ = tempFile.Close()
		_ = os.Rename(tempFile.Name(), dataFile)
	}

	return candles, nil
}

// FetchAndStreamOHLCV streams candles from a URL or local file path with zero-allocation-on-hot-path parsing.
func (r *YahooFinanceCSVReader) FetchAndStreamOHLCV(ctx context.Context, pathOrURL string, tradeDate time.Time) ([]Candle, error) {
	var input io.ReadCloser
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		cookie, crumb, err := r.sessionMgr.GetCredentials(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get yahoo credentials: %w", err)
		}

		if !strings.Contains(pathOrURL, "crumb=") {
			if strings.Contains(pathOrURL, "?") {
				pathOrURL = fmt.Sprintf("%s&crumb=%s", pathOrURL, crumb)
			} else {
				pathOrURL = fmt.Sprintf("%s?crumb=%s", pathOrURL, crumb)
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", pathOrURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}

		var resp *http.Response
		if r.client != nil {
			resp, err = r.client.Do(req)
		} else {
			resp, err = http.DefaultClient.Do(req)
		}
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusTooManyRequests {
			r.sessionMgr.Invalidate()
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("server responded with status %d", resp.StatusCode)
		}
		input = resp.Body
	} else {
		file, err := os.Open(pathOrURL)
		if err != nil {
			return nil, err
		}
		input = file
	}
	defer input.Close()

	candles := make([]Candle, 0, 512)
	reader := bufio.NewReaderSize(input, 64*1024)

	header, err := reader.ReadSlice('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV header: %w", err)
	}
	_ = header

	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) == 0 {
					break
				}
			} else {
				return nil, fmt.Errorf("failed reading stream line: %w", err)
			}
		}

		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) == 0 {
			continue
		}

		candle, valid, parseErr := parseCSVRowFast(line, tradeDate)
		if parseErr != nil {
			continue
		}
		if !valid {
			continue
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

// parseCSVRowFast parses fields without allocating strings or string slices on the heap.
func parseCSVRowFast(line []byte, tradeDate time.Time) (Candle, bool, error) {
	var candle Candle
	var colIdx int
	start := 0
	n := len(line)

	var dateVal time.Time
	var oVal, hVal, lVal, cVal, vVal float64

	for i := 0; i <= n; i++ {
		if i == n || line[i] == ',' {
			field := line[start:i]
			start = i + 1

			switch colIdx {
			case 0: // Date ("YYYY-MM-DD")
				if len(field) != 10 {
					return candle, false, fmt.Errorf("invalid date field length: %s", string(field))
				}
				d, err := time.Parse("2006-01-02", string(field))
				if err != nil {
					return candle, false, fmt.Errorf("failed to parse date: %w", err)
				}
				dateVal = d
				// CRITICAL LOOK-AHEAD BIAS FILTER
				if d.After(tradeDate) {
					return candle, false, nil // Discard future candle
				}
			case 1: // Open
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("open parse error: %w", err)
				}
				oVal = val
			case 2: // High
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("high parse error: %w", err)
				}
				hVal = val
			case 3: // Low
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("low parse error: %w", err)
				}
				lVal = val
			case 4: // Close
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("close parse error: %w", err)
				}
				cVal = val
			case 5: // Adj Close (Skipped)
			case 6: // Volume
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("volume parse error: %w", err)
				}
				vVal = val
			}
			colIdx++
		}
	}

	if colIdx < 7 {
		return candle, false, fmt.Errorf("incomplete columns: expected 7, parsed %d", colIdx)
	}

	candle = Candle{
		Time:   dateVal,
		Open:   oVal,
		High:   hVal,
		Low:    lVal,
		Close:  cVal,
		Volume: vVal,
	}

	return candle, true, nil
}

type rawValue struct {
	Raw *float64 `json:"raw"`
}

type quoteSummaryResponse struct {
	QuoteSummary struct {
		Result []struct {
			AssetProfile struct {
				Sector   string `json:"sector"`
				Industry string `json:"industry"`
			} `json:"assetProfile"`
			Price struct {
				LongName string `json:"longName"`
			} `json:"price"`
			FinancialData struct {
				TotalRevenue      rawValue `json:"totalRevenue"`
				GrossProfits      rawValue `json:"grossProfits"`
				Ebitda            rawValue `json:"ebitda"`
				NetIncomeToCommon rawValue `json:"netIncomeToCommon"`
				ProfitMargins     rawValue `json:"profitMargins"`
				OperatingMargins  rawValue `json:"operatingMargins"`
				ReturnOnEquity    rawValue `json:"returnOnEquity"`
				ReturnOnAssets    rawValue `json:"returnOnAssets"`
				DebtToEquity      rawValue `json:"debtToEquity"`
				CurrentRatio      rawValue `json:"currentRatio"`
				FreeCashflow      rawValue `json:"freeCashflow"`
			} `json:"financialData"`
			DefaultKeyStatistics struct {
				ForwardPE   rawValue `json:"forwardPE"`
				PegRatio    rawValue `json:"pegRatio"`
				PriceToBook rawValue `json:"priceToBook"`
				TrailingEps rawValue `json:"trailingEps"`
				ForwardEps  rawValue `json:"forwardEps"`
				BookValue   rawValue `json:"bookValue"`
			} `json:"defaultKeyStatistics"`
			SummaryDetail struct {
				MarketCap            rawValue `json:"marketCap"`
				TrailingPE           rawValue `json:"trailingPE"`
				DividendYield        rawValue `json:"dividendYield"`
				Beta                 rawValue `json:"beta"`
				FiftyTwoWeekHigh     rawValue `json:"fiftyTwoWeekHigh"`
				FiftyTwoWeekLow      rawValue `json:"fiftyTwoWeekLow"`
				FiftyDayAverage      rawValue `json:"fiftyDayAverage"`
				TwoHundredDayAverage rawValue `json:"twoHundredDayAverage"`
			} `json:"summaryDetail"`
		} `json:"result"`
	} `json:"quoteSummary"`
}

// FetchFundamentals retrieves structured company overview statements.
func (r *YahooFinanceCSVReader) FetchFundamentals(ctx context.Context, ticker string, tradeDate time.Time) (string, error) {
	safeTicker := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, ticker)

	// Check local fundamentals cache
	var cacheFile string
	if r.cacheDir != "" {
		cacheFile = filepath.Join(r.cacheDir, fmt.Sprintf("%s-fundamentals.txt", safeTicker))
		if data, err := os.ReadFile(cacheFile); err == nil {
			return string(data), nil
		}
	}

	cookie, crumb, err := r.sessionMgr.GetCredentials(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get yahoo credentials: %w", err)
	}

	url := fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=assetProfile,financialData,defaultKeyStatistics,summaryDetail,price&crumb=%s", ticker, crumb)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	var resp *http.Response
	if r.client != nil {
		resp, err = r.client.Do(req)
	} else {
		resp, err = http.DefaultClient.Do(req)
	}
	if err != nil {
		return "", fmt.Errorf("failed fetching fundamentals: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusTooManyRequests {
		r.sessionMgr.Invalidate()
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fundamentals server returned status %d", resp.StatusCode)
	}

	var qResp quoteSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qResp); err != nil {
		return "", fmt.Errorf("failed to decode fundamentals JSON: %w", err)
	}

	if len(qResp.QuoteSummary.Result) == 0 {
		return "", fmt.Errorf("no fundamentals data found for symbol '%s'", ticker)
	}

	res := qResp.QuoteSummary.Result[0]

	formatFloat := func(rv rawValue) string {
		if rv.Raw == nil {
			return ""
		}
		return fmt.Sprintf("%.2f", *rv.Raw)
	}

	fields := []struct {
		Label string
		Val   string
	}{
		{"Name", res.Price.LongName},
		{"Sector", res.AssetProfile.Sector},
		{"Industry", res.AssetProfile.Industry},
		{"Market Cap", formatFloat(res.SummaryDetail.MarketCap)},
		{"PE Ratio (TTM)", formatFloat(res.SummaryDetail.TrailingPE)},
		{"Forward PE", formatFloat(res.DefaultKeyStatistics.ForwardPE)},
		{"PEG Ratio", formatFloat(res.DefaultKeyStatistics.PegRatio)},
		{"Price to Book", formatFloat(res.DefaultKeyStatistics.PriceToBook)},
		{"EPS (TTM)", formatFloat(res.DefaultKeyStatistics.TrailingEps)},
		{"Forward EPS", formatFloat(res.DefaultKeyStatistics.ForwardEps)},
		{"Dividend Yield", formatFloat(res.SummaryDetail.DividendYield)},
		{"Beta", formatFloat(res.SummaryDetail.Beta)},
		{"52 Week High", formatFloat(res.SummaryDetail.FiftyTwoWeekHigh)},
		{"52 Week Low", formatFloat(res.SummaryDetail.FiftyTwoWeekLow)},
		{"50 Day Average", formatFloat(res.SummaryDetail.FiftyDayAverage)},
		{"200 Day Average", formatFloat(res.SummaryDetail.TwoHundredDayAverage)},
		{"Revenue (TTM)", formatFloat(res.FinancialData.TotalRevenue)},
		{"Gross Profit", formatFloat(res.FinancialData.GrossProfits)},
		{"EBITDA", formatFloat(res.FinancialData.Ebitda)},
		{"Net Income", formatFloat(res.FinancialData.NetIncomeToCommon)},
		{"Profit Margin", formatFloat(res.FinancialData.ProfitMargins)},
		{"Operating Margin", formatFloat(res.FinancialData.OperatingMargins)},
		{"Return on Equity", formatFloat(res.FinancialData.ReturnOnEquity)},
		{"Return on Assets", formatFloat(res.FinancialData.ReturnOnAssets)},
		{"Debt to Equity", formatFloat(res.FinancialData.DebtToEquity)},
		{"Current Ratio", formatFloat(res.FinancialData.CurrentRatio)},
		{"Book Value", formatFloat(res.DefaultKeyStatistics.BookValue)},
		{"Free Cash Flow", formatFloat(res.FinancialData.FreeCashflow)},
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("# Company Fundamentals for %s", strings.ToUpper(ticker)))
	lines = append(lines, fmt.Sprintf("# Data retrieved on: %s", time.Now().Format("2006-01-02 15:04:05")))
	lines = append(lines, "")

	for _, f := range fields {
		if f.Val != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", f.Label, f.Val))
		}
	}

	resultStr := strings.Join(lines, "\n")

	// Write cache
	if cacheFile != "" {
		_ = os.WriteFile(cacheFile, []byte(resultStr), 0644)
	}

	return resultStr, nil
}
