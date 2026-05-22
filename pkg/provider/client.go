package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// ThinkingConfig captures parameters for reasoning-focused models.
type ThinkingConfig struct {
	// Token budget allocated to reasoning (supported by Gemini & Anthropic)
	BudgetTokens int
	// Effort level, e.g., "low", "medium", "high" (supported by OpenAI o-series)
	EffortLevel string
}

// LLMRequest encapsulates the input params for standard generation or structured outputs.
type LLMRequest struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float64
	MaxTokens    int
	Thinking     *ThinkingConfig
	// Optional Raw JSON schema used to enforce structured JSON output format
	JSONSchema   string
}

// LLMProvider defines the unified interface across OpenAI, Gemini, and Anthropic.
type LLMProvider interface {
	// Generate requests a standard natural-language response.
	Generate(ctx context.Context, req LLMRequest) (string, error)

	// GenerateStructured unmarshals a structured JSON response directly into the target interface structure.
	GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error
}

// ProviderAPIKeyEnv maps LLM providers to their canonical API key environment variable.
var ProviderAPIKeyEnv = map[string]string{
	"openai":     "OPENAI_API_KEY",
	"anthropic":  "ANTHROPIC_API_KEY",
	"gemini":     "GEMINI_API_KEY",
	"google":     "GEMINI_API_KEY",
	"azure":      "AZURE_OPENAI_API_KEY",
	"xai":        "XAI_API_KEY",
	"deepseek":   "DEEPSEEK_API_KEY",
	"qwen":       "DASHSCOPE_API_KEY",
	"qwen-cn":    "DASHSCOPE_CN_API_KEY",
	"glm":        "ZHIPU_API_KEY",
	"glm-cn":     "ZHIPU_CN_API_KEY",
	"minimax":    "MINIMAX_API_KEY",
	"minimax-cn": "MINIMAX_CN_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
}

// ProviderBaseURLs maps LLM providers to their canonical base API URLs.
var ProviderBaseURLs = map[string]string{
	"xai":        "https://api.x.ai/v1",
	"deepseek":   "https://api.deepseek.com",
	"qwen":       "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
	"qwen-cn":    "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"glm":        "https://api.z.ai/api/paas/v4/",
	"glm-cn":     "https://open.bigmodel.cn/api/paas/v4/",
	"minimax":    "https://api.minimax.io/v1",
	"minimax-cn": "https://api.minimaxi.com/v1",
	"openrouter": "https://openrouter.ai/api/v1",
	"ollama":     "http://localhost:11434/v1",
}

// NewLLMProvider resolves, configures, and instantiates the correct LLM provider adapter.
func NewLLMProvider(providerName, model string, baseURLEverride string, debugDir string) (LLMProvider, error) {
	provLower := strings.ToLower(providerName)

	// Local runtimes and mocks do not authenticate.
	if provLower == "mock" {
		return NewMockProvider(model), nil
	}

	// Resolve the base URL (override takes priority)
	baseURL := baseURLEverride
	if baseURL == "" {
		if provLower == "ollama" {
			if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
				baseURL = envURL
			} else {
				baseURL = ProviderBaseURLs["ollama"]
			}
		} else if url, ok := ProviderBaseURLs[provLower]; ok {
			baseURL = url
		}
	}

	// Resolve key if required
	var apiKey string
	if keyEnv, ok := ProviderAPIKeyEnv[provLower]; ok {
		apiKey = os.Getenv(keyEnv)
		if apiKey == "" {
			return nil, fmt.Errorf("API key for provider '%s' is not set. Please set the %s environment variable", providerName, keyEnv)
		}
	}

	switch provLower {
	case "openai":
		return NewOpenAIAdapter(apiKey, model, debugDir), nil
	case "anthropic":
		return NewAnthropicAdapter(apiKey, model, debugDir), nil
	case "gemini", "google":
		return NewGeminiAdapter(apiKey, model, debugDir)
	case "ollama":
		return NewOllamaAdapter(baseURL, model, debugDir), nil
	case "deepseek":
		return NewDeepSeekAdapter(apiKey, baseURL, model, debugDir), nil
	case "qwen", "qwen-cn":
		return NewQwenAdapter(apiKey, baseURL, model, debugDir), nil
	case "glm", "glm-cn":
		return NewZhipuAdapter(apiKey, baseURL, model, debugDir), nil
	case "minimax", "minimax-cn":
		return NewMiniMaxAdapter(apiKey, baseURL, model, debugDir), nil
	case "xai":
		return NewXAIAdapter(apiKey, baseURL, model, debugDir), nil
	case "openrouter":
		return NewOpenRouterAdapter(apiKey, baseURL, model, debugDir), nil
	case "azure":
		return NewAzureAdapter(apiKey, baseURL, model, debugDir), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", providerName)
	}
}
