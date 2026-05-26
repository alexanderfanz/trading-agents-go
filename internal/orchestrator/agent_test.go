package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"trading-agents-go/pkg/provider"
)

type trackingMockLLM struct {
	generateCalled   bool
	structuredCalled bool
	lastReq          provider.LLMRequest
	structuredTarget interface{}
}

func (m *trackingMockLLM) Generate(_ context.Context, req provider.LLMRequest) (string, error) {
	m.generateCalled = true
	m.lastReq = req
	return "generated-response", nil
}

func (m *trackingMockLLM) GenerateStructured(_ context.Context, req provider.LLMRequest, target interface{}) error {
	m.structuredCalled = true
	m.lastReq = req
	m.structuredTarget = target
	raw := `{"recommendation":"Hold","rationale":"test","strategic_actions":"wait"}`
	return json.Unmarshal([]byte(raw), target)
}

func TestAgentCallForwardsToProvider(t *testing.T) {
	mock := &trackingMockLLM{}
	agent := NewAgent("Test Agent", "Tester", "System instruction", mock)

	out, err := agent.Call(context.Background(), "user prompt")
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if out != "generated-response" {
		t.Fatalf("Call output = %q, want generated-response", out)
	}
	if !mock.generateCalled {
		t.Fatal("expected Generate to be called")
	}
	if mock.lastReq.SystemPrompt != "System instruction" {
		t.Fatalf("system prompt = %q", mock.lastReq.SystemPrompt)
	}
	if mock.lastReq.UserPrompt != "user prompt" {
		t.Fatalf("user prompt = %q", mock.lastReq.UserPrompt)
	}
	if mock.lastReq.Temperature != 0.2 {
		t.Fatalf("temperature = %v, want 0.2", mock.lastReq.Temperature)
	}
}

func TestAgentCallStructuredForwardsToProvider(t *testing.T) {
	mock := &trackingMockLLM{}
	agent := NewAgent("Test Agent", "Tester", "System instruction", mock)

	var plan ResearchPlan
	if err := agent.CallStructured(context.Background(), "structured prompt", &plan); err != nil {
		t.Fatalf("CallStructured returned error: %v", err)
	}
	if !mock.structuredCalled {
		t.Fatal("expected GenerateStructured to be called")
	}
	if mock.lastReq.UserPrompt != "structured prompt" {
		t.Fatalf("user prompt = %q", mock.lastReq.UserPrompt)
	}
	if mock.lastReq.Temperature != 0.1 {
		t.Fatalf("temperature = %v, want 0.1", mock.lastReq.Temperature)
	}
	if plan.Recommendation != "Hold" {
		t.Fatalf("recommendation = %q, want Hold", plan.Recommendation)
	}
}
