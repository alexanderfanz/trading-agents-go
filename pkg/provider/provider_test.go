package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type TestSubStruct struct {
	Active bool    `json:"active" description:"Whether active"`
	Ratio  float64 `json:"ratio"`
}

type TestStruct struct {
	Name        string         `json:"name" description:"User name"`
	Age         int            `json:"age"`
	Score       *int           `json:"score,omitempty"`
	Tags        []string       `json:"tags"`
	Sub         TestSubStruct  `json:"sub"`
	PtrSub      *TestSubStruct `json:"ptr_sub,omitempty"`
	IgnoredField string        `json:"-"`
}

func TestConvertTypeToSchema(t *testing.T) {
	typ := reflect.TypeOf(TestStruct{})
	schema := ConvertTypeToSchema(typ)

	if schema.Type != "OBJECT" {
		t.Errorf("expected OBJECT, got %s", schema.Type)
	}

	properties := schema.Properties
	if properties == nil {
		t.Fatalf("expected properties to be populated")
	}

	nameSchema, ok := properties["name"]
	if !ok {
		t.Fatalf("expected 'name' property")
	}
	if nameSchema.Type != "STRING" || nameSchema.Description != "User name" {
		t.Errorf("invalid 'name' schema: %+v", nameSchema)
	}

	ageSchema, ok := properties["age"]
	if !ok {
		t.Fatalf("expected 'age' property")
	}
	if ageSchema.Type != "INTEGER" {
		t.Errorf("expected 'age' to be INTEGER, got %s", ageSchema.Type)
	}

	tagsSchema, ok := properties["tags"]
	if !ok {
		t.Fatalf("expected 'tags' property")
	}
	if tagsSchema.Type != "ARRAY" || tagsSchema.Items == nil || tagsSchema.Items.Type != "STRING" {
		t.Errorf("invalid 'tags' schema: %+v", tagsSchema)
	}

	subSchema, ok := properties["sub"]
	if !ok {
		t.Fatalf("expected 'sub' property")
	}
	if subSchema.Type != "OBJECT" {
		t.Errorf("expected 'sub' to be OBJECT, got %s", subSchema.Type)
	}

	// Verify required fields: Name, Age, Tags, Sub should be required.
	// Score and PtrSub are not required because of omitempty/pointers.
	requiredMap := make(map[string]bool)
	for _, req := range schema.Required {
		requiredMap[req] = true
	}

	if !requiredMap["name"] {
		t.Errorf("expected 'name' to be required")
	}
	if !requiredMap["age"] {
		t.Errorf("expected 'age' to be required")
	}
	if requiredMap["score"] {
		t.Errorf("expected 'score' to NOT be required")
	}
	if requiredMap["ptr_sub"] {
		t.Errorf("expected 'ptr_sub' to NOT be required")
	}
}

func TestExtractAndUnmarshalJSON(t *testing.T) {
	type Output struct {
		Ticker string `json:"ticker"`
		Signal string `json:"signal"`
	}

	tests := []struct {
		name    string
		input   string
		want    Output
		wantErr bool
	}{
		{
			name:    "clean json",
			input:   `{"ticker": "AAPL", "signal": "BUY"}`,
			want:    Output{Ticker: "AAPL", Signal: "BUY"},
			wantErr: false,
		},
		{
			name:    "markdown fenced json",
			input:   "```json\n{\n  \"ticker\": \"MSFT\",\n  \"signal\": \"SELL\"\n}\n```",
			want:    Output{Ticker: "MSFT", Signal: "SELL"},
			wantErr: false,
		},
		{
			name:    "conversational braces text",
			input:   "Sure! Here is the trading signal:\n{\n  \"ticker\": \"GOOG\",\n  \"signal\": \"HOLD\"\n}\nHope this helps!",
			want:    Output{Ticker: "GOOG", Signal: "HOLD"},
			wantErr: false,
		},
		{
			name:    "invalid json",
			input:   "No JSON here",
			want:    Output{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Output
			err := ExtractAndUnmarshalJSON(tt.input, &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error: %v, got: %v", tt.wantErr, err)
			}
			if !tt.wantErr {
				if got.Ticker != tt.want.Ticker || got.Signal != tt.want.Signal {
					t.Errorf("expected %+v, got %+v", tt.want, got)
				}
			}
		})
	}
}

