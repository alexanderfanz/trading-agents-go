package indicators

import (
	"context"
	"math"
	"testing"
	"time"

	"trading-agents-go/internal/dataflow"
)

func TestSMA(t *testing.T) {
	prices := []float64{10.0, 11.0, 12.0, 13.0, 14.0}
	out := make([]float64, len(prices))

	// Normal case
	err := SMA(prices, 3, out)
	if err != nil {
		t.Fatalf("SMA failed: %v", err)
	}

	expected := []float64{0.0, 0.0, 11.0, 12.0, 13.0}
	for i := 2; i < len(prices); i++ {
		if math.Abs(out[i]-expected[i]) > 1e-9 {
			t.Errorf("Index %d: expected %f, got %f", i, expected[i], out[i])
		}
	}

	// Insufficient data
	if err := SMA(prices, 10, out); err == nil {
		t.Error("expected error for SMA with period > prices len")
	}

	// Output capacity too small
	smallOut := make([]float64, 2)
	if err := SMA(prices, 3, smallOut); err == nil {
		t.Error("expected error for SMA with small output slice")
	}
}

func TestEMA(t *testing.T) {
	prices := []float64{10.0, 11.0, 12.0, 13.0, 14.0}
	out := make([]float64, len(prices))

	// Normal case
	err := EMA(prices, 3, out)
	if err != nil {
		t.Fatalf("EMA failed: %v", err)
	}

	expected := []float64{0.0, 0.0, 11.0, 12.0, 13.0}
	for i := 2; i < len(prices); i++ {
		if math.Abs(out[i]-expected[i]) > 1e-9 {
			t.Errorf("Index %d: expected %f, got %f", i, expected[i], out[i])
		}
	}

	// Insufficient data
	if err := EMA(prices, 10, out); err == nil {
		t.Error("expected error for EMA with period > prices len")
	}

	// Output capacity too small
	smallOut := make([]float64, 2)
	if err := EMA(prices, 3, smallOut); err == nil {
		t.Error("expected error for EMA with small output slice")
	}
}

func TestRSI(t *testing.T) {
	prices := []float64{10, 12, 15, 14, 13, 16, 18, 20, 19, 21, 22, 25, 24, 23, 26, 28}
	out := make([]float64, len(prices))

	// Normal case
	err := RSI(prices, 14, out)
	if err != nil {
		t.Fatalf("RSI failed: %v", err)
	}

	// Verify that the computed RSI values are bounded between 0 and 100
	for i := 14; i < len(prices); i++ {
		if out[i] < 0 || out[i] > 100 {
			t.Errorf("Index %d: RSI %f out of bounds [0, 100]", i, out[i])
		}
	}

	// Flat prices result in an RSI of 100 in this implementation because avgLoss is 0.
	flat := []float64{10, 10, 10, 10, 10}
	flatOut := make([]float64, len(flat))
	err = RSI(flat, 3, flatOut)
	if err != nil {
		t.Fatalf("RSI failed: %v", err)
	}
	if flatOut[3] != 100 {
		t.Errorf("Expected flat RSI to be 100, got %f", flatOut[3])
	}

	// Insufficient data
	if err := RSI(prices[:5], 14, out); err == nil {
		t.Error("expected error for RSI with period >= prices len")
	}

	// Output capacity too small
	smallOut := make([]float64, 2)
	if err := RSI(prices, 14, smallOut); err == nil {
		t.Error("expected error for RSI with small output slice")
	}
}

func TestMACD(t *testing.T) {
	prices := make([]float64, 40)
	for i := 0; i < 40; i++ {
		prices[i] = 100.0 + float64(i)
	}

	fastBuf := make([]float64, len(prices))
	slowBuf := make([]float64, len(prices))
	macdOut := make([]float64, len(prices))
	sigOut := make([]float64, len(prices))
	histOut := make([]float64, len(prices))

	err := MACD(prices, 12, 26, 9, fastBuf, slowBuf, macdOut, sigOut, histOut)
	if err != nil {
		t.Fatalf("MACD failed: %v", err)
	}

	// Error propagation (fastP > len(prices))
	err = MACD(prices[:10], 12, 26, 9, fastBuf[:10], slowBuf[:10], macdOut[:10], sigOut[:10], histOut[:10])
	if err == nil {
		t.Error("expected error for MACD with insufficient prices")
	}
}

