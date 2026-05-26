package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// MiniMaxAdapter implements the LLMProvider interface for MiniMax models.
type MiniMaxAdapter struct {
	compat *OpenAIAdapter
}

// MiniMaxRoundTripper intercepts the request to inject reasoning_split: true for MiniMax reasoning models.
type MiniMaxRoundTripper struct {
	Transport http.RoundTripper
	Model     string
}

// RoundTrip implements the http.RoundTripper interface.
func (m *MiniMaxRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return m.Transport.RoundTrip(req)
	}

	modelLower := strings.ToLower(m.Model)
	// Apply reasoning_split only to M2.x reasoning models as they support this parameter.
	if strings.Contains(modelLower, "minimax-m2") {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &payload); err == nil {
			payload["reasoning_split"] = true
			newBody, err := json.Marshal(payload)
			if err == nil {
				req.Body = io.NopCloser(bytes.NewReader(newBody))
				req.ContentLength = int64(len(newBody))
				req.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))
			}
		}
	}

	return m.Transport.RoundTrip(req)
}

// NewMiniMaxAdapter creates a new MiniMax adapter.
func NewMiniMaxAdapter(apiKey, baseURL, model string, debugDir string) *MiniMaxAdapter {
	baseTransport := http.RoundTripper(http.DefaultTransport)
	if debugDir != "" {
		if dt, err := NewDebugLoggingRoundTripper(http.DefaultTransport, debugDir); err == nil {
			baseTransport = dt
		}
	}

	// Inject the MiniMaxRoundTripper middleware
	transport := &MiniMaxRoundTripper{
		Transport: baseTransport,
		Model:     model,
	}

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(&http.Client{Transport: transport}),
	}

	client := openai.NewClient(opts...)
	compat := &OpenAIAdapter{client: client, model: model}

	return &MiniMaxAdapter{compat: compat}
}

// Generate requests a standard natural-language response.
func (a *MiniMaxAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	return a.compat.Generate(ctx, req)
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface.
func (a *MiniMaxAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	return a.compat.GenerateStructured(ctx, req, target)
}
