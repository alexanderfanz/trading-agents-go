package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type TestOutput struct {
	Ticker string `json:"ticker"`
	Signal string `json:"signal"`
}

type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.roundTripFunc(req)
}

func TestAdapters_GenerateAndStructured(t *testing.T) {
	// Start a mock HTTP server to handle both OpenAI-style and Anthropic-style APIs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", 500)
			return
		}

		bodyStr := string(bodyBytes)

		// 1. Anthropic message endpoint
		if strings.Contains(r.URL.Path, "messages") {
			if strings.Contains(bodyStr, "tools") {
				// Tool use response for GenerateStructured
				resp := `{
					"content": [
						{
							"type": "tool_use",
							"id": "toolu_123",
							"name": "submit_structured_response",
							"input": {
								"ticker": "TSLA",
								"signal": "BUY"
							}
						}
					]
				}`
				_, _ = w.Write([]byte(resp))
			} else {
				// Normal response for Generate
				resp := `{
					"content": [
						{
							"type": "text",
							"text": "Hello from mock Anthropic!"
						}
					]
				}`
				_, _ = w.Write([]byte(resp))
			}
			return
		}

		// 2. OpenAI chat completions endpoint (and compatible models)
		if strings.Contains(r.URL.Path, "chat/completions") {
			// Check if we are doing structured output (JSONSchema or response_format)
			if strings.Contains(bodyStr, "response_format") {
				resp := `{
					"choices": [{
						"message": {
							"content": "{\"ticker\": \"TSLA\", \"signal\": \"BUY\"}"
						}
					}]
				}`
				_, _ = w.Write([]byte(resp))
			} else {
				resp := `{
					"choices": [{
						"message": {
							"content": "Hello from mock OpenAI!"
						}
					}]
				}`
				_, _ = w.Write([]byte(resp))
			}
			return
		}

		// Fallback error
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error": "not found"}`))
	}))
	defer server.Close()

	// Set required mock API keys in environment so NewLLMProvider constructs properly
	envs := map[string]string{
		"OPENAI_API_KEY":       "mock-openai-key",
		"ANTHROPIC_API_KEY":    "mock-anthropic-key",
		"AZURE_OPENAI_API_KEY": "mock-azure-key",
		"XAI_API_KEY":          "mock-xai-key",
		"DEEPSEEK_API_KEY":     "mock-deepseek-key",
		"DASHSCOPE_API_KEY":    "mock-qwen-key",
		"ZHIPU_API_KEY":        "mock-zhipu-key",
		"MINIMAX_API_KEY":      "mock-minimax-key",
		"OPENROUTER_API_KEY":   "mock-openrouter-key",
	}
	for k, v := range envs {
		_ = os.Setenv(k, v)
		defer func(key string) { _ = os.Unsetenv(key) }(k)
	}

	// 3. Test OpenAIAdapter directly (using custom base URL constructor)
	t.Run("OpenAIAdapter", func(t *testing.T) {
		adapter := NewOpenAICompatibleAdapter("mock-key", server.URL, "gpt-4o", "")
		ctx := context.Background()

		// Generate
		res, err := adapter.Generate(ctx, LLMRequest{UserPrompt: "Hello", Temperature: 0.7, MaxTokens: 100})
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if res != "Hello from mock OpenAI!" {
			t.Errorf("unexpected Generate result: %s", res)
		}

		// GenerateStructured
		var out TestOutput
		err = adapter.GenerateStructured(ctx, LLMRequest{UserPrompt: "Hello"}, &out)
		if err != nil {
			t.Fatalf("GenerateStructured failed: %v", err)
		}
		if out.Ticker != "TSLA" || out.Signal != "BUY" {
			t.Errorf("unexpected GenerateStructured output: %+v", out)
		}
	})

	// 4. Test AnthropicAdapter (by setting ANTHROPIC_BASE_URL env var)
	t.Run("AnthropicAdapter", func(t *testing.T) {
		_ = os.Setenv("ANTHROPIC_BASE_URL", server.URL)
		defer func() { _ = os.Unsetenv("ANTHROPIC_BASE_URL") }()

		adapter := NewAnthropicAdapter("mock-key", "claude-3-7-sonnet", "")
		ctx := context.Background()

		// Generate with thinking config
		res, err := adapter.Generate(ctx, LLMRequest{
			UserPrompt: "Hello",
			Thinking: &ThinkingConfig{
				BudgetTokens: 1024,
			},
		})
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if res != "Hello from mock Anthropic!" {
			t.Errorf("unexpected Generate result: %s", res)
		}

		// GenerateStructured
		var out TestOutput
		err = adapter.GenerateStructured(ctx, LLMRequest{
			UserPrompt: "Hello",
			Thinking: &ThinkingConfig{
				BudgetTokens: 1024,
			},
		}, &out)
		if err != nil {
			t.Fatalf("GenerateStructured failed: %v", err)
		}
		if out.Ticker != "TSLA" || out.Signal != "BUY" {
			t.Errorf("unexpected GenerateStructured output: %+v", out)
		}
	})

	// 5. Test OpenAI-Compatible Mapped Providers via NewLLMProvider baseURLEverride
	providers := []struct {
		name  string
		model string
	}{
		{providerDeepSeek, "deepseek-reasoner"},
		{providerOllama, "qwen3:latest"},
		{providerQwen, "qwen3.6-plus"},
		{providerGLM, "glm-5"},
		{providerMinimax, "MiniMax-M2.7"}, // Will trigger reasoning_split parameter injection via MiniMaxRoundTripper
		{providerXAI, "grok-4.20-reasoner"},
		{providerOpenRouter, "meta-llama/llama-3"},
		{providerAzure, "gpt-4"},
	}

	for _, prov := range providers {
		t.Run(prov.name, func(t *testing.T) {
			adapter, err := NewLLMProvider(prov.name, prov.model, server.URL, "")
			if err != nil {
				t.Fatalf("NewLLMProvider failed to instantiate %s: %v", prov.name, err)
			}

			ctx := context.Background()

			// Generate
			res, err := adapter.Generate(ctx, LLMRequest{UserPrompt: "Hello", Temperature: 0.5})
			if err != nil {
				t.Fatalf("Generate failed for %s: %v", prov.name, err)
			}
			if res != "Hello from mock OpenAI!" {
				t.Errorf("unexpected Generate result for %s: %s", prov.name, res)
			}

			// GenerateStructured
			var out TestOutput
			err = adapter.GenerateStructured(ctx, LLMRequest{UserPrompt: "Hello"}, &out)
			if err != nil {
				t.Fatalf("GenerateStructured failed for %s: %v", prov.name, err)
			}
			if out.Ticker != "TSLA" || out.Signal != "BUY" {
				t.Errorf("unexpected GenerateStructured output for %s: %+v", prov.name, out)
			}
		})
	}
}

