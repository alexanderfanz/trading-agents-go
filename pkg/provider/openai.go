package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// OpenAIAdapter wraps the official OpenAI Go SDK.
type OpenAIAdapter struct {
	client openai.Client
	model  string
}

// NewOpenAIAdapter instantiates a new adapter using the official OpenAI Go SDK.
func NewOpenAIAdapter(apiKey, model string, debugDir string) *OpenAIAdapter {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	if debugDir != "" {
		transport, err := NewDebugLoggingRoundTripper(http.DefaultTransport, debugDir)
		if err == nil {
			opts = append(opts, option.WithHTTPClient(&http.Client{Transport: transport}))
		}
	}

	client := openai.NewClient(opts...)
	return &OpenAIAdapter{client: client, model: model}
}

// NewOpenAICompatibleAdapter instantiates a new adapter pointing to a custom OpenAI-compatible endpoint.
func NewOpenAICompatibleAdapter(apiKey, baseURL, model string, debugDir string) *OpenAIAdapter {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	}

	if debugDir != "" {
		transport, err := NewDebugLoggingRoundTripper(http.DefaultTransport, debugDir)
		if err == nil {
			opts = append(opts, option.WithHTTPClient(&http.Client{Transport: transport}))
		}
	}

	client := openai.NewClient(opts...)
	return &OpenAIAdapter{client: client, model: model}
}


// Generate requests a standard natural-language response.
func (a *OpenAIAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0)
	if req.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(req.SystemPrompt))
	}
	messages = append(messages, openai.UserMessage(req.UserPrompt))

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(a.model),
		Messages: messages,
	}

	if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = param.NewOpt(int64(req.MaxTokens))
	}

	if req.Thinking != nil && req.Thinking.EffortLevel != "" {
		params.ReasoningEffort = shared.ReasoningEffort(req.Thinking.EffortLevel)
		// For reasoning models (e.g. o1/o3-mini), temperature/max_tokens rules might differ.
		params.Temperature = param.Null[float64]()
	}

	if req.JSONSchema != "" {
		var schema interface{}
		if err := json.Unmarshal([]byte(req.JSONSchema), &schema); err != nil {
			return "", fmt.Errorf("invalid json schema configuration: %w", err)
		}
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "structured_target",
					Strict: param.NewOpt(true),
					Schema: schema,
				},
			},
		}
	}

	resp, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("openai execution error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no completion choices")
	}

	return resp.Choices[0].Message.Content, nil
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface structure.
func (a *OpenAIAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
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

	respStr, err := a.Generate(ctx, req)
	if err != nil {
		return err
	}

	return ExtractAndUnmarshalJSON(respStr, target)
}
