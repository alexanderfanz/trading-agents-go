package provider

import (
	"context"
)

// QwenAdapter implements the LLMProvider interface for Qwen/DashScope models.
type QwenAdapter struct {
	compat *OpenAIAdapter
}

// NewQwenAdapter creates a new Qwen adapter.
func NewQwenAdapter(apiKey, baseURL, model string, debugDir string) *QwenAdapter {
	compat := NewOpenAICompatibleAdapter(apiKey, baseURL, model, debugDir)
	return &QwenAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *QwenAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *QwenAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
