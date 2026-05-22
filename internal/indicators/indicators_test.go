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
	err := SMA(prices, 3, out)
	if err != nil {
		t.Fatalf("SMA failed: %v", err)
	}

	// First two elements should be 0.0 (unpopulated)
	// Third element: (10 + 11 + 12)/3 = 11.0
	// Fourth element: (11 + 12 + 13)/3 = 12.0
	// Fifth element: (12 + 13 + 14)/3 = 13.0
	expected := []float64{0.0, 0.0, 11.0, 12.0, 13.0}
	for i := 2; i < len(prices); i++ {
		if math.Abs(out[i]-expected[i]) > 1e-9 {
			t.Errorf("Index %d: expected %f, got %f", i, expected[i], out[i])
		}
	}
}

func TestEMA(t *testing.T) {
	prices := []float64{10.0, 11.0, 12.0, 13.0, 14.0}
	out := make([]float64, len(prices))
	err := EMA(prices, 3, out)
	if err != nil {
		t.Fatalf("EMA failed: %v", err)
	}
	// Initial SMA: (10+11+12)/3 = 11.0
	// Next: k = 2/(3+1) = 0.5
	// Out[3] = 13.0 * 0.5 + 11.0 * 0.5 = 12.0
	// Out[4] = 14.0 * 0.5 + 12.0 * 0.5 = 13.0
	expected := []float64{0.0, 0.0, 11.0, 12.0, 13.0}
	for i := 2; i < len(prices); i++ {
		if math.Abs(out[i]-expected[i]) > 1e-9 {
			t.Errorf("Index %d: expected %f, got %f", i, expected[i], out[i])
		}
	}
}

func TestDynamicIndicatorResolver(t *testing.T) {
	cache := NewIndicatorCache()
	resolver := NewDynamicIndicatorResolver(cache)

	candles := []dataflow.Candle{
		{Time: time.Now(), Close: 10.0, High: 11.0, Low: 9.0, Volume: 1000},
		{Time: time.Now(), Close: 11.0, High: 12.0, Low: 10.0, Volume: 1100},
		{Time: time.Now(), Close: 12.0, High: 13.0, Low: 11.0, Volume: 1200},
		{Time: time.Now(), Close: 13.0, High: 14.0, Low: 12.0, Volume: 1300},
		{Time: time.Now(), Close: 14.0, High: 15.0, Low: 13.0, Volume: 1400},
	}

	tradeDate := time.Now()
	val, err := resolver.Resolve(context.Background(), candles, "TEST", "close_3_sma", tradeDate)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	expected := 13.0 // Last 3 close prices: 12, 13, 14 => avg = 13.0
	if math.Abs(val-expected) > 1e-9 {
		t.Errorf("Expected %f, got %f", expected, val)
	}

	// Verify it got cached
	valCached, ok := cache.Get("TEST", "close_3_sma", tradeDate)
	if !ok || math.Abs(valCached-expected) > 1e-9 {
		t.Errorf("Caching failed")
	}
}
