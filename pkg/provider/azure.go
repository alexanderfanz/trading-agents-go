package provider

import (
	"context"
	"net/http"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// AzureAdapter implements the LLMProvider interface for Azure OpenAI models.
type AzureAdapter struct {
	compat *OpenAIAdapter
}

// NewAzureAdapter creates a new Azure OpenAI adapter.
func NewAzureAdapter(apiKey, baseURL, model string, debugDir string) *AzureAdapter {
	opts := []option.RequestOption{
		option.WithHeader("api-key", apiKey),
		option.WithBaseURL(baseURL),
	}

	if debugDir != "" {
		transport, err := NewDebugLoggingRoundTripper(http.DefaultTransport, debugDir)
		if err == nil {
			opts = append(opts, option.WithHTTPClient(&http.Client{Transport: transport}))
		}
	}

	client := openai.NewClient(opts...)
	compat := &OpenAIAdapter{client: client, model: model}

	return &AzureAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *AzureAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *AzureAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
