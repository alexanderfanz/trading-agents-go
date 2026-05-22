package orchestrator

import (
	"context"
	"trading-agents-go/pkg/provider"
)

// Agent represents a specialized AI agent configured with a specific role, system instruction, and client adapter.
type Agent struct {
	Name             string
	Role             string
	SystemInstruction string
	Client           provider.LLMProvider
}

// NewAgent creates a new type-safe Agent boundary wrapping our unified LLMProvider.
func NewAgent(name, role, systemInstruction string, client provider.LLMProvider) *Agent {
	return &Agent{
		Name:              name,
		Role:              role,
		SystemInstruction: systemInstruction,
		Client:            client,
	}
}

// Call executes the agent's LLM generation cycle under the configured role instructions.
func (a *Agent) Call(ctx context.Context, userPrompt string) (string, error) {
	req := provider.LLMRequest{
		SystemPrompt: a.SystemInstruction,
		UserPrompt:   userPrompt,
		Temperature:  0.2, // Moderate conservative temperature for highly logical reasoning
	}
	return a.Client.Generate(ctx, req)
}

// CallStructured executes the agent's LLM cycle and unmarshals the output directly into a target struct.
func (a *Agent) CallStructured(ctx context.Context, userPrompt string, target interface{}) error {
	req := provider.LLMRequest{
		SystemPrompt: a.SystemInstruction,
		UserPrompt:   userPrompt,
		Temperature:  0.1, // High deterministic temperature for structured validation
	}
	return a.Client.GenerateStructured(ctx, req, target)
}
