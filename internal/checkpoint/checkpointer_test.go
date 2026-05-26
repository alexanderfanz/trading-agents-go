package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLConnectionManagerAndCheckpointer(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "checkpoint_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	dbPath := filepath.Join(tempDir, "checkpoints.db")

	// Initialize manager
	mgr, err := NewSQLConnectionManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create connection manager: %v", err)
	}
	defer func() {
		_ = mgr.Close()
	}()

	// Initialize checkpointer
	cp := NewStateCheckpointer(mgr)

	ctx := context.Background()
	ticker := "AAPL"
	tradeDate := "2026-05-22"

	// 1. Assert Load returns -1 when no checkpoint exists
	state, stepIndex, err := cp.Load(ctx, ticker, tradeDate)
	if err != nil {
		t.Fatalf("unexpected error on loading empty state: %v", err)
	}
	if stepIndex != -1 || state != nil {
		t.Errorf("expected stepIndex -1 and nil state, got %d and %+v", stepIndex, state)
	}

	// 2. Save a state
	initialState := &TradingState{
		Ticker:    ticker,
		TradeDate: tradeDate,
		StepIndex: 3,
		Portfolio: PortfolioState{
			Cash:        100000.0,
			TotalEquity: 100000.0,
			Holdings: map[string]float64{
				"AAPL": 10.0,
			},
		},
		SignalLogs: []SignalEntry{
			{
				Timestamp: time.Now().Unix(),
				Action:    "BUY",
				Price:     150.0,
				Quantity:  10.0,
				Reasoning: "Strong technical indicator signals",
			},
		},
		Metadata: map[string]string{
			"test": "value",
		},
	}

	err = cp.Save(ctx, initialState)
	if err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// 3. Load state and verify fields
	loadedState, stepIndex, err := cp.Load(ctx, ticker, tradeDate)
	if err != nil {
		t.Fatalf("failed to load saved state: %v", err)
	}

	if stepIndex != 3 {
		t.Errorf("expected stepIndex 3, got %d", stepIndex)
	}
	if loadedState.Ticker != ticker || loadedState.TradeDate != tradeDate {
		t.Errorf("loaded state context mismatch: %s %s", loadedState.Ticker, loadedState.TradeDate)
	}
	if loadedState.Portfolio.Cash != 100000.0 || loadedState.Portfolio.Holdings["AAPL"] != 10.0 {
		t.Errorf("portfolio values mismatch: %+v", loadedState.Portfolio)
	}
	if len(loadedState.SignalLogs) != 1 || loadedState.SignalLogs[0].Action != "BUY" {
		t.Errorf("signal logs mismatch: %+v", loadedState.SignalLogs)
	}

	// 4. Validate checkpoint verification
	valArgs := ValidationArgs{
		ExpectedTicker:    ticker,
		ExpectedTradeDate: tradeDate,
		SystemVersion:     "1.0.0",
		MaxTimeDriftSec:   60,
	}
	err = ValidateCheckpoint(loadedState, valArgs)
	if err != nil {
		t.Errorf("checkpoint validation failed: %v", err)
	}

	// 5. Trigger CleanupWorker executePruneAndVacuum
	worker := NewCleanupWorker(mgr, 100*time.Millisecond, 1024, 0)
	worker.executePruneAndVacuum(ctx)

	// 6. Clear checkpoints
	err = cp.Clear(ctx, ticker, tradeDate)
	if err != nil {
		t.Fatalf("failed to clear checkpoints: %v", err)
	}

	// 7. Load and ensure deleted
	state, stepIndex, err = cp.Load(ctx, ticker, tradeDate)
	if err != nil {
		t.Fatalf("failed to load state after clearing: %v", err)
	}
	if stepIndex != -1 || state != nil {
		t.Errorf("expected checkpoint to be deleted, but loaded: %d", stepIndex)
	}
}
