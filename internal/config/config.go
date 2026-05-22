package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config represents the application's runtime configuration parameters.
type Config struct {
	ProjectDir             string
	ResultsDir             string
	DataCacheDir           string
	MemoryLogPath          string
	MemoryLogMaxEntries    int
	LLMProvider            string // openai, gemini, anthropic
	DeepThinkLLM           string
	QuickThinkLLM          string
	BackendURL             string
	GoogleThinkingLevel    string
	OpenAIReasoningEffort  string
	AnthropicEffort        string
	CheckpointEnabled      bool
	OutputLanguage         string
	MaxDebateRounds        int
	MaxRiskDiscussRounds   int
	NewsArticleLimit       int
	GlobalNewsArticleLimit int
	GlobalNewsLookbackDays int
	BenchmarkTicker        string
	ExecutionTimeout       int
}

// GetDefaultHome returns the standard home folder (~/.tradingagents) for local logs/cache.
func GetDefaultHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".tradingagents")
}

// LoadConfig creates a default configuration and overrides keys with env-vars.
func LoadConfig() *Config {
	home := GetDefaultHome()

	// 1. Establish base defaults
	cfg := &Config{
		ProjectDir:             ".",
		ResultsDir:             filepath.Join(home, "logs"),
		DataCacheDir:           filepath.Join(home, "cache"),
		MemoryLogPath:          filepath.Join(home, "memory", "trading_memory.md"),
		MemoryLogMaxEntries:    0, // 0 means unlimited
		LLMProvider:            "openai",
		DeepThinkLLM:           "gpt-4o", // Upgraded to standard models for 2026 stability
		QuickThinkLLM:          "gpt-4o-mini",
		BackendURL:             "",
		GoogleThinkingLevel:    "",
		OpenAIReasoningEffort:  "",
		AnthropicEffort:        "",
		CheckpointEnabled:      false,
		OutputLanguage:         "English",
		MaxDebateRounds:        1,
		MaxRiskDiscussRounds:   1,
		NewsArticleLimit:       20,
		GlobalNewsArticleLimit: 10,
		GlobalNewsLookbackDays: 7,
		BenchmarkTicker:        "SPY",
		ExecutionTimeout:       300,
	}

	// 2. Resolve specific overrides from the environment variables
	if v := os.Getenv("TRADINGAGENTS_RESULTS_DIR"); v != "" {
		cfg.ResultsDir = v
	}
	if v := os.Getenv("TRADINGAGENTS_CACHE_DIR"); v != "" {
		cfg.DataCacheDir = v
	}
	if v := os.Getenv("TRADINGAGENTS_MEMORY_LOG_PATH"); v != "" {
		cfg.MemoryLogPath = v
	}
	if v := os.Getenv("TRADINGAGENTS_LLM_PROVIDER"); v != "" {
		cfg.LLMProvider = strings.ToLower(v)
	}
	if v := os.Getenv("TRADINGAGENTS_DEEP_THINK_LLM"); v != "" {
		cfg.DeepThinkLLM = v
	}
	if v := os.Getenv("TRADINGAGENTS_QUICK_THINK_LLM"); v != "" {
		cfg.QuickThinkLLM = v
	}
	if v := os.Getenv("TRADINGAGENTS_LLM_BACKEND_URL"); v != "" {
		cfg.BackendURL = v
	}
	if v := os.Getenv("TRADINGAGENTS_OUTPUT_LANGUAGE"); v != "" {
		cfg.OutputLanguage = v
	}
	if v := os.Getenv("TRADINGAGENTS_BENCHMARK_TICKER"); v != "" {
		cfg.BenchmarkTicker = v
	}

	// Integer variables
	if v := os.Getenv("TRADINGAGENTS_MAX_DEBATE_ROUNDS"); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			cfg.MaxDebateRounds = val
		}
	}
	if v := os.Getenv("TRADINGAGENTS_MAX_RISK_ROUNDS"); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			cfg.MaxRiskDiscussRounds = val
		}
	}
	if v := os.Getenv("TRADINGAGENTS_EXECUTION_TIMEOUT"); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			cfg.ExecutionTimeout = val
		}
	}

	// Boolean variables
	if v := os.Getenv("TRADINGAGENTS_CHECKPOINT_ENABLED"); v != "" {
		cfg.CheckpointEnabled = strings.ToLower(v) == "true" || v == "1" || strings.ToLower(v) == "yes"
	}

	return cfg
}
