package provider

import (
	"context"
)

// OllamaAdapter implements the LLMProvider interface for local/remote Ollama.
type OllamaAdapter struct {
	compat *OpenAIAdapter
}

// NewOllamaAdapter creates a new Ollama adapter. Ollama requires no API key.
func NewOllamaAdapter(baseURL, model string, debugDir string) *OllamaAdapter {
	// Ollama expects a non-empty key structure, but does not validate it locally.
	compat := NewOpenAICompatibleAdapter("ollama", baseURL, model, debugDir)
	return &OllamaAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *OllamaAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *OllamaAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
