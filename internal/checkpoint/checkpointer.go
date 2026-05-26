package checkpoint

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure-Go CGO-free SQLite driver
)

// PortfolioState captures the active financial standing of the trading agent.
type PortfolioState struct {
	Cash             float64            `json:"cash" gob:"cash"`
	Holdings         map[string]float64 `json:"holdings" gob:"holdings"`
	TotalEquity      float64            `json:"total_equity" gob:"total_equity"`
	UpdatedTimestamp int64              `json:"updated_timestamp" gob:"updated_timestamp"`
}

// SignalEntry logs individual agent decision pathways.
type SignalEntry struct {
	Timestamp int64   `json:"timestamp" gob:"timestamp"`
	Action    string  `json:"action" gob:"action"`
	Price     float64 `json:"price" gob:"price"`
	Quantity  float64 `json:"quantity" gob:"quantity"`
	Reasoning string  `json:"reasoning" gob:"reasoning"`
}

// InvestDebateState tracks debate history and rounds.
type InvestDebateState struct {
	History       string `json:"history" gob:"history"`
	Count         int    `json:"count" gob:"count"`
	JudgeDecision string `json:"judge_decision" gob:"judge_decision"`
}

// RiskDebateState tracks risk debate history and rounds.
type RiskDebateState struct {
	History string `json:"history" gob:"history"`
	Count   int    `json:"count" gob:"count"`
}

// TradingState defines the central state checkpoint struct.
type TradingState struct {
	mu               sync.RWMutex      `json:"-" gob:"-"` // Thread-safe state access
	Ticker           string            `json:"ticker" gob:"ticker"`
	TradeDate        string            `json:"trade_date" gob:"trade_date"`
	StepIndex        int               `json:"step_index" gob:"step_index"`
	Portfolio        PortfolioState    `json:"portfolio" gob:"portfolio"`
	SignalLogs       []SignalEntry     `json:"signal_logs" gob:"signal_logs"`
	Metadata         map[string]string `json:"metadata" gob:"metadata"`
	Version          string            `json:"version" gob:"version"`
	Checksum         string            `json:"checksum" gob:"checksum"`
	UpdatedTimestamp int64             `json:"updated_timestamp" gob:"updated_timestamp"`

	// Orchestrator support fields
	InvestmentDebate        InvestDebateState `json:"investment_debate" gob:"investment_debate"`
	InvestmentPlan          string            `json:"investment_plan" gob:"investment_plan"`
	AnalystReports          map[string]string `json:"analyst_reports" gob:"analyst_reports"`
	TraderInvestmentPlan    string            `json:"trader_investment_plan" gob:"trader_investment_plan"`
	OptionsStrategy         string            `json:"options_strategy" gob:"options_strategy"`
	RiskDebate              RiskDebateState   `json:"risk_debate" gob:"risk_debate"`
	FinalTradeDecision      string            `json:"final_trade_decision" gob:"final_trade_decision"`
	BullDebateHistory       []string          `json:"bull_debate_history" gob:"bull_debate_history"`
	BearDebateHistory       []string          `json:"bear_debate_history" gob:"bear_debate_history"`
	AggressiveRiskHistory   []string          `json:"aggressive_risk_history" gob:"aggressive_risk_history"`
	ConservativeRiskHistory []string          `json:"conservative_risk_history" gob:"conservative_risk_history"`
	NeutralRiskHistory      []string          `json:"neutral_risk_history" gob:"neutral_risk_history"`
}

// Lock locks the state for thread-safe mutations.
func (s *TradingState) Lock() {
	s.mu.Lock()
}

// Unlock unlocks the state.
func (s *TradingState) Unlock() {
	s.mu.Unlock()
}

// RLock acquires a read lock on the state.
func (s *TradingState) RLock() {
	s.mu.RLock()
}

// RUnlock releases a read lock on the state.
func (s *TradingState) RUnlock() {
	s.mu.RUnlock()
}

// SQLConnectionManager controls thread-safe database interactions with split pools.
type SQLConnectionManager struct {
	writeDB *sql.DB // Exclusive write connection (MaxOpenConns = 1)
	readDB  *sql.DB // Shared read pool (MaxOpenConns = concurrent limit)
	dbPath  string
}

