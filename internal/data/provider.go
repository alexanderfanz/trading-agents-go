package data

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"tradingagents/internal/config"
)

type DataProvider struct {
	cfg      *config.Config
	yClient  *YFinanceClient
	sClient  *SocialClient
	cacheDir string
}

func NewDataProvider(cfg *config.Config) *DataProvider {
	return &DataProvider{
		cfg:      cfg,
		yClient:  NewYFinanceClient(),
		sClient:  NewSocialClient(),
		cacheDir: cfg.DataCacheDir,
	}
}

// GetStockData downloads historical daily OHLCV prices from yfinance.
func (p *DataProvider) GetStockData(ctx context.Context, symbol string, startDate, endDate string) ([]OHLCV, error) {
	// Try loading from cache first
	cacheKey := fmt.Sprintf("%s-ohlcv-%s-%s.json", strings.ToLower(symbol), startDate, endDate)
	cacheFile := filepath.Join(p.cacheDir, cacheKey)

	if err := os.MkdirAll(p.cacheDir, 0755); err == nil {
		if data, err := os.ReadFile(cacheFile); err == nil {
			var cached []OHLCV
			if err := json.Unmarshal(data, &cached); err == nil {
				return cached, nil
			}
		}
	}

	// Fetch fresh from Yahoo Finance
	data, err := p.yClient.FetchOHLCV(ctx, symbol, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Save to cache
	if err := os.MkdirAll(p.cacheDir, 0755); err == nil {
		if bytes, err := json.Marshal(data); err == nil {
			_ = os.WriteFile(cacheFile, bytes, 0644)
		}
	}

	return data, nil
}

var IndicatorDescriptions = map[string]string{
	"close_50_sma":  "50 SMA: A medium-term trend indicator. Usage: Identify trend direction and serve as dynamic support/resistance. Tips: It lags price; combine with faster indicators for timely signals.",
	"close_200_sma": "200 SMA: A long-term trend benchmark. Usage: Confirm overall market trend and identify golden/death cross setups. Tips: It reacts slowly; best for strategic trend confirmation rather than frequent trading entries.",
	"close_10_ema":  "10 EMA: A responsive short-term average. Usage: Capture quick shifts in momentum and potential entry points. Tips: Prone to noise in choppy markets; use alongside longer averages for filtering false signals.",
	"macd":          "MACD: Computes momentum via differences of EMAs. Usage: Look for crossovers and divergence as signals of trend changes. Tips: Confirm with other indicators in low-volatility or sideways markets.",
	"macds":         "MACD Signal: An EMA smoothing of the MACD line. Usage: Use crossovers with the MACD line to trigger trades. Tips: Should be part of a broader strategy to avoid false positives.",
	"macdh":         "MACD Histogram: Shows the gap between the MACD line and its signal. Usage: Visualize momentum strength and spot divergence early. Tips: Can be volatile; complement with additional filters in fast-moving markets.",
	"rsi":           "RSI: Measures momentum to flag overbought/oversold conditions. Usage: Apply 70/30 thresholds and watch for divergence to signal reversals. Tips: In strong trends, RSI may remain extreme; always cross-check with trend analysis.",
	"boll":          "Bollinger Middle: A 20 SMA serving as the basis for Bollinger Bands. Usage: Acts as a dynamic benchmark for price movement. Tips: Combine with the upper and lower bands to effectively spot breakouts or reversals.",
	"boll_ub":       "Bollinger Upper Band: Typically 2 standard deviations above the middle line. Usage: Signals potential overbought conditions and breakout zones. Tips: Confirm signals with other tools; prices may ride the band in strong trends.",
	"boll_lb":       "Bollinger Lower Band: Typically 2 standard deviations below the middle line. Usage: Indicates potential oversold conditions. Tips: Use additional analysis to avoid false reversal signals.",
	"atr":           "ATR: Averages true range to measure volatility. Usage: Set stop-loss levels and adjust position sizes based on current market volatility. Tips: It's a reactive measure, so use it as part of a broader risk management strategy.",
	"vwma":          "VWMA: A moving average weighted by volume. Usage: Confirm trends by integrating price action with volume data. Tips: Watch for skewed results from volume spikes; use in combination with other volume analyses.",
	"mfi":           "MFI: The Money Flow Index is a momentum indicator that uses both price and volume to measure buying and selling pressure. Usage: Identify overbought (>80) or oversold (<20) conditions and confirm the strength of trends or reversals. Tips: Use alongside RSI or MACD to confirm signals; divergence between price and MFI can indicate potential reversals.",
}

// GetIndicators retrieves technical analysis indicators formatted as a chronological list.
func (p *DataProvider) GetIndicators(ctx context.Context, symbol string, indicator string, currDate string, lookBackDays int) (string, error) {
	desc, exists := IndicatorDescriptions[indicator]
	if !exists {
		return "", fmt.Errorf("indicator %s is not supported", indicator)
	}

	currT, err := time.Parse("2006-01-02", currDate)
	if err != nil {
		return "", fmt.Errorf("invalid current date format: %w", err)
	}

	// Fetch 5 years of historical data up to currDate to ensure averages have sufficient warm-up period
	startT := currT.AddDate(-5, 0, 0)
	startDate := startT.Format("2006-01-02")

	data, err := p.GetStockData(ctx, symbol, startDate, currDate)
	if err != nil {
		return "", fmt.Errorf("failed to fetch historical data for indicator %s: %w", indicator, err)
	}

	var computedValues []float64

	switch indicator {
	case "close_50_sma":
		computedValues = CalculateSMA(data, 50)
	case "close_200_sma":
		computedValues = CalculateSMA(data, 200)
	case "close_10_ema":
		computedValues = CalculateEMA(data, 10)
	case "macd":
		computedValues = CalculateMACD(data, 12, 26, 9).MACD
	case "macds":
		computedValues = CalculateMACD(data, 12, 26, 9).Signal
	case "macdh":
		computedValues = CalculateMACD(data, 12, 26, 9).Histogram
	case "rsi":
		computedValues = CalculateRSI(data, 14)
	case "boll":
		computedValues = CalculateBollingerBands(data, 20, 2).Middle
	case "boll_ub":
		computedValues = CalculateBollingerBands(data, 20, 2).Upper
	case "boll_lb":
		computedValues = CalculateBollingerBands(data, 20, 2).Lower
	case "atr":
		computedValues = CalculateATR(data, 14)
	case "vwma":
		computedValues = CalculateVWMA(data, 20)
	case "mfi":
		computedValues = CalculateMFI(data, 14)
	}

	// Map computed values by date string
	valMap := make(map[string]string)
	for i, ohlcv := range data {
		if i < len(computedValues) {
			valMap[ohlcv.Date] = FormatIndicatorValue(computedValues[i])
		}
	}

	// Loop back day-by-day to extract the requested time window
	var lines []string
	before := currT.AddDate(0, 0, -lookBackDays)

	current := currT
	for !current.Before(before) {
		dateStr := current.Format("2006-01-02")
		valStr := "N/A: Not a trading day (weekend or holiday)"
		if cachedVal, ok := valMap[dateStr]; ok {
			valStr = cachedVal
		}

		lines = append(lines, fmt.Sprintf("%s: %s", dateStr, valStr))
		current = current.AddDate(0, 0, -1)
	}

	resultStr := fmt.Sprintf("## %s values from %s to %s:\n\n%s\n\n%s",
		indicator, before.Format("2006-01-02"), currDate, strings.Join(lines, "\n"), desc)

	return resultStr, nil
}

// GetFundamentals fetches fundamental data for the company.
func (p *DataProvider) GetFundamentals(ctx context.Context, symbol string) (string, error) {
	return p.yClient.FetchFundamentals(ctx, symbol)
}

// GetBalanceSheet fetches the balance sheet statement.
func (p *DataProvider) GetBalanceSheet(ctx context.Context, symbol string, freq string) (string, error) {
	return p.yClient.FetchFinancialStatements(ctx, symbol, "balance_sheet", freq)
}

// GetCashFlow fetches the cash flow statement.
func (p *DataProvider) GetCashFlow(ctx context.Context, symbol string, freq string) (string, error) {
	return p.yClient.FetchFinancialStatements(ctx, symbol, "cash_flow", freq)
}

// GetIncomeStatement fetches the income statement.
func (p *DataProvider) GetIncomeStatement(ctx context.Context, symbol string, freq string) (string, error) {
	return p.yClient.FetchFinancialStatements(ctx, symbol, "income_statement", freq)
}

// GetNews fetches ticker-specific news.
func (p *DataProvider) GetNews(ctx context.Context, symbol string, startDate, endDate string, limit int) (string, error) {
	return p.yClient.FetchNews(ctx, symbol, startDate, endDate, limit)
}

// GetGlobalNews fetches global macro news articles.
func (p *DataProvider) GetGlobalNews(ctx context.Context, queries []string, currDate string, lookBackDays int, limit int) (string, error) {
	if len(queries) == 0 {
		queries = p.cfg.GlobalNewsQueries
	}
	return p.yClient.FetchGlobalNews(ctx, queries, currDate, lookBackDays, limit)
}

// GetInsiderTransactions fetches insider transactions list.
func (p *DataProvider) GetInsiderTransactions(ctx context.Context, symbol string) (string, error) {
	return p.yClient.FetchInsiderTransactions(ctx, symbol)
}

// GetRedditPosts gets social posts mentioning ticker.
func (p *DataProvider) GetRedditPosts(ctx context.Context, ticker string, subreddits []string, limitPerSub int) (string, error) {
	return p.sClient.FetchRedditPosts(ctx, ticker, subreddits, limitPerSub)
}

// GetStockTwitsMessages gets message stream for ticker symbol.
func (p *DataProvider) GetStockTwitsMessages(ctx context.Context, ticker string, limit int) (string, error) {
	return p.sClient.FetchStockTwitsMessages(ctx, ticker, limit)
}

// FormatIndicatorValue converts float values to strings with 2 decimal precision, handling NaN.
func FormatIndicatorValue(val float64) string {
	if math.IsNaN(val) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", val)
}
