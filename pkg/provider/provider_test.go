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
	defer os.RemoveAll(tempDir)

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

	// Give the async logging a moment to write to the file
	time.Sleep(100 * time.Millisecond)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read logging dir: %v", err)
	}

	if len(files) == 0 {
		t.Fatalf("expected log file to be written")
	}

	// Read and verify log payload
	logFilePath := filepath.Join(tempDir, files[0].Name())
	logData, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var payload LogPayload
	if err := json.Unmarshal(logData, &payload); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if payload.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %s", payload.Provider)
	}
	if payload.Tokens == nil || payload.Tokens.PromptTokens != 12 || payload.Tokens.CompletionTokens != 8 {
		t.Errorf("incorrect tokens logged: %+v", payload.Tokens)
	}
}
