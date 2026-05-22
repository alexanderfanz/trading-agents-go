# Component 3: Unified Type-Safe Provider Interface

## 1. Technical Architecture & Data Flows

The Python implementation wraps Gemini, OpenAI, and Anthropic API calls with dynamic dictionary constructors and runtime type-assertion wrappers. This style introduces substantial fragile configuration maps, struggles to support advanced features (e.g. thinking budgets, structured formats), and can trigger silent type conversion failures.

The Go architecture replaces this with a unified, strongly-typed `LLMProvider` facade interface. It maps direct concrete adapters to the official, stable, and modern Go SDKs for all three models:

* **Gemini**: `google.golang.org/genai` (Official V1 client)
* **OpenAI**: `github.com/openai/openai-go` (Official OpenAI client)
* **Anthropic**: `github.com/anthropics/anthropic-sdk-go` (Official Anthropic client)

```
                       ┌─────────────────────────┐
                       │      Client Request     │
                       │ (Context, LLMRequest)   │
                       └────────────┬────────────┘
                                    │
           ┌────────────────────────┼────────────────────────┐
           ▼ (Gemini Adapter)       ▼ (OpenAI Adapter)       ▼ (Anthropic Adapter)
  [google.golang.org/genai]   [openai/openai-go]       [anthropics-sdk-go]
           │                        │                        │
           ▼                        ▼                        ▼
  [genai.GenerateContent]    [Completions.New]        [Messages.New]
           │                        │                        │
     (ResponseSchema)         (ResponseFormat)          (ToolUse / JSON)
  [Strict JSON Validation]   [Strict JSON Validation]  [Strict Schema Validate]
           │                        │                        │
           └────────────────────────┼────────────────────────┘
                                    │
                                    ▼
                       ┌─────────────────────────┐
                       │ Strongly-Typed Struct  │
                       │ Returned to Orchestrator│
                       └─────────────────────────┘
```

### Advanced Provider Features Supported:
1. **Deadline Context Propagation**: Standard Go `context.Context` is carried as the first parameter, facilitating automatic HTTP network cancellations if agent execution cycles time out.
2. **Deep-Thinking Controls**: Dynamic tokens allocation and effort level controls mapped natively for Gemini (using `ThinkingConfig`) and OpenAI (using `ReasoningEffort`).
3. **Strict Structured JSON Output**: Native schemas are passed to Gemini's `ResponseSchema` and OpenAI's structured outputs (`json_schema`). Results are immediately unmarshaled into type-safe target structs.

---

## 2. Go Interfaces & Struct Definitions

```go
package llm

import (
	"context"
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
```

```go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/genai" // Official V1 client
)

// GeminiAdapter wraps the official Google GenAI Go Client.
type GeminiAdapter struct {
	client *genai.Client
	model  string
}

func NewGeminiAdapter(apiKey, model string) (*GeminiAdapter, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	return &GeminiAdapter{client: client, model: model}, nil
}

func (a *GeminiAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	config := &genai.GenerateContentConfig{
		Temperature:     &req.Temperature,
		MaxOutputTokens: &req.MaxTokens,
	}

	if req.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		}
	}

	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		config.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingBudget: int64(req.Thinking.BudgetTokens),
		}
	}

	if req.JSONSchema != "" {
		var schema genai.Schema
		if err := json.Unmarshal([]byte(req.JSONSchema), &schema); err != nil {
			return "", fmt.Errorf("invalid gemini response schema: %w", err)
		}
		config.ResponseMimeType = "application/json"
		config.ResponseSchema = &schema
	}

	resp, err := a.client.Models.GenerateContent(ctx, a.model, genai.NewUserContent(req.UserPrompt), config)
	if err != nil {
		return "", fmt.Errorf("gemini execution error: %w", err)
	}
	return resp.Text, nil
}

func (a *GeminiAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	if req.JSONSchema == "" {
		return fmt.Errorf("missing JSONSchema definition for structured query")
	}

	respStr, err := a.Generate(ctx, req)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(respStr), target)
}
```

