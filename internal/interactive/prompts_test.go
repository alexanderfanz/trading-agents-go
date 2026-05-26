package interactive

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/huh"

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
	cfg.MaxDebateRounds = 2
	cfg.MaxRiskDiscussRounds = 2
	defaults := app.RunOptions{
		Ticker:    "spy",
		TradeDate: "2026-01-02",
		DBPath:    filepath.Join("tmp", "checkpoints.db"),
	}

	values := newFormValues(cfg, defaults)

	if values.initialProvider != values.provider {
		t.Fatalf("expected initial provider to match provider, got %s/%s", values.initialProvider, values.provider)
	}
	if values.ticker != "SPY" {
		t.Fatalf("expected ticker SPY, got %s", values.ticker)
	}
	if values.tradeDate != "2026-01-02" {
		t.Fatalf("expected trade date from defaults, got %s", values.tradeDate)
	}
	if values.dbPath != filepath.Join("tmp", "checkpoints.db") {
		t.Fatalf("expected db path from defaults, got %s", values.dbPath)
	}
	if values.researchDepth != 3 {
		t.Fatalf("expected research depth to map to selectable option 3, got %d", values.researchDepth)
	}
}

func TestApplyModelDefaultsForProvider(t *testing.T) {
	tests := []struct {
		provider string
		quick    string
		deep     string
	}{
		{provider: "openai", quick: "gpt-5.4-nano", deep: "gpt-5.5"},
		{provider: "gemini", quick: "gemini-3.5-flash", deep: "gemini-3.1-pro-preview"},
		{provider: "google", quick: "gemini-3.5-flash", deep: "gemini-3.1-pro-preview"},
		{provider: "anthropic", quick: "claude-sonnet-4-6", deep: "claude-opus-4-7"},
		{provider: "azure", quick: "gpt-5.4-nano", deep: "gpt-5.5"},
		{provider: "xai", quick: "grok-4.1-fast", deep: "grok-4.20"},
		{provider: "deepseek", quick: "deepseek-v4-flash", deep: "deepseek-v4-pro"},
		{provider: "qwen", quick: "qwen3.6-flash", deep: "qwen3.7-max"},
		{provider: "qwen-cn", quick: "qwen3.6-flash", deep: "qwen3.7-max"},
		{provider: "glm", quick: "glm-5.1-highspeed", deep: "glm-5.1"},
		{provider: "glm-cn", quick: "glm-5.1-highspeed", deep: "glm-5.1"},
		{provider: "minimax", quick: "MiniMax-M2.7-highspeed", deep: "MiniMax-M2.7"},
		{provider: "minimax-cn", quick: "MiniMax-M2.7-highspeed", deep: "MiniMax-M2.7"},
		{provider: "openrouter", quick: "google/gemini-3.5-flash", deep: "anthropic/claude-opus-4.7"},
		{provider: "ollama", quick: "qwen3:4b", deep: "qwen3:32b"},
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

func TestDefaultModelsForProviderHandlesUnknownProvider(t *testing.T) {
	defaults := defaultModelsForProvider("non-existent-provider")
	if defaults.quick != "" || defaults.deep != "" {
		t.Fatalf("expected unknown provider to return empty defaults, got %s/%s", defaults.quick, defaults.deep)
	}
}

func TestApplyModelDefaultsForProviderPreservesConfiguredModelsWhenProviderUnchanged(t *testing.T) {
	values := &formValues{
		initialProvider: "openai",
		provider:        "openai",
		quickThinkLLM:   "custom-quick",
		deepThinkLLM:    "custom-deep",
	}

	values.applyModelDefaultsForProvider()

	if values.quickThinkLLM != "custom-quick" || values.deepThinkLLM != "custom-deep" {
		t.Fatalf("expected unchanged provider to preserve configured models, got %s/%s", values.quickThinkLLM, values.deepThinkLLM)
	}
}

func TestApplyModelDefaultsForProviderUpdatesModelsWhenProviderChanges(t *testing.T) {
	values := &formValues{
		initialProvider: "openai",
		provider:        "gemini",
		quickThinkLLM:   "custom-quick",
		deepThinkLLM:    "custom-deep",
	}

	values.applyModelDefaultsForProvider()

	if values.quickThinkLLM != "gemini-3.5-flash" || values.deepThinkLLM != "gemini-3.1-pro-preview" {
		t.Fatalf("expected changed provider to use gemini defaults, got %s/%s", values.quickThinkLLM, values.deepThinkLLM)
	}
}

func TestModelOptionsForProvider(t *testing.T) {
	for _, providerOption := range providerOptions() {
		quickOptions := modelOptionsForProvider(providerOption.Value, modelCategoryQuick)
		deepOptions := modelOptionsForProvider(providerOption.Value, modelCategoryDeep)

		if providerOption.Value == providerMock {
			if len(quickOptions) != 1 || len(deepOptions) != 1 {
				t.Fatalf("expected mock provider to have one quick and deep model, got %d/%d", len(quickOptions), len(deepOptions))
			}
			continue
		}

		if len(quickOptions) < 3 || len(quickOptions) > 6 {
			t.Fatalf("expected %s quick options to have 3-6 models, got %d", providerOption.Value, len(quickOptions))
		}
		if len(deepOptions) < 3 || len(deepOptions) > 6 {
			t.Fatalf("expected %s deep options to have 3-6 models, got %d", providerOption.Value, len(deepOptions))
		}

		defaults := defaultModelsForProvider(providerOption.Value)
		if quickOptions[0].Value != defaults.quick {
			t.Fatalf("expected %s first quick option to match default %s, got %s", providerOption.Value, defaults.quick, quickOptions[0].Value)
		}
		if deepOptions[0].Value != defaults.deep {
			t.Fatalf("expected %s first deep option to match default %s, got %s", providerOption.Value, defaults.deep, deepOptions[0].Value)
		}
	}
}

func TestModelOptionsForProviderWithCurrentIncludesCustomModel(t *testing.T) {
	current := "custom-gemini-model"
	options := modelOptionsForProviderWithCurrent(providerGemini, modelCategoryQuick, current)

	if len(options) == 0 {
		t.Fatal("expected options for curated provider")
	}
	if options[0].Value != current {
		t.Fatalf("expected current model to be first option, got %s", options[0].Value)
	}
	if options[0].Key != current+" (current)" {
		t.Fatalf("expected current model label, got %s", options[0].Key)
	}
}

func TestModelOptionsForProviderWithCurrentDoesNotDuplicateCuratedModel(t *testing.T) {
	defaults := defaultModelsForProvider(providerGemini)
	options := modelOptionsForProviderWithCurrent(providerGemini, modelCategoryQuick, defaults.quick)
	baseOptions := modelOptionsForProvider(providerGemini, modelCategoryQuick)

	if len(options) != len(baseOptions) {
		t.Fatalf("expected curated current model not to be duplicated, got %d options instead of %d", len(options), len(baseOptions))
	}
	if options[0].Value != defaults.quick {
		t.Fatalf("expected first option to remain provider default %s, got %s", defaults.quick, options[0].Value)
	}
}

func TestModelFieldForProviderUsesSelectForCuratedModels(t *testing.T) {
	value := "old-model"
	field := modelFieldForProvider(providerGemini, modelCategoryQuick, &value)

	if _, ok := field.(*huh.Select[string]); !ok {
		t.Fatalf("expected curated provider to use select field, got %T", field)
	}
}

func TestModelFieldForProviderUsesInputForCustomModels(t *testing.T) {
	value := "custom-model"
	field := modelFieldForProvider("custom-provider", modelCategoryDeep, &value)

	if _, ok := field.(*huh.Input); !ok {
		t.Fatalf("expected custom provider to use input field, got %T", field)
	}
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

func TestValidateLocalReportsDir(t *testing.T) {
	values := &formValues{createLocalReports: false}
	if err := values.validateLocalReportsDir(" "); err != nil {
		t.Fatalf("expected empty local reports dir to pass when reports disabled: %v", err)
	}

	values.createLocalReports = true
	if err := values.validateLocalReportsDir("reports"); err != nil {
		t.Fatalf("expected local reports dir to pass when reports enabled: %v", err)
	}
	if err := values.validateLocalReportsDir(" "); err == nil {
		t.Fatal("expected empty local reports dir to fail when reports enabled")
	}
}

func TestSelectableResearchDepth(t *testing.T) {
	tests := []struct {
		depth int
		want  int
	}{
		{depth: 0, want: 1},
		{depth: 1, want: 1},
		{depth: 2, want: 3},
		{depth: 3, want: 3},
		{depth: 4, want: 5},
		{depth: 5, want: 5},
		{depth: 10, want: 5},
	}

	for _, tc := range tests {
		if got := selectableResearchDepth(tc.depth); got != tc.want {
			t.Fatalf("expected depth %d to map to %d, got %d", tc.depth, tc.want, got)
		}
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