func TestMFI(t *testing.T) {
	n := 20
	high := make([]float64, n)
	low := make([]float64, n)
	close := make([]float64, n)
	volume := make([]float64, n)

	for i := 0; i < n; i++ {
		high[i] = 15.0 + float64(i)
		low[i] = 10.0 + float64(i)
		close[i] = 12.0 + float64(i)
		volume[i] = 1000.0
	}

	pmfBuf := make([]float64, n)
	nmfBuf := make([]float64, n)
	out := make([]float64, n)

	// Normal case
	err := MFI(high, low, close, volume, 14, pmfBuf, nmfBuf, out)
	if err != nil {
		t.Fatalf("MFI failed: %v", err)
	}

	// Flat/decreasing case where sumNMF = 0 should give 100.0
	if out[14] != 100.0 {
		t.Errorf("Expected MFI for strictly increasing prices to be 100.0, got %f", out[14])
	}

	// Decreasing prices (sumPMF = 0)
	for i := 0; i < n; i++ {
		high[i] = 100.0 - float64(i)
		low[i] = 90.0 - float64(i)
		close[i] = 95.0 - float64(i)
	}
	err = MFI(high, low, close, volume, 14, pmfBuf, nmfBuf, out)
	if err != nil {
		t.Fatalf("MFI failed: %v", err)
	}
	if out[14] != 0.0 {
		t.Errorf("Expected MFI for strictly decreasing prices to be 0.0, got %f", out[14])
	}

	// Flat price case (tp = tpPrev, sumPMF = sumNMF = 0) -> sumNMF = 0 => MFI = 100.0
	for i := 0; i < n; i++ {
		high[i] = 10.0
		low[i] = 10.0
		close[i] = 10.0
	}
	err = MFI(high, low, close, volume, 14, pmfBuf, nmfBuf, out)
	if err != nil {
		t.Fatalf("MFI failed: %v", err)
	}
	if out[14] != 100.0 {
		t.Errorf("Expected MFI for flat typical price to be 100.0, got %f", out[14])
	}

	// Insufficient data
	if err := MFI(high[:10], low[:10], close[:10], volume[:10], 14, pmfBuf[:10], nmfBuf[:10], out[:10]); err == nil {
		t.Error("expected error for MFI with insufficient data")
	}

	// Period <= 0
	if err := MFI(high, low, close, volume, 0, pmfBuf, nmfBuf, out); err == nil {
		t.Error("expected error for MFI with period <= 0")
	}

	// Input slice lengths mismatch
	if err := MFI(high[:19], low, close, volume, 14, pmfBuf, nmfBuf, out); err == nil {
		t.Error("expected error for MFI with high slice length mismatch")
	}

	// Buffer or output slice capacities too small
	if err := MFI(high, low, close, volume, 14, pmfBuf[:10], nmfBuf, out); err == nil {
		t.Error("expected error for MFI with small pmfBuf")
	}
}

func TestATR(t *testing.T) {
	n := 20
	high := make([]float64, n)
	low := make([]float64, n)
	close := make([]float64, n)

	for i := 0; i < n; i++ {
		high[i] = 15.0 + float64(i)
		low[i] = 10.0 + float64(i)
		close[i] = 12.0 + float64(i)
	}

	trBuf := make([]float64, n)
	out := make([]float64, n)

	// Normal case
	err := ATR(high, low, close, 14, trBuf, out)
	if err != nil {
		t.Fatalf("ATR failed: %v", err)
	}

	// Since high - low is always 5 and previous close is within range, TR is always 5.
	// Thus, ATR should be exactly 5.0.
	expectedATR := 5.0
	for i := 14; i < n; i++ {
		if math.Abs(out[i]-expectedATR) > 1e-9 {
			t.Errorf("Index %d: expected ATR %f, got %f", i, expectedATR, out[i])
		}
	}

	// Insufficient data
	if err := ATR(high[:10], low[:10], close[:10], 14, trBuf[:10], out[:10]); err == nil {
		t.Error("expected error for ATR with short data")
	}

	// Period <= 0
	if err := ATR(high, low, close, 0, trBuf, out); err == nil {
		t.Error("expected error for ATR with period <= 0")
	}

	// Input length mismatch
	if err := ATR(high[:19], low, close, 14, trBuf, out); err == nil {
		t.Error("expected error for ATR with high length mismatch")
	}

	// Small buffers
	if err := ATR(high, low, close, 14, trBuf[:10], out); err == nil {
		t.Error("expected error for ATR with small trBuf")
	}
}

