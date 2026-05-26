package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type mockTripper struct {
	lastRequest *http.Request
	requestBody []byte
	respBody    string
}

func (m *mockTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.lastRequest = req
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err == nil {
			m.requestBody = body
			req.Body = io.NopCloser(bytes.NewBuffer(body))
		}
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(m.respBody)),
		Header:     make(http.Header),
	}, nil
}

func TestNewLLMProvider_AllProviders(t *testing.T) {
	// Set mock environment variables for each provider to satisfy NewLLMProvider key checks.
	envs := map[string]string{
		"OPENAI_API_KEY":       "mock-openai",
		"ANTHROPIC_API_KEY":    "mock-anthropic",
		geminiEnv:              "mock-gemini",
		"AZURE_OPENAI_API_KEY": "mock-azure",
		"XAI_API_KEY":          "mock-xai",
		"DEEPSEEK_API_KEY":     "mock-deepseek",
		"DASHSCOPE_API_KEY":    "mock-qwen-global",
		"DASHSCOPE_CN_API_KEY": "mock-qwen-cn",
		"ZHIPU_API_KEY":        "mock-glm-global",
		"ZHIPU_CN_API_KEY":     "mock-glm-cn",
		"MINIMAX_API_KEY":      "mock-minimax-global",
		"MINIMAX_CN_API_KEY":   "mock-minimax-cn",
		"OPENROUTER_API_KEY":   "mock-openrouter",
	}

	for k, v := range envs {
		if err := os.Setenv(k, v); err != nil {
			t.Fatalf("failed to set env %s: %v", k, err)
		}
		defer func(key string) {
			_ = os.Unsetenv(key)
		}(k)
	}

	providers := []struct {
		name  string
		model string
	}{
		{providerOpenAI, "gpt-4o"},
		{providerAnthropic, "claude-3-7-sonnet"},
		{providerGoogle, "gemini-2.5-flash"},
		{providerGemini, "gemini-2.5-flash"},
		{providerOllama, "qwen3:latest"},
		{providerDeepSeek, "deepseek-reasoner"},
		{providerQwen, "qwen3.6-plus"},
		{providerQwenCN, "qwen3.6-plus"},
		{providerGLM, "glm-5"},
		{providerGLMCN, "glm-5"},
		{providerMinimax, "MiniMax-M2.7"},
		{providerMinimaxCN, "MiniMax-M2.7"},
		{providerXAI, "grok-4.20-reasoner"},
		{providerOpenRouter, "meta-llama/llama-3"},
		{providerAzure, "gpt-4"},
		{"mock", "mock-model"},
	}

	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewLLMProvider(tc.name, tc.model, "", "")
			if err != nil {
				t.Fatalf("failed to create provider %s: %v", tc.name, err)
			}
			if p == nil {
				t.Fatalf("expected provider %s to be non-nil", tc.name)
			}
		})
	}
}

func TestOllama_BaseURLOverride(t *testing.T) {
	_ = os.Unsetenv("OLLAMA_BASE_URL")
	p, err := NewLLMProvider(providerOllama, "qwen3:latest", "", "")
	if err != nil {
		t.Fatalf("failed to create Ollama: %v", err)
	}
	ollama, ok := p.(*OllamaAdapter)
	if !ok {
		t.Fatalf("expected OllamaAdapter, got %T", p)
	}
	// We can't check the private client easily, but we can verify instantiation succeeded.
	if ollama == nil {
		t.Fatal("expected ollama to be non-nil")
	}

	if setErr := os.Setenv("OLLAMA_BASE_URL", "http://my-ollama:11434/v1"); setErr != nil {
		t.Fatalf("failed to set env: %v", setErr)
	}
	defer func() {
		_ = os.Unsetenv("OLLAMA_BASE_URL")
	}()
	p2, err := NewLLMProvider(providerOllama, "qwen3:latest", "", "")
	if err != nil {
		t.Fatalf("failed to create overridden Ollama: %v", err)
	}
	if p2 == nil {
		t.Fatal("expected overridden ollama to be non-nil")
	}
}

func TestMiniMaxRoundTripper(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		inputBody     string
		expectSplit   bool
		expectedSplit bool
	}{
		{
			name:          "MiniMax reasoning model gets split injected",
			model:         "MiniMax-M2.7-highspeed",
			inputBody:     `{"model": "MiniMax-M2.7-highspeed", "messages": []}`,
			expectSplit:   true,
			expectedSplit: true,
		},
		{
			name:          "MiniMax non-reasoning model does not get split injected",
			model:         "MiniMax-Coding-Plan",
			inputBody:     `{"model": "MiniMax-Coding-Plan", "messages": []}`,
			expectSplit:   false,
			expectedSplit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockTripper{
				respBody: `{"choices":[{"message":{"content":"ok"}}]}`,
			}
			rt := &MiniMaxRoundTripper{
				Transport: mock,
				Model:     tc.model,
			}

			req, err := http.NewRequest("POST", "https://api.minimax.io/v1/chat/completions", strings.NewReader(tc.inputBody))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := rt.RoundTrip(req)
			if err != nil {
				t.Fatalf("failed RoundTrip: %v", err)
			}
			_ = resp.Body.Close()

			var payload map[string]interface{}
			err = json.Unmarshal(mock.requestBody, &payload)
			if err != nil {
				t.Fatalf("failed to unmarshal request body: %v", err)
			}

			splitVal, exists := payload["reasoning_split"]
			if tc.expectSplit {
				if !exists {
					t.Errorf("expected 'reasoning_split' parameter to be present")
				} else if splitVal != tc.expectedSplit {
					t.Errorf("expected 'reasoning_split' to be %v, got %v", tc.expectedSplit, splitVal)
				}
			} else {
				if exists {
					t.Errorf("expected 'reasoning_split' parameter to be absent")
				}
			}
		})
	}
}
