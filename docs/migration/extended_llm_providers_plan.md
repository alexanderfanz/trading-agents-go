# Extended LLM Providers Integration Plan

This document outlines the detailed architecture and step-by-step implementation plan to expand the LLM provider catalog in `trading-agents-go`. By evaluating the Python reference implementation `TradingAgents`, we will add robust support for advanced reasoning/deep-thinking configs and multiple new cloud/local providers: `xAI (Grok)`, `DeepSeek`, `Qwen (DashScope)`, `GLM/Zhipu`, `MiniMax`, `OpenRouter`, `Azure OpenAI`, and `Ollama`.

---

## User Review Required

We are introducing a unified factory architecture in `pkg/provider` that supports:
1. **Dynamic Environment Key Resolving**: Distinct environment variables for each cloud provider (e.g. `ZHIPU_API_KEY`, `DASHSCOPE_API_KEY`) to prevent key leakage and collisions.
2. **Unified Custom Base URLs**: Ability to override any provider's API base URL with standard environment variables (e.g., `OLLAMA_BASE_URL` or custom proxy configs).
3. **Advanced Reasoning / Deep-Thinking Controls**: Mapping the unified `ThinkingConfig` struct to Anthropic's new thinking blocks, Gemini's thinking budgets, and OpenAI's/DeepSeek's/Grok's reasoning parameters.
4. **Provider-Specific HTTP Interceptors**: Handling unique vendor quirks (like MiniMax's `reasoning_split` requirement to keep thought blocks separate from final outputs) transparently via Go `http.RoundTripper` middleware.

---

## Proposed Changes

We will introduce the following components, ordered logically.

### 1. Unified Router Architecture
#### [MODIFY] [client.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/client.go)
- Add a unified provider factory function `NewLLMProvider(providerName, model, baseURLEverride string, debugDir string) (LLMProvider, error)`.
- Define the canonical provider API-key environment mapping and default base URLs matching the reference Python design:
  - `openai` -> `OPENAI_API_KEY`
  - `anthropic` -> `ANTHROPIC_API_KEY`
  - `gemini` -> `GEMINI_API_KEY`
  - `azure` -> `AZURE_OPENAI_API_KEY`
  - `xai` -> `XAI_API_KEY`
  - `deepseek` -> `DEEPSEEK_API_KEY`
  - `qwen` -> `DASHSCOPE_API_KEY`
  - `qwen-cn` -> `DASHSCOPE_CN_API_KEY`
  - `glm` -> `ZHIPU_API_KEY`
  - `glm-cn` -> `ZHIPU_CN_API_KEY`
  - `minimax` -> `MINIMAX_API_KEY`
  - `minimax-cn` -> `MINIMAX_CN_API_KEY`
  - `openrouter` -> `OPENROUTER_API_KEY`
  - `ollama` -> (None required, supports `OLLAMA_BASE_URL` override)

#### [MODIFY] [openai.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/openai.go)
- Expose a constructor `NewOpenAICompatibleAdapter(apiKey, baseURL, model, debugDir string) *OpenAIAdapter` to allow other compatible providers to reuse OpenAI's highly stable tool-use JSON-Schema translation and generation routines.

---

### 2. New Provider Adapters
We will write separate provider files under `pkg/provider/` containing dedicated lightweight adapter structures implementing the `LLMProvider` interface.

#### [NEW] [ollama.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/ollama.go)
- Implements the Ollama local adapter wrapping the compatible OpenAI adapter.
- Auto-resolves the endpoint via `OLLAMA_BASE_URL` env override, falling back to `http://localhost:11434/v1`.

#### [NEW] [deepseek.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/deepseek.go)
- Implements the DeepSeek adapter wrapping `NewOpenAICompatibleAdapter`.
- Uses `DEEPSEEK_API_KEY` and defaults to `https://api.deepseek.com`.

#### [NEW] [qwen.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/qwen.go)
- Implements Qwen (DashScope) adapter.
- Supports both `qwen` (international, `DASHSCOPE_API_KEY`) and `qwen-cn` (China, `DASHSCOPE_CN_API_KEY`) with the respective base URLs.

#### [NEW] [zhipu.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/zhipu.go)
- Implements GLM/Zhipu adapter.
- Supports both `glm` (international, `ZHIPU_API_KEY`) and `glm-cn` (China, `ZHIPU_CN_API_KEY`) with their respective base URLs.

#### [NEW] [minimax.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/minimax.go)
- Implements MiniMax adapter.
- Supports `minimax` (global, `MINIMAX_API_KEY`) and `minimax-cn` (China, `MINIMAX_CN_API_KEY`).
- Registers a specialized `http.RoundTripper` to inject `reasoning_split: true` in the outgoing JSON payload so that thinking blocks are cleanly separated from final outputs.

#### [NEW] [xai.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/xai.go)
- Implements xAI (Grok) adapter.
- Uses `XAI_API_KEY` and base URL `https://api.x.ai/v1`.

#### [NEW] [openrouter.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/openrouter.go)
- Implements OpenRouter adapter.
- Uses `OPENROUTER_API_KEY` and base URL `https://openrouter.ai/api/v1`.

#### [NEW] [azure.go](file:///Users/alex/repos/personal/trading-agents-go/pkg/provider/azure.go)
- Implements Azure OpenAI adapter.
- Configures `openai-go` to authenticate via Azure's standard headers (e.g. `api-key`) and custom API base URL structure.

---

### 3. Orchestrator and Main Entry Integration
#### [MODIFY] [cmd/tradingagents/main.go](file:///Users/alex/repos/personal/trading-agents-go/cmd/tradingagents/main.go)
- Update the LLM client switch-case statement to use `provider.NewLLMProvider` dynamically, resolving CLI flags and environment configurations.

---

## Verification Plan

### Automated Tests
- Create `/Users/alex/repos/personal/trading-agents-go/pkg/provider/extended_provider_test.go` with unit and integration tests.
- Verify:
  - Base URL routing for all new providers.
  - Environment variable resolution (no leakage or collisions).
  - High-fidelity formatting of system and user prompts.
  - JSON schema payload conversion and structured parsing.
  - MiniMax custom RoundTripper injects the `reasoning_split` payload successfully.

Run the test suite using:
```bash
go test -v ./pkg/provider/...
```