func TestBollingerBands(t *testing.T) {
	n := 30
	prices := make([]float64, n)
	for i := 0; i < n; i++ {
		prices[i] = 100.0 + float64(i%5)
	}

	mid := make([]float64, n)
	upper := make([]float64, n)
	lower := make([]float64, n)

	// Normal case
	err := BollingerBands(prices, 20, 2.0, mid, upper, lower)
	if err != nil {
		t.Fatalf("BollingerBands failed: %v", err)
	}

	// Verify upper is always greater than or equal to lower and assert exact values.
	// Since the input prices repeat [100, 101, 102, 103, 104] every 5 elements,
	// any window of 20 elements has a constant mean of 102.0 and variance of 2.0.
	expectedMid := 102.0
	expectedStdDev := math.Sqrt(2.0)
	expectedUpper := expectedMid + 2.0*expectedStdDev
	expectedLower := expectedMid - 2.0*expectedStdDev

	for i := 19; i < n; i++ {
		if upper[i] < lower[i] {
			t.Errorf("Index %d: Upper band %f less than Lower band %f", i, upper[i], lower[i])
		}
		if math.Abs(mid[i]-expectedMid) > 1e-9 {
			t.Errorf("Index %d: expected mid %f, got %f", i, expectedMid, mid[i])
		}
		if math.Abs(upper[i]-expectedUpper) > 1e-9 {
			t.Errorf("Index %d: expected upper %f, got %f", i, expectedUpper, upper[i])
		}
		if math.Abs(lower[i]-expectedLower) > 1e-9 {
			t.Errorf("Index %d: expected lower %f, got %f", i, expectedLower, lower[i])
		}
	}

	// Insufficient data
	if err := BollingerBands(prices[:10], 20, 2.0, mid[:10], upper[:10], lower[:10]); err == nil {
		t.Error("expected error for BollingerBands with short data")
	}

	// Period <= 0
	if err := BollingerBands(prices, 0, 2.0, mid, upper, lower); err == nil {
		t.Error("expected error for BollingerBands with period <= 0")
	}

	// Output capacity too small
	if err := BollingerBands(prices, 20, 2.0, mid[:10], upper, lower); err == nil {
		t.Error("expected error for BollingerBands with small mid output slice")
	}
}

