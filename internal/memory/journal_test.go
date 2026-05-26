package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"trading-agents-go/internal/dataflow"
	"trading-agents-go/pkg/provider"
)

type mockDataProvider struct {
	stockCandles []dataflow.Candle
	benchCandles []dataflow.Candle
	err          error
}

func (m *mockDataProvider) FetchOHLCV(ctx context.Context, ticker string, start, end time.Time, tradeDate time.Time) ([]dataflow.Candle, error) {
	if m.err != nil {
		return nil, m.err
	}
	if ticker == "SPY" || ticker == "^N225" {
		return m.benchCandles, nil
	}
	return m.stockCandles, nil
}

func (m *mockDataProvider) FetchFundamentals(ctx context.Context, ticker string, tradeDate time.Time) (string, error) {
	return "", nil
}

type mockLLMProvider struct {
	reply string
	err   error
}

func (m *mockLLMProvider) Generate(ctx context.Context, req provider.LLMRequest) (string, error) {
	return m.reply, m.err
}

func (m *mockLLMProvider) GenerateStructured(ctx context.Context, req provider.LLMRequest, target interface{}) error {
	return nil
}

func TestJournalParsingAndStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "journal_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logFile := filepath.Join(tmpDir, "test_memory.md")
	log := NewTradingMemoryLog(logFile, 0)

	// 1. Store first decision
	err = log.StoreDecision("TSLA", "2026-05-15", "Rating: Buy\nThis is the initial conviction.")
	if err != nil {
		t.Fatalf("StoreDecision failed: %v", err)
	}

	// Verify file was created and contains expected content
	entries, err := log.LoadEntries()
	if err != nil {
		t.Fatalf("LoadEntries failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Ticker != "TSLA" || entry.Date != "2026-05-15" || entry.Rating != "Buy" || !entry.Pending {
		t.Errorf("entry fields mismatch: %+v", entry)
	}
	if entry.Decision != "Rating: Buy\nThis is the initial conviction." {
		t.Errorf("expected decision prose, got %q", entry.Decision)
	}

	// 2. Test Idempotency (no duplicate append)
	err = log.StoreDecision("TSLA", "2026-05-15", "Rating: Buy\nThis is a duplicate conviction.")
	if err != nil {
		t.Fatalf("second StoreDecision failed: %v", err)
	}
	entries, err = log.LoadEntries()
	if err != nil {
		t.Fatalf("second LoadEntries failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected still 1 entry due to idempotency, got %d", len(entries))
	}
}

func TestPastContextRetrieval(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "past_context_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logFile := filepath.Join(tmpDir, "test_memory.md")
	log := NewTradingMemoryLog(logFile, 0)

	// Write past resolved entries
	updates := []OutcomeUpdate{
		{Ticker: "TSLA", TradeDate: "2026-05-10", RawReturn: 0.10, AlphaReturn: 0.05, HoldingDays: 5, Reflection: "Thesis on TSLA held perfectly."},
		{Ticker: "AAPL", TradeDate: "2026-05-11", RawReturn: -0.02, AlphaReturn: -0.01, HoldingDays: 5, Reflection: "Thesis on AAPL failed due to macro."},
		{Ticker: "TSLA", TradeDate: "2026-05-12", RawReturn: 0.04, AlphaReturn: 0.02, HoldingDays: 5, Reflection: "Conviction on TSLA continued to pay off."},
	}

	// Add decisions as pending first so we can resolve them
	_ = log.StoreDecision("TSLA", "2026-05-10", "Rating: Buy\nBuy TSLA.")
	_ = log.StoreDecision("AAPL", "2026-05-11", "Rating: Hold\nHold AAPL.")
	_ = log.StoreDecision("TSLA", "2026-05-12", "Rating: Buy\nBuy TSLA again.")

	err = log.BatchUpdateWithOutcomes(updates)
	if err != nil {
		t.Fatalf("BatchUpdateWithOutcomes failed: %v", err)
	}

	// Retrieve past context for TSLA
	ctxStr, err := log.GetPastContext("TSLA", 5, 3)
	if err != nil {
		t.Fatalf("GetPastContext failed: %v", err)
	}

	// Check if same-ticker and cross-ticker are present and formatted
	if !strings.Contains(ctxStr, "Past analyses of TSLA") {
		t.Errorf("expected same-ticker section, got:\n%s", ctxStr)
	}
	if !strings.Contains(ctxStr, "Recent cross-ticker lessons:") {
		t.Errorf("expected cross-ticker section, got:\n%s", ctxStr)
	}
	if !strings.Contains(ctxStr, "Thesis on AAPL failed") {
		t.Errorf("expected AAPL cross-ticker lesson inside context, got:\n%s", ctxStr)
	}
}