```go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIAdapter wraps the official OpenAI Go SDK.
type OpenAIAdapter struct {
	client *openai.Client
	model  string
}

func NewOpenAIAdapter(apiKey, model string) *OpenAIAdapter {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAIAdapter{client: client, model: model}
}

func (a *OpenAIAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	params := openai.ChatCompletionNewParams{
		Model: openai.F(a.model),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(req.SystemPrompt),
			openai.UserMessage(req.UserPrompt),
		}),
	}

	if req.Thinking != nil && req.Thinking.EffortLevel != "" {
		params.ReasoningEffort = openai.F(req.Thinking.EffortLevel)
	}

	if req.JSONSchema != "" {
		var schema interface{}
		if err := json.Unmarshal([]byte(req.JSONSchema), &schema); err != nil {
			return "", fmt.Errorf("invalid json schema configuration: %w", err)
		}
		params.ResponseFormat = openai.F(openai.ChatCompletionResponseFormatParam{
			Type: openai.F(openai.ChatCompletionResponseFormatTypeJSONSchema),
			JSONSchema: openai.F(openai.ChatCompletionResponseFormatJSONSchemaParam{
				Name:        openai.F("structured_target"),
				Schema:      openai.F(schema),
				Strict:      openai.F(true),
			}),
		})
	}

	resp, err := a.client.Chat.Completions.New(ctx, params, option.WithMiddleware())
	if err != nil {
		return "", fmt.Errorf("openai execution error: %w", err)
	}
	return resp.Choices[0].Message.Content, nil
}

func (a *OpenAIAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	if req.JSONSchema == "" {
		return fmt.Errorf("missing JSONSchema definition for structured query")
	}

	respStr, err := a.Generate(ctx, req)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(respStr), target)
}
```

---

## 3. Concrete Client Configurations, Schema Converters, & Strict Validation

To guarantee absolute type safety and operational observability across all model transactions, the provider system introduces concrete configurations, schema engines, robust exception parsing, and request interceptors.

### 3.1. Anthropic Adapter Tool Schema Integration

Since Anthropic does not support a native schema injection parameter (like OpenAI's structured outputs or Gemini's response schema), the Go system implements structured validation using **Anthropic's tool-use mechanism**. 

The adapter registers a specialized collection tool, instructs the model to populate it, and locks `ToolChoice` to force the tool to be executed. Additionally, it handles Claude 3.7's `ThinkingConfig` to support deep-thinking efforts.

