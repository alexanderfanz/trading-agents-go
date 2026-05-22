package provider

import (
	"context"
)

// DeepSeekAdapter implements the LLMProvider interface for DeepSeek models.
type DeepSeekAdapter struct {
	compat *OpenAIAdapter
}

// NewDeepSeekAdapter creates a new DeepSeek adapter.
func NewDeepSeekAdapter(apiKey, baseURL, model string, debugDir string) *DeepSeekAdapter {
	compat := NewOpenAICompatibleAdapter(apiKey, baseURL, model, debugDir)
	return &DeepSeekAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *DeepSeekAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *DeepSeekAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
