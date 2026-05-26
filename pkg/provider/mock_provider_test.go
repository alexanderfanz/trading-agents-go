package provider

import (
	"context"
	"strings"
	"testing"
)

func TestMockProvider_Generate(t *testing.T) {
	p := NewMockProvider("")
	if p.Ticker != "AAPL" {
		t.Errorf("expected default ticker 'AAPL', got '%s'", p.Ticker)
	}

	p2 := NewMockProvider("msft")
	if p2.Ticker != "MSFT" {
		t.Errorf("expected ticker 'MSFT', got '%s'", p2.Ticker)
	}

	tests := []struct {
		name         string
		systemPrompt string
		userPrompt   string
		expectSub    string
	}{
		{
			name:         "market analyst",
			systemPrompt: "You are a market analyst.",
			userPrompt:   "",
			expectSub:    "Simulated Market Analyst Report",
		},
		{
			name:         "sentiment analyst",
			systemPrompt: "You are a sentiment analyst.",
			userPrompt:   "",
			expectSub:    "Simulated Sentiment Analyst Report",
		},
		{
			name:         "news analyst",
			systemPrompt: "You are a news analyst.",
			userPrompt:   "",
			expectSub:    "Simulated News Analyst Report",
		},
		{
			name:         "fundamentals analyst",
			systemPrompt: "You are a fundamentals analyst.",
			userPrompt:   "",
			expectSub:    "Simulated Fundamentals Analyst Report",
		},
		{
			name:         "bull analyst simple",
			systemPrompt: "You are a bull analyst.",
			userPrompt:   "",
			expectSub:    "Simulated Bull Pitch",
		},
		{
			name:         "bull analyst countering bear",
			systemPrompt: "You are a bull analyst.",
			userPrompt:   "What about the bear analyst argument?",
			expectSub:    "While the bear correctly highlights valuation premiums",
		},
		{
			name:         "bear analyst",
			systemPrompt: "You are a bear analyst.",
			userPrompt:   "",
			expectSub:    "Simulated Bear Counter-Pitch",
		},
		{
			name:         "aggressive risk",
			systemPrompt: "Evaluate with aggressive risk profile.",
			userPrompt:   "",
			expectSub:    "double the allocation",
		},
		{
			name:         "conservative risk",
			systemPrompt: "Evaluate with conservative risk profile.",
			userPrompt:   "",
			expectSub:    "Tighten stop-loss",
		},
		{
			name:         "neutral risk",
			systemPrompt: "Evaluate with neutral risk profile.",
			userPrompt:   "",
			expectSub:    "strict risk-parity constraints",
		},
		{
			name:         "default fallback",
			systemPrompt: "something else",
			userPrompt:   "",
			expectSub:    "Simulated general analyst response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			res, err := p2.Generate(ctx, LLMRequest{
				SystemPrompt: tc.systemPrompt,
				UserPrompt:   tc.userPrompt,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(res, tc.expectSub) {
				t.Errorf("expected response to contain '%s', got: %s", tc.expectSub, res)
			}
		})
	}
}

func TestMockProvider_GenerateStructured(t *testing.T) {
	p := NewMockProvider("GOOG")

	type PortfolioDecision struct {
		Rating           string  `json:"rating"`
		ExecutiveSummary string  `json:"executive_summary"`
		InvestmentThesis string  `json:"investment_thesis"`
		PriceTarget      float64 `json:"price_target"`
		TimeHorizon      string  `json:"time_horizon"`
	}

	type ResearchDecision struct {
		Recommendation  string   `json:"recommendation"`
		Rationale       string   `json:"rationale"`
		StrategicActions string   `json:"strategic_actions"`
	}

	type TraderDecision struct {
		Action         string `json:"action"`
		Reasoning      string `json:"reasoning"`
		EntryPrice     float64 `json:"entry_price"`
		StopLoss       float64 `json:"stop_loss"`
		PositionSizing string `json:"position_sizing"`
	}

	type GenericDecision struct {
		Message string `json:"message"`
	}

	// 1. Portfolio Manager
	var port PortfolioDecision
	err := p.GenerateStructured(context.Background(), LLMRequest{
		SystemPrompt: "You are a portfolio manager",
	}, &port)
	if err != nil {
		t.Fatalf("portfolio manager failed: %v", err)
	}
	if port.Rating != "Overweight" || port.PriceTarget != 215.00 {
		t.Errorf("unexpected portfolio decision parsed: %+v", port)
	}

	// 2. Research Manager
	var res ResearchDecision
	err = p.GenerateStructured(context.Background(), LLMRequest{
		SystemPrompt: "You are a research manager",
	}, &res)
	if err != nil {
		t.Fatalf("research manager failed: %v", err)
	}
	if res.Recommendation != "Buy" || !strings.Contains(res.Rationale, "flawless fundamentals") {
		t.Errorf("unexpected research decision parsed: %+v", res)
	}

	// 3. Trader
	var trader TraderDecision
	err = p.GenerateStructured(context.Background(), LLMRequest{
		SystemPrompt: "You are a trader",
	}, &trader)
	if err != nil {
		t.Fatalf("trader failed: %v", err)
	}
	if trader.Action != "Buy" || trader.EntryPrice != 182.50 {
		t.Errorf("unexpected trader decision parsed: %+v", trader)
	}

	// 4. Default / Generic fallback
	var generic GenericDecision
	err = p.GenerateStructured(context.Background(), LLMRequest{
		SystemPrompt: "some generic role",
	}, &generic)
	if err != nil {
		t.Fatalf("generic role failed: %v", err)
	}
	if generic.Message != "" {
		t.Errorf("expected empty generic decision, got %+v", generic)
	}
}

func TestMockProvider_ContextCancellation(t *testing.T) {
	p := NewMockProvider("AAPL")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Test Generate context cancellation
	_, err := p.Generate(ctx, LLMRequest{SystemPrompt: "market analyst"})
	if err == nil {
		t.Fatal("expected error on cancelled context for Generate")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	// Test GenerateStructured context cancellation
	var out struct{}
	err = p.GenerateStructured(ctx, LLMRequest{SystemPrompt: "portfolio manager"}, &out)
	if err == nil {
		t.Fatal("expected error on cancelled context for GenerateStructured")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
