package interactive

import (
	"path/filepath"
	"testing"
	"time"

	"trading-agents-go/internal/app"
	"trading-agents-go/internal/config"
)

func TestFormValuesApplyToConfig(t *testing.T) {
	cfg := config.LoadConfig()
	values := &formValues{
		ticker:             "msft",
		tradeDate:          time.Now().Format("2006-01-02"),
		outputLanguage:     "Spanish",
		provider:           "Mock",
		quickThinkLLM:      "quick-model",
		deepThinkLLM:       "deep-model",
		researchDepth:      3,
		checkpointEnabled:  true,
		createLocalReports: true,
		localReportsDir:    "reports/manual",
		resultsDir:         "logs/manual",
		cacheDir:           "cache/manual",
		memoryPath:         "memory/manual.md",
		dbPath:             filepath.Join("state", "checkpoints.db"),
		timeoutSeconds:     "42",
	}

	opts, err := values.applyTo(cfg)
	if err != nil {
		t.Fatalf("applyTo failed: %v", err)
	}

	if opts.Ticker != "MSFT" {
		t.Fatalf("expected normalized ticker MSFT, got %s", opts.Ticker)
	}
	if opts.DBPath != filepath.Join("state", "checkpoints.db") {
		t.Fatalf("unexpected db path: %s", opts.DBPath)
	}
	if cfg.LLMProvider != "mock" {
		t.Fatalf("expected provider mock, got %s", cfg.LLMProvider)
	}
	if cfg.MaxDebateRounds != 3 || cfg.MaxRiskDiscussRounds != 3 {
		t.Fatalf("expected research depth to update both round counts, got %d/%d", cfg.MaxDebateRounds, cfg.MaxRiskDiscussRounds)
	}
	if cfg.ExecutionTimeout != 42 {
		t.Fatalf("expected timeout 42, got %d", cfg.ExecutionTimeout)
	}
}

func TestNewFormValuesUsesDefaults(t *testing.T) {
	cfg := config.LoadConfig()
	defaults := app.RunOptions{
		Ticker:    "spy",
		TradeDate: "2026-01-02",
		DBPath:    filepath.Join("tmp", "checkpoints.db"),
	}

	values := newFormValues(cfg, defaults)

	if values.ticker != "SPY" {
		t.Fatalf("expected ticker SPY, got %s", values.ticker)
	}
	if values.tradeDate != "2026-01-02" {
		t.Fatalf("expected trade date from defaults, got %s", values.tradeDate)
	}
	if values.dbPath != filepath.Join("tmp", "checkpoints.db") {
		t.Fatalf("expected db path from defaults, got %s", values.dbPath)
	}
}

func TestApplyModelDefaultsForProvider(t *testing.T) {
	values := &formValues{
		provider:      "gemini",
		quickThinkLLM: "gpt-4o-mini",
		deepThinkLLM:  "gpt-4o",
	}

	values.applyModelDefaultsForProvider()

	if values.quickThinkLLM != "gemini-3.5-flash" {
		t.Fatalf("expected Gemini quick model default, got %s", values.quickThinkLLM)
	}
	if values.deepThinkLLM != "gemini-3.5-flash" {
		t.Fatalf("expected Gemini deep model default, got %s", values.deepThinkLLM)
	}
}

func TestValidateTicker(t *testing.T) {
	valid := []string{"AAPL", "CNC.TO", "7203.T", "BTC-USD", "^GSPC"}
	for _, ticker := range valid {
		if err := validateTicker(ticker); err != nil {
			t.Fatalf("expected %s to be valid: %v", ticker, err)
		}
	}

	invalid := []string{"", "../AAPL", "AAPL/../MSFT"}
	for _, ticker := range invalid {
		if err := validateTicker(ticker); err == nil {
			t.Fatalf("expected %s to be invalid", ticker)
		}
	}
}