```go
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicAdapter implements the unified LLMProvider interface for Anthropic models.
type AnthropicAdapter struct {
	client *anthropic.Client
	model  string
}

// NewAnthropicAdapter instantiates a new adapter using the official Anthropic client.
func NewAnthropicAdapter(apiKey, model string) *AnthropicAdapter {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicAdapter{
		client: client,
		model:  model,
	}
}

// Generate requests a standard natural-language string response.
func (a *AnthropicAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(req.UserPrompt)),
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.F(a.model),
		MaxTokens: anthropic.F(int64(req.MaxTokens)),
		Messages:  anthropic.F(messages),
	}

	if req.SystemPrompt != "" {
		params.System = anthropic.F([]anthropic.SystemMessageParam{
			anthropic.NewSystemMessage(req.SystemPrompt),
		})
	}

	if req.Temperature > 0 {
		params.Temperature = anthropic.F(req.Temperature)
	}

	// Support Claude 3.7 Sonnet thinking budgets
	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		params.Thinking = anthropic.F(anthropic.MessageThinkingConfigParam{
			Type:         anthropic.F(anthropic.MessageThinkingConfigParamTypeEnabled),
			BudgetTokens: anthropic.F(int64(req.Thinking.BudgetTokens)),
		})
		// Ensure MaxTokens is configured higher than the thinking budget
		if int64(req.MaxTokens) <= int64(req.Thinking.BudgetTokens) {
			params.MaxTokens = anthropic.F(int64(req.Thinking.BudgetTokens) + 1024)
		}
	}

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("anthropic execution error: %w", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty content response from anthropic")
	}
	return resp.Content[0].Text, nil
}

// GenerateStructured leverages Anthropic's tool use mode to guarantee typed structured outputs.
func (a *AnthropicAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	if req.JSONSchema == "" {
		return fmt.Errorf("missing JSONSchema definition for structured query")
	}

	// Parse raw JSON Schema string to format dynamic Tool InputSchema
	var parsedSchema map[string]interface{}
	if err := json.Unmarshal([]byte(req.JSONSchema), &parsedSchema); err != nil {
		return fmt.Errorf("invalid json schema: %w", err)
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(req.UserPrompt)),
	}

	// Register structured collection tool
	tool := anthropic.ToolParam{
		Name:        anthropic.F("submit_structured_response"),
		Description: anthropic.F("Submit the typed JSON structured representation of the trading agent analysis."),
		InputSchema: anthropic.F(parsedSchema),
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.F(a.model),
		MaxTokens: anthropic.F(int64(req.MaxTokens)),
		Messages:  anthropic.F(messages),
		Tools:     anthropic.F([]anthropic.ToolParam{tool}),
		// Enforce tool execution strictly
		ToolChoice: anthropic.F(anthropic.ToolChoiceParam{
			Type: anthropic.F(anthropic.ToolChoiceParamTypeTool),
			Name: anthropic.F("submit_structured_response"),
		}),
	}

	if req.SystemPrompt != "" {
		params.System = anthropic.F([]anthropic.SystemMessageParam{
			anthropic.NewSystemMessage(req.SystemPrompt),
		})
	}

	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		params.Thinking = anthropic.F(anthropic.MessageThinkingConfigParam{
			Type:         anthropic.F(anthropic.MessageThinkingConfigParamTypeEnabled),
			BudgetTokens: anthropic.F(int64(req.Thinking.BudgetTokens)),
		})
		if int64(req.MaxTokens) <= int64(req.Thinking.BudgetTokens) {
			params.MaxTokens = anthropic.F(int64(req.Thinking.BudgetTokens) + 1024)
		}
	}

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return fmt.Errorf("anthropic structured execution error: %w", err)
	}

	// Iterate through message content blocks to capture tool invocation payload
	for _, block := range resp.Content {
		if block.Type == anthropic.ContentBlockTypeToolUse {
			// Convert tool use parameters back to JSON raw string
			rawArgs, err := json.Marshal(block.Input)
			if err != nil {
				return fmt.Errorf("failed to marshal tool use arguments: %w", err)
			}
			// Load target interface using robust extraction utility
			return ExtractAndUnmarshalJSON(string(rawArgs), target)
		}
	}

	return fmt.Errorf("model failed to invoke submit_structured_response tool call")
}
```

---

### 3.2. Gemini GenAI V1 Schema Conversions

In the Google GenAI V1 SDK (`google.golang.org/genai`), structured output requires passing a schema of type `*genai.Schema`. Rather than defining dynamic JSON strings manually, the Go system provides a **reflection-driven converter** to build type-safe `*genai.Schema` representations from standard Go structs recursively.

