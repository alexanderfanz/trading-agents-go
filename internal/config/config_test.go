package config

import (
	"path/filepath"
	"strings"
	"testing"
)

var tradingAgentsEnvVars = []string{
	"TRADINGAGENTS_RESULTS_DIR",
	"TRADINGAGENTS_CACHE_DIR",
	"TRADINGAGENTS_MEMORY_LOG_PATH",
	"TRADINGAGENTS_LLM_PROVIDER",
	"TRADINGAGENTS_DEEP_THINK_LLM",
	"TRADINGAGENTS_QUICK_THINK_LLM",
	"TRADINGAGENTS_LLM_BACKEND_URL",
	"TRADINGAGENTS_OUTPUT_LANGUAGE",
	"TRADINGAGENTS_BENCHMARK_TICKER",
	"TRADINGAGENTS_MAX_DEBATE_ROUNDS",
	"TRADINGAGENTS_MAX_RISK_ROUNDS",
	"TRADINGAGENTS_EXECUTION_TIMEOUT",
	"TRADINGAGENTS_CHECKPOINT_ENABLED",
	"TRADINGAGENTS_CREATE_LOCAL_REPORTS",
	"TRADINGAGENTS_LOCAL_REPORTS_DIR",
}

func clearTradingAgentsEnv(t *testing.T) {
	t.Helper()
	for _, key := range tradingAgentsEnvVars {
		t.Setenv(key, "")
	}
}

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestGetDefaultHome(t *testing.T) {
	home := setTestHome(t)

	got := GetDefaultHome()
	want := filepath.Join(home, ".tradingagentsgo")

	if got != want {
		t.Errorf("GetDefaultHome() = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, ".tradingagentsgo") {
		t.Errorf("GetDefaultHome() = %q, expected suffix .tradingagentsgo", got)
	}
}


func TestGetDefaultHomeFallback(t *testing.T) {
	t.Setenv("HOME", "")

	got := GetDefaultHome()
	want := filepath.Join(".", ".tradingagentsgo")

	if got != want {
		t.Errorf("GetDefaultHome() = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, ".tradingagentsgo") {
		t.Errorf("GetDefaultHome() = %q, expected suffix .tradingagentsgo", got)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	clearTradingAgentsEnv(t)
	home := setTestHome(t)
	defaultHome := filepath.Join(home, ".tradingagentsgo")

	cfg := LoadConfig()

	if cfg.ProjectDir != "." {
		t.Errorf("ProjectDir = %q, want %q", cfg.ProjectDir, ".")
	}
	if cfg.ResultsDir != filepath.Join(defaultHome, "logs") {
		t.Errorf("ResultsDir = %q, want %q", cfg.ResultsDir, filepath.Join(defaultHome, "logs"))
	}
	if cfg.DataCacheDir != filepath.Join(defaultHome, "cache") {
		t.Errorf("DataCacheDir = %q, want %q", cfg.DataCacheDir, filepath.Join(defaultHome, "cache"))
	}
	if cfg.MemoryLogPath != filepath.Join(defaultHome, "memory", "trading_memory.md") {
		t.Errorf("MemoryLogPath = %q, want %q", cfg.MemoryLogPath, filepath.Join(defaultHome, "memory", "trading_memory.md"))
	}
	if cfg.MemoryLogMaxEntries != 0 {
		t.Errorf("MemoryLogMaxEntries = %d, want 0", cfg.MemoryLogMaxEntries)
	}
	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "openai")
	}
	if cfg.DeepThinkLLM != "gpt-4o" {
		t.Errorf("DeepThinkLLM = %q, want %q", cfg.DeepThinkLLM, "gpt-4o")
	}
	if cfg.QuickThinkLLM != "gpt-4o-mini" {
		t.Errorf("QuickThinkLLM = %q, want %q", cfg.QuickThinkLLM, "gpt-4o-mini")
	}
	if cfg.BackendURL != "" {
		t.Errorf("BackendURL = %q, want empty", cfg.BackendURL)
	}
	if cfg.GoogleThinkingLevel != "" {
		t.Errorf("GoogleThinkingLevel = %q, want empty", cfg.GoogleThinkingLevel)
	}
	if cfg.OpenAIReasoningEffort != "" {
		t.Errorf("OpenAIReasoningEffort = %q, want empty", cfg.OpenAIReasoningEffort)
	}
	if cfg.AnthropicEffort != "" {
		t.Errorf("AnthropicEffort = %q, want empty", cfg.AnthropicEffort)
	}
	if cfg.CheckpointEnabled {
		t.Error("CheckpointEnabled = true, want false")
	}
	if cfg.OutputLanguage != "English" {
		t.Errorf("OutputLanguage = %q, want %q", cfg.OutputLanguage, "English")
	}
	if cfg.MaxDebateRounds != 1 {
		t.Errorf("MaxDebateRounds = %d, want 1", cfg.MaxDebateRounds)
	}
	if cfg.MaxRiskDiscussRounds != 1 {
		t.Errorf("MaxRiskDiscussRounds = %d, want 1", cfg.MaxRiskDiscussRounds)
	}
	if cfg.NewsArticleLimit != 20 {
		t.Errorf("NewsArticleLimit = %d, want 20", cfg.NewsArticleLimit)
	}
	if cfg.GlobalNewsArticleLimit != 10 {
		t.Errorf("GlobalNewsArticleLimit = %d, want 10", cfg.GlobalNewsArticleLimit)
	}
	if cfg.GlobalNewsLookbackDays != 7 {
		t.Errorf("GlobalNewsLookbackDays = %d, want 7", cfg.GlobalNewsLookbackDays)
	}
	if cfg.BenchmarkTicker != "SPY" {
		t.Errorf("BenchmarkTicker = %q, want %q", cfg.BenchmarkTicker, "SPY")
	}
	if cfg.ExecutionTimeout != 300 {
		t.Errorf("ExecutionTimeout = %d, want 300", cfg.ExecutionTimeout)
	}
	if !cfg.CreateLocalReports {
		t.Error("CreateLocalReports = false, want true")
	}
	if cfg.LocalReportsDir != "reports" {
		t.Errorf("LocalReportsDir = %q, want %q", cfg.LocalReportsDir, "reports")
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	clearTradingAgentsEnv(t)
	setTestHome(t)

	tests := []struct {
		name   string
		envVar string
		value  string
		check  func(t *testing.T, cfg *Config)
	}{
		{
			name:   "TRADINGAGENTS_RESULTS_DIR",
			envVar: "TRADINGAGENTS_RESULTS_DIR",
			value:  "/custom/results",
			check: func(t *testing.T, cfg *Config) {
				if cfg.ResultsDir != "/custom/results" {
					t.Errorf("ResultsDir = %q, want %q", cfg.ResultsDir, "/custom/results")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_CACHE_DIR",
			envVar: "TRADINGAGENTS_CACHE_DIR",
			value:  "/custom/cache",
			check: func(t *testing.T, cfg *Config) {
				if cfg.DataCacheDir != "/custom/cache" {
					t.Errorf("DataCacheDir = %q, want %q", cfg.DataCacheDir, "/custom/cache")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_MEMORY_LOG_PATH",
			envVar: "TRADINGAGENTS_MEMORY_LOG_PATH",
			value:  "/custom/memory.md",
			check: func(t *testing.T, cfg *Config) {
				if cfg.MemoryLogPath != "/custom/memory.md" {
					t.Errorf("MemoryLogPath = %q, want %q", cfg.MemoryLogPath, "/custom/memory.md")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_LLM_PROVIDER lowercase",
			envVar: "TRADINGAGENTS_LLM_PROVIDER",
			value:  "Anthropic",
			check: func(t *testing.T, cfg *Config) {
				if cfg.LLMProvider != "anthropic" {
					t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "anthropic")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_DEEP_THINK_LLM",
			envVar: "TRADINGAGENTS_DEEP_THINK_LLM",
			value:  "claude-opus-4",
			check: func(t *testing.T, cfg *Config) {
				if cfg.DeepThinkLLM != "claude-opus-4" {
					t.Errorf("DeepThinkLLM = %q, want %q", cfg.DeepThinkLLM, "claude-opus-4")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_QUICK_THINK_LLM",
			envVar: "TRADINGAGENTS_QUICK_THINK_LLM",
			value:  "claude-haiku",
			check: func(t *testing.T, cfg *Config) {
				if cfg.QuickThinkLLM != "claude-haiku" {
					t.Errorf("QuickThinkLLM = %q, want %q", cfg.QuickThinkLLM, "claude-haiku")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_LLM_BACKEND_URL",
			envVar: "TRADINGAGENTS_LLM_BACKEND_URL",
			value:  "http://localhost:8080",
			check: func(t *testing.T, cfg *Config) {
				if cfg.BackendURL != "http://localhost:8080" {
					t.Errorf("BackendURL = %q, want %q", cfg.BackendURL, "http://localhost:8080")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_OUTPUT_LANGUAGE",
			envVar: "TRADINGAGENTS_OUTPUT_LANGUAGE",
			value:  "Spanish",
			check: func(t *testing.T, cfg *Config) {
				if cfg.OutputLanguage != "Spanish" {
					t.Errorf("OutputLanguage = %q, want %q", cfg.OutputLanguage, "Spanish")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_BENCHMARK_TICKER",
			envVar: "TRADINGAGENTS_BENCHMARK_TICKER",
			value:  "QQQ",
			check: func(t *testing.T, cfg *Config) {
				if cfg.BenchmarkTicker != "QQQ" {
					t.Errorf("BenchmarkTicker = %q, want %q", cfg.BenchmarkTicker, "QQQ")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_MAX_DEBATE_ROUNDS",
			envVar: "TRADINGAGENTS_MAX_DEBATE_ROUNDS",
			value:  "5",
			check: func(t *testing.T, cfg *Config) {
				if cfg.MaxDebateRounds != 5 {
					t.Errorf("MaxDebateRounds = %d, want 5", cfg.MaxDebateRounds)
				}
			},
		},
		{
			name:   "TRADINGAGENTS_MAX_RISK_ROUNDS",
			envVar: "TRADINGAGENTS_MAX_RISK_ROUNDS",
			value:  "3",
			check: func(t *testing.T, cfg *Config) {
				if cfg.MaxRiskDiscussRounds != 3 {
					t.Errorf("MaxRiskDiscussRounds = %d, want 3", cfg.MaxRiskDiscussRounds)
				}
			},
		},
		{
			name:   "TRADINGAGENTS_EXECUTION_TIMEOUT",
			envVar: "TRADINGAGENTS_EXECUTION_TIMEOUT",
			value:  "600",
			check: func(t *testing.T, cfg *Config) {
				if cfg.ExecutionTimeout != 600 {
					t.Errorf("ExecutionTimeout = %d, want 600", cfg.ExecutionTimeout)
				}
			},
		},
		{
			name:   "TRADINGAGENTS_CHECKPOINT_ENABLED true",
			envVar: "TRADINGAGENTS_CHECKPOINT_ENABLED",
			value:  "true",
			check: func(t *testing.T, cfg *Config) {
				if !cfg.CheckpointEnabled {
					t.Error("CheckpointEnabled = false, want true")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_CREATE_LOCAL_REPORTS false",
			envVar: "TRADINGAGENTS_CREATE_LOCAL_REPORTS",
			value:  "false",
			check: func(t *testing.T, cfg *Config) {
				if cfg.CreateLocalReports {
					t.Error("CreateLocalReports = true, want false")
				}
			},
		},
		{
			name:   "TRADINGAGENTS_LOCAL_REPORTS_DIR",
			envVar: "TRADINGAGENTS_LOCAL_REPORTS_DIR",
			value:  "custom-reports",
			check: func(t *testing.T, cfg *Config) {
				if cfg.LocalReportsDir != "custom-reports" {
					t.Errorf("LocalReportsDir = %q, want %q", cfg.LocalReportsDir, "custom-reports")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearTradingAgentsEnv(t)
			setTestHome(t)
			t.Setenv(tt.envVar, tt.value)

			cfg := LoadConfig()
			tt.check(t, cfg)
		})
	}
}

func TestLoadConfigInvalidIntegerFallback(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		value  string
		field  string
		want   int
	}{
		{
			name:   "invalid TRADINGAGENTS_MAX_DEBATE_ROUNDS",
			envVar: "TRADINGAGENTS_MAX_DEBATE_ROUNDS",
			value:  "not-a-number",
			field:  "MaxDebateRounds",
			want:   1,
		},
		{
			name:   "invalid TRADINGAGENTS_MAX_RISK_ROUNDS",
			envVar: "TRADINGAGENTS_MAX_RISK_ROUNDS",
			value:  "abc",
			field:  "MaxRiskDiscussRounds",
			want:   1,
		},
		{
			name:   "invalid TRADINGAGENTS_EXECUTION_TIMEOUT",
			envVar: "TRADINGAGENTS_EXECUTION_TIMEOUT",
			value:  "timeout",
			field:  "ExecutionTimeout",
			want:   300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearTradingAgentsEnv(t)
			setTestHome(t)
			t.Setenv(tt.envVar, tt.value)

			cfg := LoadConfig()

			var got int
			switch tt.field {
			case "MaxDebateRounds":
				got = cfg.MaxDebateRounds
			case "MaxRiskDiscussRounds":
				got = cfg.MaxRiskDiscussRounds
			case "ExecutionTimeout":
				got = cfg.ExecutionTimeout
			default:
				t.Fatalf("unknown field %q", tt.field)
			}

			if got != tt.want {
				t.Errorf("%s = %d, want default %d", tt.field, got, tt.want)
			}
		})
	}
}

func TestLoadConfigBooleanParsing(t *testing.T) {
	trueValues := []string{"true", "TRUE", "1", "yes", "Yes"}
	falseValues := []string{"false", "FALSE", "0", "no", "anything"}

	t.Run("TRADINGAGENTS_CHECKPOINT_ENABLED", func(t *testing.T) {
		for _, value := range trueValues {
			t.Run("true_"+value, func(t *testing.T) {
				clearTradingAgentsEnv(t)
				setTestHome(t)
				t.Setenv("TRADINGAGENTS_CHECKPOINT_ENABLED", value)

				cfg := LoadConfig()
				if !cfg.CheckpointEnabled {
					t.Errorf("CheckpointEnabled = false for value %q, want true", value)
				}
			})
		}

		for _, value := range falseValues {
			t.Run("false_"+value, func(t *testing.T) {
				clearTradingAgentsEnv(t)
				setTestHome(t)
				t.Setenv("TRADINGAGENTS_CHECKPOINT_ENABLED", value)

				cfg := LoadConfig()
				if cfg.CheckpointEnabled {
					t.Errorf("CheckpointEnabled = true for value %q, want false", value)
				}
			})
		}

		t.Run("empty keeps default false", func(t *testing.T) {
			clearTradingAgentsEnv(t)
			setTestHome(t)

			cfg := LoadConfig()
			if cfg.CheckpointEnabled {
				t.Error("CheckpointEnabled = true with empty env, want default false")
			}
		})
	})

	t.Run("TRADINGAGENTS_CREATE_LOCAL_REPORTS", func(t *testing.T) {
		for _, value := range trueValues {
			t.Run("true_"+value, func(t *testing.T) {
				clearTradingAgentsEnv(t)
				setTestHome(t)
				t.Setenv("TRADINGAGENTS_CREATE_LOCAL_REPORTS", value)

				cfg := LoadConfig()
				if !cfg.CreateLocalReports {
					t.Errorf("CreateLocalReports = false for value %q, want true", value)
				}
			})
		}

		for _, value := range falseValues {
			t.Run("false_"+value, func(t *testing.T) {
				clearTradingAgentsEnv(t)
				setTestHome(t)
				t.Setenv("TRADINGAGENTS_CREATE_LOCAL_REPORTS", value)

				cfg := LoadConfig()
				if cfg.CreateLocalReports {
					t.Errorf("CreateLocalReports = true for value %q, want false", value)
				}
			})
		}

		t.Run("empty keeps default true", func(t *testing.T) {
			clearTradingAgentsEnv(t)
			setTestHome(t)

			cfg := LoadConfig()
			if !cfg.CreateLocalReports {
				t.Error("CreateLocalReports = false with empty env, want default true")
			}
		})
	})
}

func TestLoadConfigUnknownEnvIgnored(t *testing.T) {
	clearTradingAgentsEnv(t)
	home := setTestHome(t)
	defaultHome := filepath.Join(home, ".tradingagentsgo")

	t.Setenv("TRADINGAGENTS_UNKNOWN_SETTING", "should-be-ignored")
	t.Setenv("TRADINGAGENTS_FOO", "bar")

	cfg := LoadConfig()

	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider = %q, want default openai", cfg.LLMProvider)
	}
	if cfg.ResultsDir != filepath.Join(defaultHome, "logs") {
		t.Errorf("ResultsDir = %q, want default %q", cfg.ResultsDir, filepath.Join(defaultHome, "logs"))
	}
}
