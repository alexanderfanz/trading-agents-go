package provider

import (
	"context"
)

// ThinkingConfig captures parameters for reasoning-focused models.
type ThinkingConfig struct {
	// Token budget allocated to reasoning (supported by Gemini & Anthropic)
	BudgetTokens int
	// Effort level, e.g., "low", "medium", "high" (supported by OpenAI o-series)
	EffortLevel string
}

// LLMRequest encapsulates the input params for standard generation or structured outputs.
type LLMRequest struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float64
	MaxTokens    int
	Thinking     *ThinkingConfig
	// Optional Raw JSON schema used to enforce structured JSON output format
	JSONSchema   string
}

// LLMProvider defines the unified interface across OpenAI, Gemini, and Anthropic.
type LLMProvider interface {
	// Generate requests a standard natural-language response.
	Generate(ctx context.Context, req LLMRequest) (string, error)

	// GenerateStructured unmarshals a structured JSON response directly into the target interface structure.
	GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error
}