```go
package provider

import (
	"reflect"
	"strings"
	"google.golang.org/genai"
)

// ConvertTypeToSchema converts a standard Go reflect.Type recursively into a *genai.Schema.
// It maps basic kinds, respects "json" and "description" tags, handles nested arrays,
// and determines optional parameters based on pointers and omitempty.
func ConvertTypeToSchema(t reflect.Type) *genai.Schema {
	schema := &genai.Schema{}

	// Handle pointer dereferencing
	if t.Kind() == reflect.Ptr {
		schema = ConvertTypeToSchema(t.Elem())
		schema.Nullable = true
		return schema
	}

	switch t.Kind() {
	case reflect.Struct:
		schema.Type = "OBJECT"
		schema.Properties = make(map[string]*genai.Schema)
		schema.Required = make([]string, 0)

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			
			// Skip unexported fields
			if field.PkgPath != "" {
				continue
			}

			// Read json structural tag
			jsonTag := field.Tag.Get("json")
			if jsonTag == "-" {
				continue
			}

			fieldName := field.Name
			omitempty := false
			if jsonTag != "" {
				parts := strings.Split(jsonTag, ",")
				if len(parts) > 0 && parts[0] != "" {
					fieldName = parts[0]
				}
				for _, part := range parts[1:] {
					if part == "omitempty" {
						omitempty = true
					}
				}
			}

			// Recursively parse struct field schema
			fieldSchema := ConvertTypeToSchema(field.Type)

			// Capture description annotations
			if desc := field.Tag.Get("description"); desc != "" {
				fieldSchema.Description = desc
			}

			schema.Properties[fieldName] = fieldSchema

			// Required configuration: fields are required if they aren't pointers or omitempty
			if !omitempty && field.Type.Kind() != reflect.Ptr {
				schema.Required = append(schema.Required, fieldName)
			}
		}

	case reflect.Slice, reflect.Array:
		schema.Type = "ARRAY"
		schema.Items = ConvertTypeToSchema(t.Elem())

	case reflect.String:
		schema.Type = "STRING"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema.Type = "INTEGER"

	case reflect.Float32, reflect.Float64:
		schema.Type = "NUMBER"

	case reflect.Bool:
		schema.Type = "BOOLEAN"

	default:
		schema.Type = "STRING" // Generic fallback for unsupported custom typings
	}

	return schema
}
```

---

### 3.3. Structured Output Fallback Parsing

If a provider's output filters fail, or if a fallback model ignores structured JSON instructions and includes conversational introductions, the system implements a **multi-stage extraction pipeline**. 

It uses regex to locate markdown JSON fences and falls back to tracking JSON character boundaries (`{` ... `}` and `[` ... `]`) to extract valid JSON blocks from raw textual responses.

```go
package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// Matches json codeblocks wrapped in standard markdown fences: ```json ... ```
	markdownFencedJSONRegex = regexp.MustCompile(`(?s)\x60\x60\x60(?:json)?\s*(.*?)\s*\x60\x60\x60`)
)

// ExtractAndUnmarshalJSON isolates valid JSON strings from conversational and markdown wrappers
// and unmarshals the content directly into the specified target object structure.
func ExtractAndUnmarshalJSON(rawText string, target interface{}) error {
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return errors.New("empty response content received")
	}

	// 1. Direct try: the model output pure, clean JSON
	if err := json.Unmarshal([]byte(trimmed), target); err == nil {
		return nil
	}

	// 2. Fallback: Parse markdown block fences ```json { ... } ``` or ``` { ... } ```
	matches := markdownFencedJSONRegex.FindStringSubmatch(trimmed)
	if len(matches) > 1 {
		jsonBlock := strings.TrimSpace(matches[1])
		if err := json.Unmarshal([]byte(jsonBlock), target); err == nil {
			return nil
		}
	}

	// 3. Fallback: Scan text to locate standard JSON boundaries `{` ... `}`
	firstBrace := strings.Index(trimmed, "{")
	lastBrace := strings.LastIndex(trimmed, "}")
	if firstBrace != -1 && lastBrace != -1 && lastBrace > firstBrace {
		jsonBlock := trimmed[firstBrace : lastBrace+1]
		if err := json.Unmarshal([]byte(jsonBlock), target); err == nil {
			return nil
		}
	}

	// 4. Fallback: Scan text to locate JSON array boundaries `[` ... `]`
	firstBracket := strings.Index(trimmed, "[")
	lastBracket := strings.LastIndex(trimmed, "]")
	if firstBracket != -1 && lastBracket != -1 && lastBracket > firstBracket {
		jsonBlock := trimmed[firstBracket : lastBracket+1]
		if err := json.Unmarshal([]byte(jsonBlock), target); err == nil {
			return nil
		}
	}

	// If all extraction pathways fail, return a comprehensive diagnostic error
	return fmt.Errorf("failed to parse structured JSON from text: raw output length %d bytes", len(rawText))
}
```

