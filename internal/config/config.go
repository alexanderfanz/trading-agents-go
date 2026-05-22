package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type DataVendors struct {
	CoreStockAPIs       string `json:"core_stock_apis"`
	TechnicalIndicators string `json:"technical_indicators"`
	FundamentalData     string `json:"fundamental_data"`
	NewsData            string `json:"news_data"`
}

type Config struct {
	ProjectDir              string            `json:"project_dir"`
	ResultsDir              string            `json:"results_dir"`
	DataCacheDir            string            `json:"data_cache_dir"`
	MemoryLogPath           string            `json:"memory_log_path"`
	MemoryLogMaxEntries     *int              `json:"memory_log_max_entries"`
	LLMProvider             string            `json:"llm_provider"`
	DeepThinkLLM            string            `json:"deep_think_llm"`
	QuickThinkLLM           string            `json:"quick_think_llm"`
	BackendURL              string            `json:"backend_url"`
	GoogleThinkingLevel     string            `json:"google_thinking_level"`
	OpenAIReasoningEffort   string            `json:"openai_reasoning_effort"`
	AnthropicEffort         string            `json:"anthropic_effort"`
	CheckpointEnabled       bool              `json:"checkpoint_enabled"`
	OutputLanguage          string            `json:"output_language"`
	MaxDebateRounds         int               `json:"max_debate_rounds"`
	MaxRiskDiscussRounds    int               `json:"max_risk_discuss_rounds"`
	MaxRecurLimit           int               `json:"max_recur_limit"`
	AnalystConcurrencyLimit int               `json:"analyst_concurrency_limit"`
	NewsArticleLimit        int               `json:"news_article_limit"`
	GlobalNewsArticleLimit  int               `json:"global_news_article_limit"`
	GlobalNewsLookbackDays  int               `json:"global_news_lookback_days"`
	GlobalNewsQueries       []string          `json:"global_news_queries"`
	DataVendors             DataVendors       `json:"data_vendors"`
	ToolVendors             map[string]string `json:"tool_vendors"`
	BenchmarkTicker         string            `json:"benchmark_ticker"`
	BenchmarkMap            map[string]string `json:"benchmark_map"`
}

// getEnvOr returns the env value if present, else a fallback
func getEnvOr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// getEnvBool returns bool value from env
func getEnvBool(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}
	return b
}

// getEnvInt returns int value from env
func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return i
}

// LoadDefaultConfig instantiates the default configuration and applies environmental overrides.
func LoadDefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	taHome := filepath.Join(home, ".tradingagents")

	c := &Config{
		ProjectDir:              ".",
		ResultsDir:              getEnvOr("TRADINGAGENTS_RESULTS_DIR", filepath.Join(taHome, "logs")),
		DataCacheDir:            getEnvOr("TRADINGAGENTS_CACHE_DIR", filepath.Join(taHome, "cache")),
		MemoryLogPath:           getEnvOr("TRADINGAGENTS_MEMORY_LOG_PATH", filepath.Join(taHome, "memory", "trading_memory.md")),
		MemoryLogMaxEntries:     nil,
		LLMProvider:             getEnvOr("TRADINGAGENTS_LLM_PROVIDER", "openai"),
		DeepThinkLLM:            getEnvOr("TRADINGAGENTS_DEEP_THINK_LLM", "gpt-4o"),
		QuickThinkLLM:           getEnvOr("TRADINGAGENTS_QUICK_THINK_LLM", "gpt-4o-mini"),
		BackendURL:              getEnvOr("TRADINGAGENTS_LLM_BACKEND_URL", ""),
		GoogleThinkingLevel:     getEnvOr("TRADINGAGENTS_GOOGLE_THINKING_LEVEL", ""),
		OpenAIReasoningEffort:   getEnvOr("TRADINGAGENTS_OPENAI_REASONING_EFFORT", ""),
		AnthropicEffort:         getEnvOr("TRADINGAGENTS_ANTHROPIC_EFFORT", ""),
		CheckpointEnabled:       getEnvBool("TRADINGAGENTS_CHECKPOINT_ENABLED", false),
		OutputLanguage:          getEnvOr("TRADINGAGENTS_OUTPUT_LANGUAGE", "English"),
		MaxDebateRounds:         getEnvInt("TRADINGAGENTS_MAX_DEBATE_ROUNDS", 1),
		MaxRiskDiscussRounds:    getEnvInt("TRADINGAGENTS_MAX_RISK_ROUNDS", 1),
		MaxRecurLimit:           100,
		AnalystConcurrencyLimit: 1,
		NewsArticleLimit:        20,
		GlobalNewsArticleLimit:  10,
		GlobalNewsLookbackDays:  7,
		GlobalNewsQueries: []string{
			"Federal Reserve interest rates inflation",
			"S&P 500 earnings GDP economic outlook",
			"geopolitical risk trade war sanctions",
			"ECB Bank of England BOJ central bank policy",
			"oil commodities supply chain energy",
		},
		DataVendors: DataVendors{
			CoreStockAPIs:       "yfinance",
			TechnicalIndicators: "yfinance",
			FundamentalData:     "yfinance",
			NewsData:            "yfinance",
		},
		ToolVendors: make(map[string]string),
		BenchmarkTicker: getEnvOr("TRADINGAGENTS_BENCHMARK_TICKER", ""),
		BenchmarkMap: map[string]string{
			".NS": "^NSEI",
			".BO": "^BSESN",
			".T":  "^N225",
			".HK": "^HSI",
			".L":  "^FTSE",
			".TO": "^GSPTSE",
			".AX": "^AXJO",
			"":    "SPY",
		},
	}

	// Apply dynamic overrides from tool specific config if needed
	if maxEntries := os.Getenv("TRADINGAGENTS_MEMORY_LOG_MAX_ENTRIES"); maxEntries != "" {
		if val, err := strconv.Atoi(maxEntries); err == nil {
			c.MemoryLogMaxEntries = &val
		}
	}

	return c
}

// PrintSummary prints a pretty summary of the loaded configuration.
func (c *Config) PrintSummary() string {
	var summary strings.Builder
	summary.WriteString("=========================================\n")
	summary.WriteString(" TRADINGAGENTS CONFIGURATION SUMMARY\n")
	summary.WriteString("=========================================\n")
	summary.WriteString(fmt.Sprintf("LLM Provider:       %s\n", c.LLMProvider))
	summary.WriteString(fmt.Sprintf("Deep Think LLM:     %s\n", c.DeepThinkLLM))
	summary.WriteString(fmt.Sprintf("Quick Think LLM:    %s\n", c.QuickThinkLLM))
	summary.WriteString(fmt.Sprintf("Language Output:    %s\n", c.OutputLanguage))
	summary.WriteString(fmt.Sprintf("Max Debate Rounds:  %d\n", c.MaxDebateRounds))
	summary.WriteString(fmt.Sprintf("Max Risk Rounds:    %d\n", c.MaxRiskDiscussRounds))
	summary.WriteString(fmt.Sprintf("Cache Directory:    %s\n", c.DataCacheDir))
	summary.WriteString(fmt.Sprintf("Results Directory:  %s\n", c.ResultsDir))
	summary.WriteString(fmt.Sprintf("Memory Log Path:    %s\n", c.MemoryLogPath))
	summary.WriteString("=========================================\n")
	return summary.String()
}