func TestGeminiAdapter_GenerateAndStructured(t *testing.T) {
	// Intercept outgoing HTTP calls to googleapis.com by overriding http.DefaultTransport
	oldTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = oldTransport }()

	// We'll set a mock transport that detects Gemini API hosts and serves custom responses
	http.DefaultTransport = &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			w := httptest.NewRecorder()
			w.Header().Set("Content-Type", "application/json")

			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			bodyStr := string(bodyBytes)

			if strings.Contains(bodyStr, "responseSchema") || strings.Contains(bodyStr, "APPLICATION_JSON") {
				// GenerateStructured response
				resp := `{
					"candidates": [{
						"content": {
							"parts": [{
								"text": "{\"ticker\": \"TSLA\", \"signal\": \"BUY\"}"
							}]
						}
					}]
				}`
				_, _ = w.Write([]byte(resp))
			} else {
				// Standard Generate response
				resp := `{
					"candidates": [{
						"content": {
							"parts": [{
								"text": "Hello from mock Gemini!"
							}]
						}
					}]
				}`
				_, _ = w.Write([]byte(resp))
			}

			return w.Result(), nil
		},
	}

	adapter, err := NewGeminiAdapter("mock-gemini-key", "gemini-2.5-flash", "")
	if err != nil {
		t.Fatalf("NewGeminiAdapter failed: %v", err)
	}

	ctx := context.Background()

	// 1. Generate
	res, err := adapter.Generate(ctx, LLMRequest{
		SystemPrompt: "You are a system.",
		UserPrompt:   "Hello",
		Temperature:  0.8,
		MaxTokens:    500,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if res != "Hello from mock Gemini!" {
		t.Errorf("unexpected result: %s", res)
	}

	// 2. Generate with ThinkingConfig
	_, err = adapter.Generate(ctx, LLMRequest{
		UserPrompt: "Hello",
		Thinking: &ThinkingConfig{
			BudgetTokens: 1024,
		},
	})
	if err != nil {
		t.Fatalf("Generate with thinking failed: %v", err)
	}

	// 3. GenerateStructured
	var out TestOutput
	err = adapter.GenerateStructured(ctx, LLMRequest{UserPrompt: "Hello"}, &out)
	if err != nil {
		t.Fatalf("GenerateStructured failed: %v", err)
	}
	if out.Ticker != "TSLA" || out.Signal != "BUY" {
		t.Errorf("unexpected output: %+v", out)
	}
}