---

### 3.4. Robust Middleware Interceptors

To achieve deep operational visibility, the system intercepts network calls at the HTTP transport level. 

By satisfying the `http.RoundTripper` interface, this logging interceptor captures prompt and completion tokens, tracks request duration, and serializes payloads asynchronously to a designated debug folder without disrupting client execution loops.

```go
package provider

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DebugLoggingRoundTripper intercepts, clocks, parses, and serializes HTTP provider transactions.
type DebugLoggingRoundTripper struct {
	Proxied  http.RoundTripper
	DebugDir string
}

// NewDebugLoggingRoundTripper creates a logging interceptor and prepares the debug subdirectory.
func NewDebugLoggingRoundTripper(proxied http.RoundTripper, debugDir string) (*DebugLoggingRoundTripper, error) {
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logging directory: %w", err)
	}
	if proxied == nil {
		proxied = http.DefaultTransport
	}
	return &DebugLoggingRoundTripper{
		Proxied:  proxied,
		DebugDir: debugDir,
	}, nil
}

// TokenUsage isolates prompt and completion usage metrics dynamically.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// LogPayload represents the structure written to the debug logs.
type LogPayload struct {
	Timestamp       string      `json:"timestamp"`
	Provider        string      `json:"provider"`
	URL             string      `json:"url"`
	Method          string      `json:"method"`
	DurationMs      int64       `json:"duration_ms"`
	RequestHeaders  http.Header `json:"request_headers"`
	RequestBody     interface{} `json:"request_body,omitempty"`
	ResponseStatus  string      `json:"response_status"`
	ResponseHeaders http.Header `json:"response_headers"`
	ResponseBody    interface{} `json:"response_body,omitempty"`
	Tokens          *TokenUsage `json:"tokens_used,omitempty"`
}

func (l *DebugLoggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	startTime := time.Now()

	// Capture and clone request payload
	var reqBodyBytes []byte
	if req.Body != nil {
		var err error
		reqBodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))
	}

	// Dispatch downstream request
	resp, err := l.Proxied.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)

	// Capture and clone response payload
	var respBodyBytes []byte
	if resp.Body != nil {
		var err error
		respBodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		resp.Body = io.NopCloser(bytes.NewBuffer(respBodyBytes))
	}

	// Asynchronously log the transaction
	go l.logTransaction(req, reqBodyBytes, resp, respBodyBytes, duration)

	return resp, nil
}

func (l *DebugLoggingRoundTripper) logTransaction(
	req *http.Request, reqBody []byte,
	resp *http.Response, respBody []byte,
	duration time.Duration,
) {
	var parsedReq interface{}
	_ = json.Unmarshal(reqBody, &parsedReq)

	var parsedResp interface{}
	_ = json.Unmarshal(respBody, &parsedResp)

	provider := "unknown"
	urlStr := req.URL.String()
	if req.URL.Host == "api.openai.com" {
		provider = "openai"
	} else if req.URL.Host == "generativelanguage.googleapis.com" {
		provider = "gemini"
	} else if req.URL.Host == "api.anthropic.com" {
		provider = "anthropic"
	}

	tokens := l.extractTokenMetrics(provider, respBody)

	payload := LogPayload{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Provider:        provider,
		URL:             urlStr,
		Method:          req.Method,
		DurationMs:      duration.Milliseconds(),
		RequestHeaders:  req.Header,
		RequestBody:     parsedReq,
		ResponseStatus:  resp.Status,
		ResponseHeaders: resp.Header,
		ResponseBody:    parsedResp,
		Tokens:          tokens,
	}

	logBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}

	// Generate safe, collision-free filename
	uuid := make([]byte, 4)
	_, _ = rand.Read(uuid)
	filename := fmt.Sprintf("%s_%d_%08x.json", provider, time.Now().UnixMilli(), uuid)
	filePath := filepath.Join(l.DebugDir, filename)

	_ = os.WriteFile(filePath, logBytes, 0644)
}

func (l *DebugLoggingRoundTripper) extractTokenMetrics(provider string, body []byte) *TokenUsage {
	if len(body) == 0 {
		return nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}

	switch provider {
	case "openai":
		if usage, ok := raw["usage"].(map[string]interface{}); ok {
			prompt, _ := usage["prompt_tokens"].(float64)
			completion, _ := usage["completion_tokens"].(float64)
			total, _ := usage["total_tokens"].(float64)
			return &TokenUsage{
				PromptTokens:     int(prompt),
				CompletionTokens: int(completion),
				TotalTokens:      int(total),
			}
		}
	case "gemini":
		if usage, ok := raw["usageMetadata"].(map[string]interface{}); ok {
			prompt, _ := usage["promptTokenCount"].(float64)
			completion, _ := usage["candidatesTokenCount"].(float64)
			total, _ := usage["totalTokenCount"].(float64)
			return &TokenUsage{
				PromptTokens:     int(prompt),
				CompletionTokens: int(completion),
				TotalTokens:      int(total),
			}
		}
	}
	return nil
}
```

