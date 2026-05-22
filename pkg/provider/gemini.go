package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"google.golang.org/genai"
)

// GeminiAdapter wraps the official Google GenAI Go Client.
type GeminiAdapter struct {
	client *genai.Client
	model  string
}

// NewGeminiAdapter instantiates a new adapter using the official Google GenAI Go Client.
func NewGeminiAdapter(apiKey, model string, debugDir string) (*GeminiAdapter, error) {
	ctx := context.Background()
	config := &genai.ClientConfig{
		APIKey: apiKey,
	}

	if debugDir != "" {
		transport, err := NewDebugLoggingRoundTripper(http.DefaultTransport, debugDir)
		if err == nil {
			config.HTTPClient = &http.Client{Transport: transport}
		}
	}

	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return nil, err
	}
	return &GeminiAdapter{client: client, model: model}, nil
}

// Generate requests a standard natural-language response.
func (a *GeminiAdapter) Generate(ctx context.Context, req LLMRequest) (string, error) {
	config := &genai.GenerateContentConfig{}

	if req.Temperature > 0 {
		temp32 := float32(req.Temperature)
		config.Temperature = &temp32
	}

	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}

	if req.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		}
	}

	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		budget32 := int32(req.Thinking.BudgetTokens)
		config.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingBudget: &budget32,
		}
		// Temperature is typically not supported for thinking configurations.
		config.Temperature = nil
	}

	if req.JSONSchema != "" {
		var schema genai.Schema
		if err := json.Unmarshal([]byte(req.JSONSchema), &schema); err != nil {
			return "", fmt.Errorf("invalid gemini response schema: %w", err)
		}
		config.ResponseMIMEType = "application/json"
		config.ResponseSchema = &schema
	}

	resp, err := a.client.Models.GenerateContent(ctx, a.model, genai.Text(req.UserPrompt), config)
	if err != nil {
		return "", fmt.Errorf("gemini execution error: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned empty content")
	}

	return resp.Candidates[0].Content.Parts[0].Text, nil
}

// GenerateStructured unmarshals a structured JSON response directly into the target interface structure.
func (a *GeminiAdapter) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
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

func boolPtr(b bool) *bool {
	return &b
}

// ConvertTypeToSchema converts a standard Go reflect.Type recursively into a *genai.Schema.
// It maps basic kinds, respects "json" and "description" tags, handles nested arrays,
// and determines optional parameters based on pointers and omitempty.
func ConvertTypeToSchema(t reflect.Type) *genai.Schema {
	schema := &genai.Schema{}

	// Handle pointer dereferencing
	if t.Kind() == reflect.Ptr {
		schema = ConvertTypeToSchema(t.Elem())
		schema.Nullable = boolPtr(true)
		return schema
	}

	switch t.Kind() {
	case reflect.Struct:
		schema.Type = genai.TypeObject
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
		schema.Type = genai.TypeArray
		schema.Items = ConvertTypeToSchema(t.Elem())

	case reflect.String:
		schema.Type = genai.TypeString

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema.Type = genai.TypeInteger

	case reflect.Float32, reflect.Float64:
		schema.Type = genai.TypeNumber

	case reflect.Bool:
		schema.Type = genai.TypeBoolean

	default:
		schema.Type = genai.TypeString // Generic fallback for unsupported custom typings
	}

	return schema
}
