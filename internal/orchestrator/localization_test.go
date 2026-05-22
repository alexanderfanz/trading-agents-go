package orchestrator

import (
	"context"
	"testing"
	"trading-agents-go/internal/config"
	"trading-agents-go/pkg/provider"
)

// MockLLMProvider is a lightweight mock implementing provider.LLMProvider.
type MockLLMProvider struct {
	LastSystemPrompt string
	LastUserPrompt   string
}

func (m *MockLLMProvider) Generate(ctx context.Context, req provider.LLMRequest) (string, error) {
	m.LastSystemPrompt = req.SystemPrompt
	m.LastUserPrompt = req.UserPrompt
	return "Mocked natural language response", nil
}

func (m *MockLLMProvider) GenerateStructured(ctx context.Context, req provider.LLMRequest, target interface{}) error {
	m.LastSystemPrompt = req.SystemPrompt
	m.LastUserPrompt = req.UserPrompt
	return nil
}

func TestGetLanguageInstruction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "English lowercase",
			input:    "english",
			expected: "",
		},
		{
			name:     "English uppercase",
			input:    "ENGLISH",
			expected: "",
		},
		{
			name:     "English with spaces",
			input:    "  English  ",
			expected: "",
		},
		{
			name:     "Empty language",
			input:    "",
			expected: "",
		},
		{
			name:     "Spanish standard",
			input:    "Spanish",
			expected: " Write your entire response in Spanish.",
		},
		{
			name:     "Chinese standard",
			input:    "Chinese",
			expected: " Write your entire response in Chinese.",
		},
		{
			name:     "German with spaces",
			input:    "  German  ",
			expected: " Write your entire response in German.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getLanguageInstruction(tt.input)
			if got != tt.expected {
				t.Errorf("getLanguageInstruction(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCreateAgentLocalizationInjection(t *testing.T) {
	mockLLM := &MockLLMProvider{}
	cfg := &config.Config{
		OutputLanguage: "French",
	}

	orch := &TradingOrchestrator{
		cfg:         cfg,
		llmProvider: mockLLM,
	}

	baseInstruction := "You are a professional Market Analyst."
	agent := orch.createAgent("Market Analyst", "Market Analyst", baseInstruction)

	expectedInstruction := "You are a professional Market Analyst. Write your entire response in French."
	if agent.SystemInstruction != expectedInstruction {
		t.Errorf("expected SystemInstruction to be %q, got %q", expectedInstruction, agent.SystemInstruction)
	}

	// Verify propagation to generation parameters
	ctx := context.Background()
	_, err := agent.Call(ctx, "Analyze AAPL")
	if err != nil {
		t.Fatalf("unexpected error calling agent: %v", err)
	}

	if mockLLM.LastSystemPrompt != expectedInstruction {
		t.Errorf("expected generated system prompt to be %q, got %q", expectedInstruction, mockLLM.LastSystemPrompt)
	}

	// Verify that English output language results in no localization suffix
	orch.cfg.OutputLanguage = "English"
	agentEnglish := orch.createAgent("Market Analyst", "Market Analyst", baseInstruction)
	if agentEnglish.SystemInstruction != baseInstruction {
		t.Errorf("expected SystemInstruction to be unmodified %q for English, got %q", baseInstruction, agentEnglish.SystemInstruction)
	}
}