func TestDynamicIndicatorResolver(t *testing.T) {
	// Let's create enough candles to satisfy MACD (at least 35 candles for 12, 26, 9 MACD calculation)
	candles := make([]dataflow.Candle, 50)
	now := time.Now()
	for i := 0; i < 50; i++ {
		candles[i] = dataflow.Candle{
			Time:   now.Add(time.Duration(i) * time.Minute),
			Open:   100.0 + float64(i),
			High:   105.0 + float64(i),
			Low:    95.0 + float64(i),
			Close:  102.0 + float64(i),
			Volume: 1000.0 + float64(i),
		}
	}

	tradeDate := now.Add(60 * time.Minute)

	// Helper function to get a clean resolver
	newResolver := func() *DynamicIndicatorResolver {
		return NewDynamicIndicatorResolver(NewIndicatorCache())
	}

	// Test cache hit
	t.Run("Cache Hit", func(t *testing.T) {
		cache := NewIndicatorCache()
		resolver := NewDynamicIndicatorResolver(cache)
		cache.Put("TEST", "cached_val", tradeDate, 123.45)
		cVal, err := resolver.Resolve(context.Background(), candles, "TEST", "cached_val", tradeDate)
		if err != nil || cVal != 123.45 {
			t.Fatalf("Cache hit failed: val=%f, err=%v", cVal, err)
		}
	})

	// Test MACD resolver
	for _, ind := range []string{"macd", "macds", "macdh"} {
		t.Run(ind, func(t *testing.T) {
			resolver := newResolver()
			_, err := resolver.Resolve(context.Background(), candles, "TEST", ind, tradeDate)
			if err != nil {
				t.Fatalf("Resolve %s failed: %v", ind, err)
			}
		})
	}

	// Test MACD insufficient data error
	t.Run("MACD Insufficient Data", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles[:20], "TEST", "macd", tradeDate); err == nil {
			t.Error("expected error for MACD with <26 candles")
		}
	})

	// Test Bollinger resolver
	t.Run("boll", func(t *testing.T) {
		resolver := newResolver()
		val, err := resolver.Resolve(context.Background(), candles, "TEST", "boll", tradeDate)
		if err != nil {
			t.Fatalf("Resolve boll failed: %v", err)
		}
		if math.Abs(val-141.5) > 1e-9 {
			t.Errorf("Expected boll to be 141.5, got %f", val)
		}
	})
	t.Run("boll_ub", func(t *testing.T) {
		resolver := newResolver()
		val, err := resolver.Resolve(context.Background(), candles, "TEST", "boll_ub", tradeDate)
		if err != nil {
			t.Fatalf("Resolve boll_ub failed: %v", err)
		}
		expected := 141.5 + 2.0*math.Sqrt(33.25)
		if math.Abs(val-expected) > 1e-9 {
			t.Errorf("Expected boll_ub to be %f, got %f", expected, val)
		}
	})
	t.Run("boll_lb", func(t *testing.T) {
		resolver := newResolver()
		val, err := resolver.Resolve(context.Background(), candles, "TEST", "boll_lb", tradeDate)
		if err != nil {
			t.Fatalf("Resolve boll_lb failed: %v", err)
		}
		expected := 141.5 - 2.0*math.Sqrt(33.25)
		if math.Abs(val-expected) > 1e-9 {
			t.Errorf("Expected boll_lb to be %f, got %f", expected, val)
		}
	})

	// Test Bollinger insufficient data error
	t.Run("Bollinger Insufficient Data", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles[:15], "TEST", "boll", tradeDate); err == nil {
			t.Error("expected error for Bollinger with <20 candles")
		}
	})

	// Test RSI resolver
	t.Run("RSI Resolver", func(t *testing.T) {
		resolver := newResolver()
		valRSI, err := resolver.Resolve(context.Background(), candles, "TEST", "rsi", tradeDate)
		if err != nil {
			t.Fatalf("Resolve rsi failed: %v", err)
		}
		if valRSI == 0 {
			t.Error("Resolve rsi returned 0")
		}
	})

	// Test RSI insufficient data error
	t.Run("RSI Insufficient Data", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles[:10], "TEST", "rsi", tradeDate); err == nil {
			t.Error("expected error for RSI with <15 candles")
		}
	})

	// Test ATR resolver
	t.Run("ATR Resolver", func(t *testing.T) {
		resolver := newResolver()
		valATR, err := resolver.Resolve(context.Background(), candles, "TEST", "atr", tradeDate)
		if err != nil {
			t.Fatalf("Resolve atr failed: %v", err)
		}
		if valATR == 0 {
			t.Error("Resolve atr returned 0")
		}
	})

	// Test ATR insufficient data error
	t.Run("ATR Insufficient Data", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles[:10], "TEST", "atr", tradeDate); err == nil {
			t.Error("expected error for ATR with <15 candles")
		}
	})

	// Test MFI resolver
	t.Run("MFI Resolver", func(t *testing.T) {
		resolver := newResolver()
		valMFI, err := resolver.Resolve(context.Background(), candles, "TEST", "mfi", tradeDate)
		if err != nil {
			t.Fatalf("Resolve mfi failed: %v", err)
		}
		if math.Abs(valMFI-100.0) > 1e-9 {
			t.Errorf("Expected MFI to be 100.0, got %f", valMFI)
		}
	})

	// Test MFI insufficient data error
	t.Run("MFI Insufficient Data", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles[:10], "TEST", "mfi", tradeDate); err == nil {
			t.Error("expected error for MFI with <15 candles")
		}
	})

	// Test dynamic SMA
	t.Run("Dynamic SMA", func(t *testing.T) {
		resolver := newResolver()
		valDynSMA, err := resolver.Resolve(context.Background(), candles, "TEST", "close_3_sma", tradeDate)
		if err != nil || valDynSMA != 150.0 { // last 3 closes: 102+47=149, 102+48=150, 102+49=151 => avg = 150
			t.Fatalf("Resolve close_3_sma failed: got %f, err: %v", valDynSMA, err)
		}
	})

	// Test dynamic EMA
	t.Run("Dynamic EMA", func(t *testing.T) {
		resolver := newResolver()
		val, err := resolver.Resolve(context.Background(), candles, "TEST", "close_3_ema", tradeDate)
		if err != nil {
			t.Fatalf("Resolve close_3_ema failed: %v", err)
		}
		if math.Abs(val-150.0) > 1e-9 {
			t.Errorf("Expected close_3_ema to be 150.0, got %f", val)
		}
	})

	// Test dynamic RSI
	t.Run("Dynamic RSI", func(t *testing.T) {
		resolver := newResolver()
		val, err := resolver.Resolve(context.Background(), candles, "TEST", "close_3_rsi", tradeDate)
		if err != nil {
			t.Fatalf("Resolve close_3_rsi failed: %v", err)
		}
		if math.Abs(val-100.0) > 1e-9 {
			t.Errorf("Expected close_3_rsi to be 100.0, got %f", val)
		}
	})

	// Test dynamic resolver error cases:
	// Unsupported dynamic indicator type
	t.Run("Unsupported Dynamic Type", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles, "TEST", "close_3_unknown", tradeDate); err == nil {
			t.Error("expected error for close_3_unknown")
		}
	})

	// Unsupported price target
	t.Run("Unsupported Price Target", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles, "TEST", "unknown_3_sma", tradeDate); err == nil {
			t.Error("expected error for unknown_3_sma")
		}
	})

	// Invalid indicator format
	t.Run("Invalid Format", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles, "TEST", "close_sma", tradeDate); err == nil {
			t.Error("expected error for close_sma format")
		}
	})

	// Invalid period number
	t.Run("Invalid Period", func(t *testing.T) {
		resolver := newResolver()
		if _, err := resolver.Resolve(context.Background(), candles, "TEST", "close_abc_sma", tradeDate); err == nil {
			t.Error("expected error for close_abc_sma period")
		}
	})
}
