package app

import (
	"testing"

	"trading-agents-go/internal/cli"
	"trading-agents-go/internal/config"
)

func TestInitializeLLMProviderUsesConfiguredMock(t *testing.T) {
	clearProviderEnv(t)

	cfg := config.LoadConfig()
	cfg.LLMProvider = "mock"
	cfg.DeepThinkLLM = "mock-model"

	got, err := initializeLLMProvider(cfg, "AAPL", testCLIController(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected mock provider")
	}
	if cfg.LLMProvider != "mock" {
		t.Fatalf("expected configured provider to stay mock, got %s", cfg.LLMProvider)
	}
}

func TestInitializeLLMProviderFallsBackToMockWithoutKeys(t *testing.T) {
	clearProviderEnv(t)

	cfg := config.LoadConfig()
	cfg.LLMProvider = "unsupported"

	got, err := initializeLLMProvider(cfg, "MSFT", testCLIController(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected fallback provider")
	}
	if cfg.LLMProvider != "mock" {
		t.Fatalf("expected fallback provider mock, got %s", cfg.LLMProvider)
	}
}

func TestInitializeLLMProviderAutoDetectsAPIKeys(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		want   string
		model  string
	}{
		{name: "openai", envVar: "OPENAI_API_KEY", want: "openai", model: "gpt-4o"},
		{name: "gemini", envVar: "GEMINI_API_KEY", want: "gemini", model: "gemini-3.5-flash"},
		{name: "anthropic", envVar: "ANTHROPIC_API_KEY", want: "anthropic", model: "claude-3-7-sonnet"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearProviderEnv(t)
			t.Setenv(tc.envVar, "test-key")

			cfg := config.LoadConfig()
			cfg.LLMProvider = "unsupported"
			cfg.DeepThinkLLM = tc.model

			got, err := initializeLLMProvider(cfg, "AAPL", testCLIController(false))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected auto-detected provider")
			}
			if cfg.LLMProvider != tc.want {
				t.Fatalf("expected provider %s, got %s", tc.want, cfg.LLMProvider)
			}
		})
	}
}

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"OPENAI_API_KEY", "GEMINI_API_KEY", "ANTHROPIC_API_KEY"} {
		t.Setenv(key, "")
	}
}

func testCLIController(isTTY bool) *cli.CLIController {
	return &cli.CLIController{
		Theme: cli.NewObsidianTheme(),
		IsTTY: isTTY,
	}
}
