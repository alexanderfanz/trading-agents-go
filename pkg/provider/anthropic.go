package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// AnthropicAdapter implements the unified LLMProvider interface for Anthropic models.
type AnthropicAdapter struct {
	client anthropic.Client
	model  string
}

// NewAnthropicAdapter instantiates a new adapter using the official Anthropic client.
func NewAnthropicAdapter(apiKey, model string, debugDir string) *AnthropicAdapter {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	if debugDir != "" {
		transport, err := NewDebugLoggingRoundTripper(http.DefaultTransport, debugDir)
		if err == nil {
			opts = append(opts, option.WithHTTPClient(&http.Client{Transport: transport}))
		}
	}

	client := anthropic.NewClient(opts...)
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
		Model:     anthropic.Model(a.model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	// Support Claude 3.7 Sonnet thinking budgets
	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(req.Thinking.BudgetTokens))
		// Ensure MaxTokens is configured higher than the thinking budget
		if int64(req.MaxTokens) <= int64(req.Thinking.BudgetTokens) {
			params.MaxTokens = int64(req.Thinking.BudgetTokens) + 1024
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
		t := reflect.TypeOf(target)
		schema := ConvertTypeToSchema(t)
		schemaBytes, err := json.Marshal(schema)
		if err == nil {
			req.JSONSchema = string(schemaBytes)
		} else {
			return fmt.Errorf("missing JSONSchema definition for structured query and failed to generate: %w", err)
		}
	}

	// Parse raw JSON Schema string to format dynamic Tool InputSchema
	var inputSchema anthropic.ToolInputSchemaParam
	if err := json.Unmarshal([]byte(req.JSONSchema), &inputSchema); err != nil {
		return fmt.Errorf("invalid json schema: %w", err)
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(req.UserPrompt)),
	}

	// Register structured collection tool
	tool := anthropic.ToolParam{
		Name:        "submit_structured_response",
		Description: param.NewOpt("Submit the typed JSON structured representation of the trading agent analysis."),
		InputSchema: inputSchema,
	}

	params := anthropic.MessageNewParams{
		Model:      anthropic.Model(a.model),
		MaxTokens:  int64(req.MaxTokens),
		Messages:   messages,
		Tools:      []anthropic.ToolUnionParam{{OfTool: &tool}},
		// Enforce tool execution strictly
		ToolChoice: anthropic.ToolChoiceParamOfTool("submit_structured_response"),
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(req.Thinking.BudgetTokens))
		if int64(req.MaxTokens) <= int64(req.Thinking.BudgetTokens) {
			params.MaxTokens = int64(req.Thinking.BudgetTokens) + 1024
		}
	}

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return fmt.Errorf("anthropic structured execution error: %w", err)
	}

	// Iterate through message content blocks to capture tool invocation payload
	for _, block := range resp.Content {
		if block.Type == "tool_use" {
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
