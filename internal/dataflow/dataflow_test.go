package dataflow

import (
	"testing"
	"time"
)

func TestTokenBucket(t *testing.T) {
	// Capacity 2, refill rate 10 tokens per second
	tb := NewTokenBucket(2.0, 10.0)

	// Consume first 2
	if !tb.Allow() {
		t.Error("Expected first token allowance")
	}
	if !tb.Allow() {
		t.Error("Expected second token allowance")
	}
	// Third should be rejected immediately
	if tb.Allow() {
		t.Error("Expected third token allowance to be rejected")
	}

	// Wait 100ms for 1 token to refill (10 tokens/sec * 0.1s = 1 token)
	time.Sleep(105 * time.Millisecond)

	if !tb.Allow() {
		t.Error("Expected token allowance after refill")
	}
}

func TestParseCSVRowFast(t *testing.T) {
	row := []byte("2026-05-15,100.00,105.00,99.00,104.00,104.00,10000")

	// 1. If tradeDate is before row date -> Row should be discarded (valid = false)
	tradeDateBefore := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	_, valid, err := parseCSVRowFast(row, tradeDateBefore)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if valid {
		t.Error("Row dated after tradeDate should be discarded")
	}

	// 2. If tradeDate is equal to row date -> Row should be accepted (valid = true)
	tradeDateSame := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	candle, valid, err := parseCSVRowFast(row, tradeDateSame)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !valid {
		t.Error("Row dated on tradeDate should be accepted")
	}
	if candle.Close != 104.0 {
		t.Errorf("Expected close = 104.0, got %f", candle.Close)
	}
	if candle.Open != 100.0 {
		t.Errorf("Expected open = 100.0, got %f", candle.Open)
	}
	if candle.High != 105.0 {
		t.Errorf("Expected high = 105.0, got %f", candle.High)
	}
	if candle.Low != 99.0 {
		t.Errorf("Expected low = 99.0, got %f", candle.Low)
	}
	if candle.Volume != 10000.0 {
		t.Errorf("Expected volume = 10000.0, got %f", candle.Volume)
	}

	// 3. Invalid row columns length should fail
	badRow := []byte("2026-05-15,100.00,105.00")
	_, _, err = parseCSVRowFast(badRow, tradeDateSame)
	if err == nil {
		t.Error("Expected error on incomplete columns row")
	}
}