// NewSQLConnectionManager instantiates the split connection pools and executes migrations.
func NewSQLConnectionManager(dbPath string) (*SQLConnectionManager, error) {
	// 1. Configure the Write Pool DSN
	// We force _txlock=immediate to ensure all write transactions start with BEGIN IMMEDIATE.
	writeDSN := fmt.Sprintf("file:%s?_txlock=immediate&_journal_mode=WAL&_busy_timeout=5000&_sync=NORMAL", dbPath)
	writeDB, err := sql.Open("sqlite", writeDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open write sqlite db: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	writeDB.SetConnMaxLifetime(0) // Keep single connection open and reusable

	// 2. Configure the Read Pool DSN
	readDSN := fmt.Sprintf("file:%s?_txlock=deferred&_journal_mode=WAL&_busy_timeout=5000&_sync=NORMAL", dbPath)
	readDB, err := sql.Open("sqlite", readDSN)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("failed to open read sqlite db: %w", err)
	}
	readDB.SetMaxOpenConns(10) // Scale read queries concurrently
	readDB.SetMaxIdleConns(4)
	readDB.SetConnMaxLifetime(1 * time.Hour)

	// 3. Optimize connection settings programmatically
	pragmas := []string{
		"PRAGMA cache_size = -64000;",   // Allocate 64MB cache memory
		"PRAGMA temp_store = MEMORY;",   // Keep temp tables in memory
		"PRAGMA mmap_size = 268435456;", // Use memory mapping up to 256MB
	}
	for _, query := range pragmas {
		if _, err := writeDB.Exec(query); err != nil {
			_ = writeDB.Close()
			_ = readDB.Close()
			return nil, fmt.Errorf("failed to apply SQLite pragma '%s': %w", query, err)
		}
		if _, err := readDB.Exec(query); err != nil {
			_ = writeDB.Close()
			_ = readDB.Close()
			return nil, fmt.Errorf("failed to apply SQLite pragma '%s' on reader: %w", query, err)
		}
	}

	mgr := &SQLConnectionManager{
		writeDB: writeDB,
		readDB:  readDB,
		dbPath:  dbPath,
	}

	if err := mgr.migrate(); err != nil {
		_ = mgr.Close()
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	return mgr, nil
}

// Close gracefully terminates both pool handlers.
func (mgr *SQLConnectionManager) Close() error {
	var errs []error
	if err := mgr.writeDB.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := mgr.readDB.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed closing pools: %v", errs)
	}
	return nil
}

// migrate creates the tables and indexes necessary for state tracking.
func (mgr *SQLConnectionManager) migrate() error {
	migrationQuery := `
	CREATE TABLE IF NOT EXISTS checkpoints (
		id TEXT PRIMARY KEY,
		ticker TEXT NOT NULL,
		trade_date TEXT NOT NULL,
		step_index INTEGER NOT NULL,
		state_data BLOB NOT NULL,
		checksum TEXT NOT NULL,
		version TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_checkpoints_ticker_date ON checkpoints(ticker, trade_date);`

	_, err := mgr.writeDB.Exec(migrationQuery)
	return err
}

// StateCheckpointer handles serialization, compression, and DB transactions.
type StateCheckpointer struct {
	mgr *SQLConnectionManager
}

// NewStateCheckpointer instantiates the state checkpoint wrapper.
func NewStateCheckpointer(mgr *SQLConnectionManager) *StateCheckpointer {
	return &StateCheckpointer{mgr: mgr}
}

// Save marshals and compresses the current TradingState into the SQLite write database.
func (c *StateCheckpointer) Save(ctx context.Context, state *TradingState) error {
	state.Lock()
	defer state.Unlock()

	// Prepare state metadata values
	state.Version = "1.0.0"
	state.UpdatedTimestamp = time.Now().Unix()

	// 1. Serialization
	rawJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state structure: %w", err)
	}

	// 2. Integrity Hash Checksum creation
	hasher := sha256.New()
	hasher.Write(rawJSON)
	state.Checksum = fmt.Sprintf("%x", hasher.Sum(nil))

	// Re-marshal to inject correct checksum field
	finalJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal finalized state: %w", err)
	}

	// 3. Compress using gzip
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, wErr := zw.Write(finalJSON); wErr != nil {
		return fmt.Errorf("failed to compress state data: %w", wErr)
	}
	if cErr := zw.Close(); cErr != nil {
		return fmt.Errorf("failed to close gzip writer: %w", cErr)
	}

	id := fmt.Sprintf("%s:%s", state.Ticker, state.TradeDate)
	query := `
	INSERT INTO checkpoints (id, ticker, trade_date, step_index, state_data, checksum, version, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		step_index = excluded.step_index,
		state_data = excluded.state_data,
		checksum = excluded.checksum,
		version = excluded.version,
		updated_at = CURRENT_TIMESTAMP;`

	// Execute write on exclusive write connection
	_, err = c.mgr.writeDB.ExecContext(ctx, query, id, state.Ticker, state.TradeDate, state.StepIndex, buf.Bytes(), state.Checksum, state.Version)
	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}
	return nil
}

