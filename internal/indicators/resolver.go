package indicators

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"trading-agents-go/internal/dataflow"
)

// IndicatorCache is a thread-safe caching/memoization layer for calculated metrics.
type IndicatorCache struct {
	mu    sync.RWMutex
	store map[string]float64
}

// NewIndicatorCache instantiates a thread-safe cache.
func NewIndicatorCache() *IndicatorCache {
	return &IndicatorCache{
		store: make(map[string]float64),
	}
}

// Get retrieves a cached indicator value if present.
func (c *IndicatorCache) Get(ticker, indicator string, date time.Time) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", ticker, indicator, date.Format("2006-01-02"))
	val, ok := c.store[key]
	return val, ok
}

// Put saves an indicator value to the cache.
func (c *IndicatorCache) Put(ticker, indicator string, date time.Time, val float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", ticker, indicator, date.Format("2006-01-02"))
	c.store[key] = val
}

// DynamicIndicatorResolver translates string indicator tool arguments into direct execution paths.
type DynamicIndicatorResolver struct {
	cache *IndicatorCache
}

// NewDynamicIndicatorResolver instantiates a resolver.
func NewDynamicIndicatorResolver(cache *IndicatorCache) *DynamicIndicatorResolver {
	return &DynamicIndicatorResolver{cache: cache}
}

// Resolve matches and parses requested indicator string (e.g. "close_50_sma") at runtime without reflection.
func (r *DynamicIndicatorResolver) Resolve(ctx context.Context, candles []dataflow.Candle, ticker, indicatorStr string, tradeDate time.Time) (float64, error) {
	if val, ok := r.cache.Get(ticker, indicatorStr, tradeDate); ok {
		return val, nil
	}

	var result float64
	var err error

	switch indicatorStr {
	case "macd", "macds", "macdh":
		n := len(candles)
		if n < 26 {
			return 0, fmt.Errorf("insufficient data points for MACD (requires at least 26, got %d)", n)
		}
		prices := make([]float64, n)
		for i, c := range candles {
			prices[i] = c.Close
		}
		fastBuf := make([]float64, n)
		slowBuf := make([]float64, n)
		macdOut := make([]float64, n)
		sigOut := make([]float64, n)
		histOut := make([]float64, n)
		if err = MACD(prices, 12, 26, 9, fastBuf, slowBuf, macdOut, sigOut, histOut); err != nil {
			return 0, err
		}
		switch indicatorStr {
		case "macd":
			result = macdOut[n-1]
		case "macds":
			result = sigOut[n-1]
		case "macdh":
			result = histOut[n-1]
		}

	case "boll", "boll_ub", "boll_lb":
		n := len(candles)
		if n < 20 {
			return 0, fmt.Errorf("insufficient data points for Bollinger Bands (requires at least 20, got %d)", n)
		}
		prices := make([]float64, n)
		for i, c := range candles {
			prices[i] = c.Close
		}
		midBand := make([]float64, n)
		upperBand := make([]float64, n)
		lowerBand := make([]float64, n)
		if err = BollingerBands(prices, 20, 2.0, midBand, upperBand, lowerBand); err != nil {
			return 0, err
		}
		switch indicatorStr {
		case "boll":
			result = midBand[n-1]
		case "boll_ub":
			result = upperBand[n-1]
		case "boll_lb":
			result = lowerBand[n-1]
		}

	case "rsi":
		n := len(candles)
		if n < 15 {
			return 0, fmt.Errorf("insufficient data points for RSI (requires at least 15, got %d)", n)
		}
		prices := make([]float64, n)
		for i, c := range candles {
			prices[i] = c.Close
		}
		out := make([]float64, n)
		if err = RSI(prices, 14, out); err != nil {
			return 0, err
		}
		result = out[n-1]

	case "atr":
		n := len(candles)
		if n < 15 {
			return 0, fmt.Errorf("insufficient data points for ATR (requires at least 15, got %d)", n)
		}
		highs := make([]float64, n)
		lows := make([]float64, n)
		closes := make([]float64, n)
		for i, c := range candles {
			highs[i] = c.High
			lows[i] = c.Low
			closes[i] = c.Close
		}
		trBuf := make([]float64, n)
		out := make([]float64, n)
		if err = ATR(highs, lows, closes, 14, trBuf, out); err != nil {
			return 0, err
		}
		result = out[n-1]

	case "mfi":
		n := len(candles)
		if n < 15 {
			return 0, fmt.Errorf("insufficient data points for MFI (requires at least 15, got %d)", n)
		}
		highs := make([]float64, n)
		lows := make([]float64, n)
		closes := make([]float64, n)
		volumes := make([]float64, n)
		for i, c := range candles {
			highs[i] = c.High
			lows[i] = c.Low
			closes[i] = c.Close
			volumes[i] = c.Volume
		}
		pmfBuf := make([]float64, n)
		nmfBuf := make([]float64, n)
		out := make([]float64, n)
		if err = MFI(highs, lows, closes, volumes, 14, pmfBuf, nmfBuf, out); err != nil {
			return 0, err
		}
		result = out[n-1]

	default:
		parts := strings.Split(indicatorStr, "_")
		if len(parts) < 3 {
			return 0, fmt.Errorf("unsupported indicator name/format: %s", indicatorStr)
		}

		targetMetric := parts[0]
		periodStr := parts[1]
		indType := parts[2]

		period, err := strconv.Atoi(periodStr)
		if err != nil {
			return 0, fmt.Errorf("invalid indicator period: %s", periodStr)
		}

		n := len(candles)
		if n < period {
			return 0, fmt.Errorf("insufficient data points for %s (requires at least %d, got %d)", indicatorStr, period, n)
		}

		prices := make([]float64, n)
		for i, c := range candles {
			switch targetMetric {
			case "close":
				prices[i] = c.Close
			case "open":
				prices[i] = c.Open
			case "high":
				prices[i] = c.High
			case "low":
				prices[i] = c.Low
			case "volume":
				prices[i] = c.Volume
			default:
				return 0, fmt.Errorf("unsupported price target: %s", targetMetric)
			}
		}

		out := make([]float64, n)
		switch indType {
		case "sma":
			err = SMA(prices, period, out)
		case "ema":
			err = EMA(prices, period, out)
		case "rsi":
			err = RSI(prices, period, out)
		default:
			return 0, fmt.Errorf("unsupported dynamic indicator type: %s", indType)
		}

		if err != nil {
			return 0, err
		}
		result = out[n-1]
	}

	r.cache.Put(ticker, indicatorStr, tradeDate, result)
	return result, nil
}