type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestDebugLoggingRoundTripper(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llm_logs_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	mockRespBody := `{"usage": {"prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20}}`
	mock := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(mockRespBody)),
			}, nil
		},
	}

	tripper, err := NewDebugLoggingRoundTripper(mock, tempDir)
	if err != nil {
		t.Fatalf("failed to create logging tripper: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBufferString(`{"model": "gpt-4o"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := tripper.RoundTrip(req)
	if err != nil {
		t.Fatalf("failed roundtrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if string(body) != mockRespBody {
		t.Errorf("expected %s, got %s", mockRespBody, string(body))
	}

	// Poll until the log file is written or timeout occurs
	var files []os.DirEntry
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		files, err = os.ReadDir(tempDir)
		if err == nil && len(files) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err != nil {
		t.Fatalf("failed to read logging dir: %v", err)
	}

	if len(files) == 0 {
		t.Fatalf("expected log file to be written within timeout")
	}

	// Read and verify log payload
	logFilePath := filepath.Clean(filepath.Join(tempDir, files[0].Name()))
	logData, err := os.ReadFile(logFilePath) // #nosec G304
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var payload LogPayload
	if err := json.Unmarshal(logData, &payload); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if payload.Provider != providerOpenAI {
		t.Errorf("expected provider 'openai', got %s", payload.Provider)
	}
	if payload.Tokens == nil || payload.Tokens.PromptTokens != 12 || payload.Tokens.CompletionTokens != 8 {
		t.Errorf("incorrect tokens logged: %+v", payload.Tokens)
	}
}

func TestNewLLMProvider_Errors(t *testing.T) {
	// Test unsupported provider
	_, err := NewLLMProvider("unsupported-provider", "model", "", "")
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}

	// Test missing API key for OpenAI
	t.Setenv("OPENAI_API_KEY", "")

	_, err = NewLLMProvider("openai", "gpt-4o", "", "")
	if err == nil {
		t.Fatal("expected error for missing OpenAI API key")
	}
	if !strings.Contains(err.Error(), "API key for provider 'openai' is not set") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtractAndUnmarshalJSON_Errors(t *testing.T) {
	var out struct {
		Val string `json:"val"`
	}

	// Empty input
	err := ExtractAndUnmarshalJSON("", &out)
	if err == nil {
		t.Fatal("expected error for empty text")
	}

	// Invalid JSON inside block fences
	err = ExtractAndUnmarshalJSON("```json\n{invalid\n```", &out)
	if err == nil {
		t.Fatal("expected error for invalid fenced JSON")
	}

	// Invalid standard braces
	err = ExtractAndUnmarshalJSON("conversational {invalid} text", &out)
	if err == nil {
		t.Fatal("expected error for invalid braced JSON")
	}

	// Invalid standard brackets
	err = ExtractAndUnmarshalJSON("conversational [invalid] text", &out)
	if err == nil {
		t.Fatal("expected error for invalid bracketed JSON")
	}
}

func TestDebugLoggingRoundTripper_GeminiTokenMetrics(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llm_logs_gemini_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	mockRespBody := `{"usageMetadata": {"promptTokenCount": 15, "candidatesTokenCount": 25, "totalTokenCount": 40}}`
	mock := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(mockRespBody)),
			}
			resp.Header.Set("Content-Type", "application/json")
			return resp, nil
		},
	}

	tripper, err := NewDebugLoggingRoundTripper(mock, tempDir)
	if err != nil {
		t.Fatalf("failed to create logging tripper: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := tripper.RoundTrip(req)
	if err != nil {
		t.Fatalf("failed roundtrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Poll until the log file is written or timeout occurs
	var files []os.DirEntry
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		files, err = os.ReadDir(tempDir)
		if err == nil && len(files) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err != nil {
		t.Fatalf("failed to read logging dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected log file to be written within timeout")
	}

	logFilePath := filepath.Clean(filepath.Join(tempDir, files[0].Name()))
	logData, err := os.ReadFile(logFilePath) // #nosec G304
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var payload LogPayload
	if err := json.Unmarshal(logData, &payload); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if payload.Provider != providerGemini {
		t.Errorf("expected provider 'gemini', got %s", payload.Provider)
	}
	if payload.Tokens == nil || payload.Tokens.PromptTokens != 15 || payload.Tokens.CompletionTokens != 25 {
		t.Errorf("incorrect tokens logged: %+v", payload.Tokens)
	}
}

func TestDebugLoggingRoundTripper_TokenMetricsEdgeCases(t *testing.T) {
	tripper := &DebugLoggingRoundTripper{}

	// Nil body
	metrics := tripper.extractTokenMetrics(providerOpenAI, nil)
	if metrics != nil {
		t.Errorf("expected nil metrics for nil body, got %+v", metrics)
	}

	// Invalid JSON
	metrics = tripper.extractTokenMetrics(providerOpenAI, []byte("invalid-json"))
	if metrics != nil {
		t.Errorf("expected nil metrics for invalid json, got %+v", metrics)
	}

	// Unknown provider
	metrics = tripper.extractTokenMetrics("unknown-provider", []byte(`{"usage": {}}`))
	if metrics != nil {
		t.Errorf("expected nil metrics for unknown provider, got %+v", metrics)
	}
}

func TestAdapters_DebugDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llm_debug_dir_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	// 1. OpenAI
	oa := NewOpenAIAdapter("mock-key", "gpt-4o", tempDir)
	if oa == nil {
		t.Fatal("expected OpenAIAdapter to be non-nil")
	}

	// OpenAI Compatible
	oac := NewOpenAICompatibleAdapter("mock-key", "http://localhost", "gpt-4o", tempDir)
	if oac == nil {
		t.Fatal("expected OpenAIAdapter to be non-nil")
	}

	// 2. Anthropic
	aa := NewAnthropicAdapter("mock-key", "claude-3-7-sonnet", tempDir)
	if aa == nil {
		t.Fatal("expected AnthropicAdapter to be non-nil")
	}

	// 3. Gemini
	ga, err := NewGeminiAdapter("mock-key", "gemini-2.5-flash", tempDir)
	if err != nil {
		t.Fatalf("NewGeminiAdapter failed: %v", err)
	}
	if ga == nil {
		t.Fatal("expected GeminiAdapter to be non-nil")
	}

	// 4. Azure
	az := NewAzureAdapter("mock-key", "http://localhost", "gpt-4", tempDir)
	if az == nil {
		t.Fatal("expected AzureAdapter to be non-nil")
	}
}


