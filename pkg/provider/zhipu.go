package provider

import (
	"context"
)

// ZhipuAdapter implements the LLMProvider interface for GLM/Zhipu models.
type ZhipuAdapter struct {
	compat *OpenAIAdapter
}

// NewZhipuAdapter creates a new Zhipu adapter.
func NewZhipuAdapter(apiKey, baseURL, model string, debugDir string) *ZhipuAdapter {
	compat := NewOpenAICompatibleAdapter(apiKey, baseURL, model, debugDir)
	return &ZhipuAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *ZhipuAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *ZhipuAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
