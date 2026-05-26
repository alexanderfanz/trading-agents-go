package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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

	// 5. Clear checkpoints
	err = cp.Clear(ctx, ticker, tradeDate)
	if err != nil {
		t.Fatalf("failed to clear checkpoints: %v", err)
	}

	// 6. Load and ensure deleted
	state, stepIndex, err = cp.Load(ctx, ticker, tradeDate)
	if err != nil {
		t.Fatalf("failed to load state after clearing: %v", err)
	}
	if stepIndex != -1 || state != nil {
		t.Errorf("expected checkpoint to be deleted, but loaded: %d", stepIndex)
	}
}

func validValidationArgs() ValidationArgs {
	return ValidationArgs{
		ExpectedTicker:    "AAPL",
		ExpectedTradeDate: "2026-05-22",
		SystemVersion:     "1.0.0",
		MaxTimeDriftSec:   3600,
	}
}

func newValidTradingState(t *testing.T) *TradingState {
	t.Helper()

	state := &TradingState{
		Ticker:           "AAPL",
		TradeDate:        "2026-05-22",
		StepIndex:        1,
		Version:          "1.0.0",
		UpdatedTimestamp: time.Now().Unix(),
		Portfolio: PortfolioState{
			Cash:        1000.0,
			TotalEquity: 1000.0,
		},
	}
	if err := setTradingStateChecksum(state); err != nil {
		t.Fatalf("failed to set checksum: %v", err)
	}
	return state
}

func setTradingStateChecksum(state *TradingState) error {
	originalChecksum := state.Checksum
	state.Checksum = ""

	rawBytes, err := json.Marshal(state)
	state.Checksum = originalChecksum
	if err != nil {
		return err
	}

	hasher := sha256.New()
	hasher.Write(rawBytes)
	state.Checksum = fmt.Sprintf("%x", hasher.Sum(nil))
	return nil
}

func TestValidateCheckpoint(t *testing.T) {
	t.Run("checksum mismatch", func(t *testing.T) {
		state := newValidTradingState(t)
		state.Checksum = "deadbeef"

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected checksum mismatch error")
		}
		if !strings.Contains(err.Error(), "integrity violation: checksum mismatch") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		state := newValidTradingState(t)
		state.Version = "9.9.9"
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected version mismatch error")
		}
		if !strings.Contains(err.Error(), "compatibility failure") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ticker mismatch", func(t *testing.T) {
		state := newValidTradingState(t)
		state.Ticker = "MSFT"
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected ticker mismatch error")
		}
		if !strings.Contains(err.Error(), "ticker mismatch") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("trade date mismatch", func(t *testing.T) {
		state := newValidTradingState(t)
		state.TradeDate = "2026-01-01"
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected trade date mismatch error")
		}
		if !strings.Contains(err.Error(), "trade date mismatch") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("negative step index", func(t *testing.T) {
		state := newValidTradingState(t)
		state.StepIndex = -1
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected negative step index error")
		}
		if !strings.Contains(err.Error(), "step index") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("negative cash", func(t *testing.T) {
		state := newValidTradingState(t)
		state.Portfolio.Cash = -1.0
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected negative cash error")
		}
		if !strings.Contains(err.Error(), "portfolio cash") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("negative total equity", func(t *testing.T) {
		state := newValidTradingState(t)
		state.Portfolio.TotalEquity = -1.0
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected invalid total equity error")
		}
		if !strings.Contains(err.Error(), "total equity is invalid") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nan total equity", func(t *testing.T) {
		state := newValidTradingState(t)
		state.Portfolio.TotalEquity = math.NaN()
		state.Checksum = newValidTradingState(t).Checksum

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected checksum marshal failure for NaN equity")
		}
		if !strings.Contains(err.Error(), "checksum evaluation marshal failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stale timestamp", func(t *testing.T) {
		state := newValidTradingState(t)
		state.UpdatedTimestamp = time.Now().Unix() - 7200
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		args := validValidationArgs()
		args.MaxTimeDriftSec = 3600

		err := ValidateCheckpoint(state, args)
		if err == nil {
			t.Fatal("expected stale checkpoint error")
		}
		if !strings.Contains(err.Error(), "checkpoint is stale") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("future timestamp", func(t *testing.T) {
		state := newValidTradingState(t)
		state.UpdatedTimestamp = time.Now().Unix() + 3600
		if err := setTradingStateChecksum(state); err != nil {
			t.Fatalf("failed to set checksum: %v", err)
		}

		err := ValidateCheckpoint(state, validValidationArgs())
		if err == nil {
			t.Fatal("expected future timestamp error")
		}
		if !strings.Contains(err.Error(), "timestamp is in the future") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid checkpoint", func(t *testing.T) {
		state := newValidTradingState(t)
		if err := ValidateCheckpoint(state, validValidationArgs()); err != nil {
			t.Fatalf("expected valid checkpoint, got error: %v", err)
		}
	})
}
