package provider

import (
	"context"
)

// OpenRouterAdapter implements the LLMProvider interface for OpenRouter models.
type OpenRouterAdapter struct {
	compat *OpenAIAdapter
}

// NewOpenRouterAdapter creates a new OpenRouter adapter.
func NewOpenRouterAdapter(apiKey, baseURL, model string, debugDir string) *OpenRouterAdapter {
	compat := NewOpenAICompatibleAdapter(apiKey, baseURL, model, debugDir)
	return &OpenRouterAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *OpenRouterAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *OpenRouterAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