// Load attempts to retrieve and restore a prior state checkpoint. Returns stepIndex = -1 if none is found.
func (c *StateCheckpointer) Load(ctx context.Context, ticker, tradeDate string) (*TradingState, int, error) {
	id := fmt.Sprintf("%s:%s", ticker, tradeDate)
	query := `SELECT step_index, state_data FROM checkpoints WHERE id = ? LIMIT 1;`

	var stepIndex int
	var compressedBytes []byte
	// Execute read on concurrent read pool
	err := c.mgr.readDB.QueryRowContext(ctx, query, id).Scan(&stepIndex, &compressedBytes)
	if err != nil {
		return nil, -1, nil // Safe exit: no checkpoint exists
	}

	// Decompress gzip payload
	zr, err := gzip.NewReader(bytes.NewReader(compressedBytes))
	if err != nil {
		return nil, -1, fmt.Errorf("failed to initialize decompression reader: %w", err)
	}
	defer func() {
		_ = zr.Close()
	}()

	var state TradingState
	if err := json.NewDecoder(zr).Decode(&state); err != nil {
		return nil, -1, fmt.Errorf("failed to decode decompressed state structure: %w", err)
	}

	return &state, stepIndex, nil
}

// Clear prunes active checkpoints on successful workflow execution.
func (c *StateCheckpointer) Clear(ctx context.Context, ticker, tradeDate string) error {
	id := fmt.Sprintf("%s:%s", ticker, tradeDate)
	query := `DELETE FROM checkpoints WHERE id = ?;`
	_, err := c.mgr.writeDB.ExecContext(ctx, query, id)
	return err
}

// ValidationArgs details the current runtime parameters to check against.
type ValidationArgs struct {
	ExpectedTicker    string
	ExpectedTradeDate string
	SystemVersion     string
	MaxTimeDriftSec   int64 // Maximum age of checkpoint allowed for resumption
}

// ValidateCheckpoint performs comprehensive logical auditing of loaded checkpoints.
func ValidateCheckpoint(state *TradingState, args ValidationArgs) error {
	state.RLock()
	defer state.RUnlock()

	// Stage 1: Checksum verification
	originalChecksum := state.Checksum
	state.Checksum = "" // Zero out to match hashing layout

	rawBytes, err := json.Marshal(state)
	state.Checksum = originalChecksum // Restore state parameter
	if err != nil {
		return fmt.Errorf("checksum evaluation marshal failed: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(rawBytes)
	computedChecksum := fmt.Sprintf("%x", hasher.Sum(nil))
	if computedChecksum != originalChecksum {
		return fmt.Errorf("integrity violation: checksum mismatch (expected %s, computed %s)", originalChecksum, computedChecksum)
	}

	// Stage 2: Schema Compatibility
	if state.Version != args.SystemVersion {
		return fmt.Errorf("compatibility failure: loaded version (%s) incompatible with system (%s)", state.Version, args.SystemVersion)
	}

	// Stage 3: Execution Context Matching
	if state.Ticker != args.ExpectedTicker {
		return fmt.Errorf("context alignment error: ticker mismatch (loaded %s, active %s)", state.Ticker, args.ExpectedTicker)
	}
	if state.TradeDate != args.ExpectedTradeDate {
		return fmt.Errorf("context alignment error: trade date mismatch (loaded %s, active %s)", state.TradeDate, args.ExpectedTradeDate)
	}

	// Stage 4: Logical Invariant Audits
	if state.StepIndex < 0 {
		return fmt.Errorf("invariant failure: step index (%d) is negative", state.StepIndex)
	}

	if state.Portfolio.Cash < 0.0 {
		return fmt.Errorf("invariant failure: portfolio cash (%f) is negative", state.Portfolio.Cash)
	}
	if math.IsNaN(state.Portfolio.TotalEquity) || state.Portfolio.TotalEquity < 0.0 {
		return fmt.Errorf("invariant failure: total equity is invalid (%f)", state.Portfolio.TotalEquity)
	}

	// Check time drift to avoid resuming stale runs (e.g. running from weeks old cache)
	now := time.Now().Unix()
	drift := now - state.UpdatedTimestamp
	if drift < 0 {
		return fmt.Errorf("invariant failure: checkpoint timestamp is in the future")
	}
	if args.MaxTimeDriftSec > 0 && drift > args.MaxTimeDriftSec {
		return fmt.Errorf("resumption rejected: checkpoint is stale (age: %d seconds, limit: %d seconds)", drift, args.MaxTimeDriftSec)
	}

	return nil
}
