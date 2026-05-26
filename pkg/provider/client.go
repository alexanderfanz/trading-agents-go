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

const (
	providerOpenAI     = "openai"
	providerAnthropic  = "anthropic"
	providerGemini     = "gemini"
	providerGoogle     = "google"
	providerAzure      = "azure"
	providerXAI        = "xai"
	providerDeepSeek   = "deepseek"
	providerQwen       = "qwen"
	providerQwenCN     = "qwen-cn"
	providerGLM        = "glm"
	providerGLMCN      = "glm-cn"
	providerMinimax    = "minimax"
	providerMinimaxCN  = "minimax-cn"
	providerOpenRouter = "openrouter"
	providerOllama     = "ollama"

	geminiEnv = "GEMINI_API_KEY"
)

// ProviderAPIKeyEnv maps LLM providers to their canonical API key environment variable.
var ProviderAPIKeyEnv = map[string]string{
	providerOpenAI:     "OPENAI_API_KEY",
	providerAnthropic:  "ANTHROPIC_API_KEY",
	providerGemini:     geminiEnv,
	providerGoogle:     geminiEnv,
	providerAzure:      "AZURE_OPENAI_API_KEY",
	providerXAI:        "XAI_API_KEY",
	providerDeepSeek:   "DEEPSEEK_API_KEY",
	providerQwen:       "DASHSCOPE_API_KEY",
	providerQwenCN:     "DASHSCOPE_CN_API_KEY",
	providerGLM:        "ZHIPU_API_KEY",
	providerGLMCN:      "ZHIPU_CN_API_KEY",
	providerMinimax:    "MINIMAX_API_KEY",
	providerMinimaxCN:  "MINIMAX_CN_API_KEY",
	providerOpenRouter: "OPENROUTER_API_KEY",
}

// ProviderBaseURLs maps LLM providers to their canonical base API URLs.
var ProviderBaseURLs = map[string]string{
	providerXAI:        "https://api.x.ai/v1",
	providerDeepSeek:   "https://api.deepseek.com",
	providerQwen:       "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
	providerQwenCN:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
	providerGLM:        "https://api.z.ai/api/paas/v4/",
	providerGLMCN:      "https://open.bigmodel.cn/api/paas/v4/",
	providerMinimax:    "https://api.minimax.io/v1",
	providerMinimaxCN:  "https://api.minimaxi.com/v1",
	providerOpenRouter: "https://openrouter.ai/api/v1",
	providerOllama:     "http://localhost:11434/v1",
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
		if provLower == providerOllama {
			if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
				baseURL = envURL
			} else {
				baseURL = ProviderBaseURLs[providerOllama]
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
	case providerOpenAI:
		return NewOpenAIAdapter(apiKey, model, debugDir), nil
	case providerAnthropic:
		return NewAnthropicAdapter(apiKey, model, debugDir), nil
	case providerGemini, providerGoogle:
		return NewGeminiAdapter(apiKey, model, debugDir)
	case providerOllama:
		return NewOllamaAdapter(baseURL, model, debugDir), nil
	case providerDeepSeek:
		return NewDeepSeekAdapter(apiKey, baseURL, model, debugDir), nil
	case providerQwen, providerQwenCN:
		return NewQwenAdapter(apiKey, baseURL, model, debugDir), nil
	case providerGLM, providerGLMCN:
		return NewZhipuAdapter(apiKey, baseURL, model, debugDir), nil
	case providerMinimax, providerMinimaxCN:
		return NewMiniMaxAdapter(apiKey, baseURL, model, debugDir), nil
	case providerXAI:
		return NewXAIAdapter(apiKey, baseURL, model, debugDir), nil
	case providerOpenRouter:
		return NewOpenRouterAdapter(apiKey, baseURL, model, debugDir), nil
	case providerAzure:
		return NewAzureAdapter(apiKey, baseURL, model, debugDir), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", providerName)
	}
}
