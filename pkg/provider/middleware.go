package provider

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	return fmt.Errorf("failed to parse structured JSON from text: raw output length %d bytes: %s", len(rawText), rawText)
}

// DebugLoggingRoundTripper intercepts, clocks, parses, and serializes HTTP provider transactions.
type DebugLoggingRoundTripper struct {
	Proxied  http.RoundTripper
	DebugDir string
}

// NewDebugLoggingRoundTripper creates a logging interceptor and prepares the debug subdirectory.
func NewDebugLoggingRoundTripper(proxied http.RoundTripper, debugDir string) (*DebugLoggingRoundTripper, error) {
	if err := os.MkdirAll(debugDir, 0750); err != nil {
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
	switch req.URL.Host {
	case "api.openai.com":
		provider = providerOpenAI
	case "generativelanguage.googleapis.com":
		provider = providerGemini
	case "api.anthropic.com":
		provider = providerAnthropic
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

	_ = os.WriteFile(filePath, logBytes, 0600)
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
	case providerOpenAI:
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
	case providerGemini:
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