#### Plugging Middleware into Official Clients

##### OpenAI Client Setup
```go
transport, _ := NewDebugLoggingRoundTripper(http.DefaultTransport, "./debug/llm_logs")
client := openai.NewClient(
	option.WithAPIKey(apiKey),
	option.WithHTTPClient(&http.Client{Transport: transport}),
)
```

##### Gemini Client Setup
```go
transport, _ := NewDebugLoggingRoundTripper(http.DefaultTransport, "./debug/llm_logs")
ctx := context.Background()
client, err := genai.NewClient(ctx, &genai.ClientConfig{
	APIKey:     apiKey,
	HTTPClient: &http.Client{Transport: transport},
})
```

---

## 4. Step-by-Step Implementation Sub-plan

- [ ] **1. Scaffolding Provider Contracts**: 
  - Define all structure models and adapter facades inside `pkg/provider/client.go`.
- [ ] **2. Gemini Client**:
  - Integrate the official `google.golang.org/genai` library in `pkg/provider/gemini.go`.
  - Implement standard `ConvertTypeToSchema` dynamic reflection utility inside the client.
- [ ] **3. OpenAI Client**:
  - Integrate official `github.com/openai/openai-go` in `pkg/provider/openai.go`.
  - Wire structured output format configurations matching the OpenAI type validation rules.
- [ ] **4. Anthropic Client**:
  - Integrate official `github.com/anthropics/anthropic-sdk-go` in `pkg/provider/anthropic.go`.
  - Implement tool-use based schema extraction patterns and Claude 3.7 thinking configuration models.
- [ ] **5. Middleware Integration & Extraction**:
  - Scaffolding the `DebugLoggingRoundTripper` custom HTTP middleware and `ExtractAndUnmarshalJSON` extraction library.
- [ ] **6. Unified Integration Tests**:
  - Add comprehensive unit tests evaluating generation calls across all three adapters using mocked API network endpoints.

---

## 5. Idiomatic Trade-offs

### Official Go SDK Integrations over Legacy Community Packages
* **Python Pattern**: Relies on general wrapper libraries that mask specific, advanced API capabilities.
* **Go Pattern**: Using direct, modern, and official Go clients (e.g. `google.golang.org/genai`) guarantees day-one access to advanced parameters like thinking budgets and reasoning controls without waiting for third-party community library updates.

### Compile-Time Interface Safety over Metaprogramming Factories
* **Python Pattern**: Uses inheritance mapping files with implicit properties, making validation fragile.
* **Go Pattern**: Structural polymorphism enforces type-safety directly at build time. Adapters are entirely self-contained, stateless, and trivially mockable using standard Go testing interfaces.

