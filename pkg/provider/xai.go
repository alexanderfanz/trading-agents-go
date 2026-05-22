package provider

import (
	"context"
)

// XAIAdapter implements the LLMProvider interface for xAI/Grok models.
type XAIAdapter struct {
	compat *OpenAIAdapter
}

// NewXAIAdapter creates a new xAI adapter.
func NewXAIAdapter(apiKey, baseURL, model string, debugDir string) *XAIAdapter {
	compat := NewOpenAICompatibleAdapter(apiKey, baseURL, model, debugDir)
	return &XAIAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *XAIAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *XAIAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