func TestJournalRotation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rotation_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logFile := filepath.Join(tmpDir, "test_memory.md")
	// Cap resolved entries at 2
	log := NewTradingMemoryLog(logFile, 2)

	_ = log.StoreDecision("TSLA", "2026-05-10", "Rating: Buy\n1")
	_ = log.StoreDecision("AAPL", "2026-05-11", "Rating: Hold\n2")
	_ = log.StoreDecision("MSFT", "2026-05-12", "Rating: Buy\n3")

	updates := []OutcomeUpdate{
		{Ticker: "TSLA", TradeDate: "2026-05-10", RawReturn: 0.05, AlphaReturn: 0.02, HoldingDays: 5, Reflection: "TSLA lesson"},
		{Ticker: "AAPL", TradeDate: "2026-05-11", RawReturn: -0.01, AlphaReturn: -0.02, HoldingDays: 5, Reflection: "AAPL lesson"},
		{Ticker: "MSFT", TradeDate: "2026-05-12", RawReturn: 0.03, AlphaReturn: 0.01, HoldingDays: 5, Reflection: "MSFT lesson"},
	}

	err = log.BatchUpdateWithOutcomes(updates)
	if err != nil {
		t.Fatalf("batch update failed: %v", err)
	}

	entries, _ := log.LoadEntries()
	// Oldest resolved entry (TSLA) should have been pruned, leaving AAPL and MSFT
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Ticker != "AAPL" || entries[1].Ticker != "MSFT" {
		t.Errorf("rotation did not prune the oldest resolved entry correctly: %+v", entries)
	}
}

func TestOutcomeResolutionPipeline(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logFile := filepath.Join(tmpDir, "test_memory.md")
	log := NewTradingMemoryLog(logFile, 0)

	// Store pending decision
	tradeDateStr := "2026-05-10"
	tradeDate, _ := time.Parse("2006-01-02", tradeDateStr)

	_ = log.StoreDecision("TSLA", tradeDateStr, "Rating: Buy\nBuy TSLA.")

	// Mock pricing candles
	stockCandles := []dataflow.Candle{
		{Time: tradeDate, Close: 200.0},
		{Time: tradeDate.AddDate(0, 0, 1), Close: 210.0},
		{Time: tradeDate.AddDate(0, 0, 2), Close: 205.0},
		{Time: tradeDate.AddDate(0, 0, 3), Close: 215.0},
		{Time: tradeDate.AddDate(0, 0, 4), Close: 220.0},
		{Time: tradeDate.AddDate(0, 0, 5), Close: 220.0},
	}
	benchCandles := []dataflow.Candle{
		{Time: tradeDate, Close: 400.0},
		{Time: tradeDate.AddDate(0, 0, 1), Close: 404.0},
		{Time: tradeDate.AddDate(0, 0, 2), Close: 402.0},
		{Time: tradeDate.AddDate(0, 0, 3), Close: 408.0},
		{Time: tradeDate.AddDate(0, 0, 4), Close: 412.0},
		{Time: tradeDate.AddDate(0, 0, 5), Close: 412.0},
	}

	mockDP := &mockDataProvider{
		stockCandles: stockCandles,
		benchCandles: benchCandles,
	}

	mockLLM := &mockLLMProvider{
		reply: "TSLA rose 10.0% outperforming SPY by 7.0%. Thesis held.",
	}

	reflector := NewReflector(mockLLM)

	ctx := context.Background()
	err = ResolvePendingEntries(ctx, "TSLA", log, reflector, mockDP, "SPY")
	if err != nil {
		t.Fatalf("ResolvePendingEntries failed: %v", err)
	}

	// Reload entries and confirm resolution
	entries, _ := log.LoadEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Pending {
		t.Fatalf("expected entry to be resolved, but is still pending")
	}

	if entry.RawReturn != "+10.0%" || entry.AlphaReturn != "+7.0%" || entry.HoldingDays != "5d" {
		t.Errorf("returns mismatch: raw=%s, alpha=%s, holding=%s", entry.RawReturn, entry.AlphaReturn, entry.HoldingDays)
	}

	if entry.Reflection != "TSLA rose 10.0% outperforming SPY by 7.0%. Thesis held." {
		t.Errorf("reflection mismatch: %q", entry.Reflection)
	}
}

func TestConcurrentJournalAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "concurrent_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logFile := filepath.Join(tmpDir, "test_memory.md")
	log := NewTradingMemoryLog(logFile, 0)

	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ticker := fmt.Sprintf("TCK%d", id)
			date := fmt.Sprintf("2026-05-%02d", 10+id)
			_ = log.StoreDecision(ticker, date, "Rating: Buy\nProse decision.")
		}(i)
	}
	wg.Wait()

	entries, err := log.LoadEntries()
	if err != nil {
		t.Fatalf("LoadEntries failed under concurrent writes: %v", err)
	}

	if len(entries) != workers {
		t.Errorf("expected %d entries, got %d", workers, len(entries))
	}
}
