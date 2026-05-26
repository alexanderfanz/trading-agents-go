package interactive

import (
	"errors"
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

func TestPromptForRunOptionsRequiresTTY(t *testing.T) {
	_, err := PromptForRunOptions(config.LoadConfig(), app.RunOptions{})
	if !errors.Is(err, ErrNonInteractive) {
		t.Fatalf("expected ErrNonInteractive, got %v", err)
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
	tests := []struct {
		provider string
		quick    string
		deep     string
	}{
		{provider: "openai", quick: "gpt-4o-mini", deep: "gpt-4o"},
		{provider: "gemini", quick: "gemini-3.5-flash", deep: "gemini-3.5-flash"},
		{provider: "google", quick: "gemini-3.5-flash", deep: "gemini-3.5-flash"},
		{provider: "anthropic", quick: "claude-3-7-sonnet", deep: "claude-3-7-sonnet"},
		{provider: "azure", quick: "gpt-4", deep: "gpt-4"},
		{provider: "xai", quick: "grok-4.20-reasoner", deep: "grok-4.20-reasoner"},
		{provider: "deepseek", quick: "deepseek-reasoner", deep: "deepseek-reasoner"},
		{provider: "qwen", quick: "qwen3.6-plus", deep: "qwen3.6-plus"},
		{provider: "qwen-cn", quick: "qwen3.6-plus", deep: "qwen3.6-plus"},
		{provider: "glm", quick: "glm-5", deep: "glm-5"},
		{provider: "glm-cn", quick: "glm-5", deep: "glm-5"},
		{provider: "minimax", quick: "MiniMax-M2.7", deep: "MiniMax-M2.7"},
		{provider: "minimax-cn", quick: "MiniMax-M2.7", deep: "MiniMax-M2.7"},
		{provider: "openrouter", quick: "meta-llama/llama-3", deep: "meta-llama/llama-3"},
		{provider: "ollama", quick: "qwen3:latest", deep: "qwen3:latest"},
		{provider: providerMock, quick: providerMock, deep: providerMock},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			values := &formValues{
				provider:      tc.provider,
				quickThinkLLM: "old-quick",
				deepThinkLLM:  "old-deep",
			}

			values.applyModelDefaultsForProvider()

			if values.quickThinkLLM != tc.quick {
				t.Fatalf("expected quick model %s, got %s", tc.quick, values.quickThinkLLM)
			}
			if values.deepThinkLLM != tc.deep {
				t.Fatalf("expected deep model %s, got %s", tc.deep, values.deepThinkLLM)
			}
		})
	}

	t.Run("unknown provider keeps existing values", func(t *testing.T) {
		values := &formValues{
			provider:      "unknown",
			quickThinkLLM: "old-quick",
			deepThinkLLM:  "old-deep",
		}

		values.applyModelDefaultsForProvider()

		if values.quickThinkLLM != "old-quick" || values.deepThinkLLM != "old-deep" {
			t.Fatalf("expected unknown provider to keep existing models, got %s/%s", values.quickThinkLLM, values.deepThinkLLM)
		}
	})
}

func TestApplyToRejectsInvalidValues(t *testing.T) {
	base := formValues{
		ticker:             "AAPL",
		tradeDate:          time.Now().Format("2006-01-02"),
		outputLanguage:     "English",
		provider:           "mock",
		quickThinkLLM:      "mock",
		deepThinkLLM:       "mock",
		researchDepth:      1,
		checkpointEnabled:  false,
		createLocalReports: true,
		localReportsDir:    "reports",
		resultsDir:         "logs",
		cacheDir:           "cache",
		memoryPath:         "memory.md",
		dbPath:             "checkpoints.db",
		timeoutSeconds:     "300",
	}

	tests := []struct {
		name   string
		mutate func(*formValues)
	}{
		{name: "invalid ticker", mutate: func(v *formValues) { v.ticker = "../AAPL" }},
		{name: "invalid date", mutate: func(v *formValues) { v.tradeDate = "not-a-date" }},
		{name: "invalid timeout", mutate: func(v *formValues) { v.timeoutSeconds = "zero" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values := base
			tc.mutate(&values)

			if _, err := values.applyTo(config.LoadConfig()); err == nil {
				t.Fatal("expected applyTo to reject invalid values")
			}
		})
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

func TestValidateTradeDate(t *testing.T) {
	if err := validateTradeDate(time.Now().Format("2006-01-02")); err != nil {
		t.Fatalf("expected today's date to be valid: %v", err)
	}
	if err := validateTradeDate("not-a-date"); err == nil {
		t.Fatal("expected malformed date to be invalid")
	}
	future := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	if err := validateTradeDate(future); err == nil {
		t.Fatal("expected future date to be invalid")
	}
}

func TestValidators(t *testing.T) {
	if err := validateRequired("field")(" value "); err != nil {
		t.Fatalf("expected non-empty value to pass: %v", err)
	}
	if err := validateRequired("field")(" "); err == nil {
		t.Fatal("expected empty value to fail")
	}
	if err := validatePositiveIntString("count")("10"); err != nil {
		t.Fatalf("expected positive int to pass: %v", err)
	}
	if err := validatePositiveIntString("count")("0"); err == nil {
		t.Fatal("expected zero to fail")
	}
	if _, err := parsePositiveInt("abc", "count"); err == nil {
		t.Fatal("expected non-number to fail")
	}
}

func TestProviderOptionsIncludesMock(t *testing.T) {
	options := providerOptions()
	for _, option := range options {
		if option.Value == providerMock {
			return
		}
	}
	t.Fatal("expected provider options to include mock")
}
