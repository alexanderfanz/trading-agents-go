package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// MockProvider is a dry-run provider that yields realistic simulated reports and structured debate decisions.
type MockProvider struct {
	Ticker string
}

// NewMockProvider creates a new mock LLM provider.
func NewMockProvider(ticker string) *MockProvider {
	if ticker == "" {
		ticker = "AAPL"
	}
	return &MockProvider{Ticker: strings.ToUpper(ticker)}
}

// Generate implements LLMProvider.
func (p *MockProvider) Generate(ctx context.Context, req LLMRequest) (string, error) {
	// Simple artificial latency
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(150 * time.Millisecond):
	}

	sys := strings.ToLower(req.SystemPrompt)
	user := strings.ToLower(req.UserPrompt)

	switch {
	case strings.Contains(sys, "market analyst"):
		return fmt.Sprintf(`### Simulated Market Analyst Report for %[1]s
The technical indicators present a highly favorable setup for %[1]s.
- The 50-day SMA is currently trending above the 200-day SMA, confirming a robust long-term bullish golden cross.
- RSI is resting at a healthy 58.60, indicating steady upward momentum without crossing into overbought territory.
- MACD shows a fresh bullish histogram expansion (MACD Line > Signal Line).
- Bollinger Bands suggest a volatility squeeze is forming, priming the stock for a breakout.

| Indicator | Value | Signal |
|---|---|---|
| SMA 50 | Trending Up | Bullish |
| SMA 200 | Support | Bullish |
| RSI (14) | 58.60 | Neutral-Bullish |
| MACD | Expansion | Bullish |
| Bollinger Bands | Squeeze | Breakout |`, p.Ticker), nil

	case strings.Contains(sys, "sentiment analyst"):
		return fmt.Sprintf(`### Simulated Sentiment Analyst Report for %[1]s
Retail and institutional sentiment metrics reflect extreme interest and positive anticipation.
- Social media volume (Twitter/StockTwits) has spiked 35%% over the past 48 hours.
- Mention sentiment is 72%% positive, focusing on the upcoming earnings call and product innovation cycles.
- Options flow data displays heavy call buying at strike prices 10%% out-of-the-money.
- Short interest remains low at 1.8%%, minimizing any immediate short-squeeze risks but highlighting strong underlying confidence.`, p.Ticker), nil

	case strings.Contains(sys, "news analyst"):
		return fmt.Sprintf(`### Simulated News Analyst Report for %[1]s
Catalyst mapping reveals significant near-term macro and micro drivers.
- **Insider activity**: CEO and major officers acquired over $2.5M in equity during open-market transactions last week.
- **Product momentum**: Wall Street reports suggest supply constraints for new hardware versions are easing rapidly.
- **Macro outlook**: Treasury yield stabilization provides a highly supportive valuation anchor for growth leaders like %[1]s.`, p.Ticker), nil

	case strings.Contains(sys, "fundamentals analyst"):
		return fmt.Sprintf(`### Simulated Fundamentals Analyst Report for %[1]s
Financial statement auditing shows exceptional balance sheet strength and operational leverage.
- **Profitability**: Gross margins are sustained at a robust 42.5%% with an EBITDA margin of 31.8%%.
- **Solvency**: Debt-to-Equity is low at 0.45, backed by an interest coverage ratio exceeding 25x.
- **Liquidity**: Cash and short-term investments total $35B, providing an massive buffer for strategic investments or capital returns.
- **Free Cash Flow**: FCF yield stands at a strong 5.2%%, showcasing superior organic growth self-funding.`, p.Ticker), nil

	case strings.Contains(sys, "bull analyst"):
		var counter string
		if strings.Contains(user, "bear analyst") {
			counter = " While the bear correctly highlights valuation premiums, "
		}
		return fmt.Sprintf(`### Simulated Bull Pitch for %[1]s
We maintain an aggressive conviction overweight rating.%[2]s%[1]s represents a dominant competitive fortress with a massive ecosystem and high switching costs. The structural shift toward high-margin recurring services and recurring subscription streams is accelerating. Margin expansion and solid organic cash generation completely overshadow any temporary macro growth slowing. Buy the breakout.`, p.Ticker, counter), nil

	case strings.Contains(sys, "bear analyst"):
		return fmt.Sprintf(`### Simulated Bear Counter-Pitch for %[1]s
We advise extreme caution and maintain an underweight/hedged posture. The bull's thesis relies on continued valuation expansion which is unsustainable at a forward PE exceeding 32x. Regulatory antitrust headwinds in both EU and US present an under-appreciated risk of business model breakup. Additionally, consumer spending elasticity is weakening, which could trigger immediate hardware average selling price compression. Protect capital.`, p.Ticker), nil

	case strings.Contains(sys, "aggressive risk"):
		return "Aggressive appetite feedback: Proposal sizing is too conservative! Technical momentum is robust; we should double the allocation to maximize return on equity.", nil

	case strings.Contains(sys, "conservative risk"):
		return "Conservative appetite feedback: Strong warning! Stop-loss is too wide. In the event of a market-wide reversal, capital drawdowns could exceed acceptable thresholds. Tighten stop-loss by 3% and cut position size by half.", nil

	case strings.Contains(sys, "neutral risk"):
		return "Neutral risk feedback: The entry target is logical, but we must stick to strict risk-parity constraints. Maintain the standard sizing with no exceptions.", nil

	default:
		return "Simulated general analyst response: Solid operational alignment and stable trend projections.", nil
	}
}

// GenerateStructured implements LLMProvider.
func (p *MockProvider) GenerateStructured(ctx context.Context, req LLMRequest, target interface{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(150 * time.Millisecond):
	}

	sys := strings.ToLower(req.SystemPrompt)

	var rawJSON string
	switch {
	case strings.Contains(sys, "portfolio manager"):
		rawJSON = `{"rating": "Overweight", "executive_summary": "Synthesized consensus confirms high-conviction buy signal. Risk adjusted return is highly asymmetric.", "investment_thesis": "Ecosystem strength, robust cash reserves, and bullish golden cross trends justify a solid position.", "price_target": 215.00, "time_horizon": "3-6 months"}`
	case strings.Contains(sys, "research manager"):
		rawJSON = `{"recommendation": "Buy", "rationale": "Technical breakout accompanied by robust insider buying and flawless fundamentals.", "strategic_actions": "Initiate long position on pullbacks to the 10-day EMA."}`
	case strings.Contains(sys, "trader"):
		rawJSON = `{"action": "Buy", "reasoning": "Breakout confirmed on high volume support.", "entry_price": 182.50, "stop_loss": 174.00, "position_sizing": "15%"}`
	default:
		// Generic fallback based on reflection
		val := ConvertTypeToSchema(reflect.TypeOf(target))
		_ = val
		rawJSON = "{}"
	}

	return json.Unmarshal([]byte(rawJSON), target)
}
