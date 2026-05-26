package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"trading-agents-go/internal/checkpoint"
	"trading-agents-go/internal/config"
	"trading-agents-go/internal/dataflow"
	"trading-agents-go/internal/indicators"
	"trading-agents-go/pkg/provider"
)

type mockDataProvider struct {
	candles      []dataflow.Candle
	fundamentals string
	ohlcvErr     error
	fundErr      error
}

func (m *mockDataProvider) FetchOHLCV(_ context.Context, _ string, _, _ time.Time, _ time.Time) ([]dataflow.Candle, error) {
	if m.ohlcvErr != nil {
		return nil, m.ohlcvErr
	}
	return m.candles, nil
}

func (m *mockDataProvider) FetchFundamentals(_ context.Context, _ string, _ time.Time) (string, error) {
	if m.fundErr != nil {
		return "", m.fundErr
	}
	if m.fundamentals != "" {
		return m.fundamentals, nil
	}
	return "Revenue: 100B\nPE: 25", nil
}

type mockNewsSocialProvider struct {
	newsErr       error
	globalNewsErr error
	stocktwitsErr error
	redditErr     error
	newsBlock     string
	globalNews    string
	stocktwits    string
	reddit        string
}

func (m *mockNewsSocialProvider) FetchNews(_ context.Context, _ string, _, _ time.Time) (string, error) {
	if m.newsErr != nil {
		return "", m.newsErr
	}
	if m.newsBlock != "" {
		return m.newsBlock, nil
	}
	return "Mock corporate news", nil
}

func (m *mockNewsSocialProvider) FetchGlobalNews(_ context.Context, _ time.Time, _, _ int) (string, error) {
	if m.globalNewsErr != nil {
		return "", m.globalNewsErr
	}
	if m.globalNews != "" {
		return m.globalNews, nil
	}
	return "Mock global news", nil
}

func (m *mockNewsSocialProvider) FetchStockTwits(_ context.Context, _ string, _ int) (string, error) {
	if m.stocktwitsErr != nil {
		return "", m.stocktwitsErr
	}
	if m.stocktwits != "" {
		return m.stocktwits, nil
	}
	return "Mock stocktwits feed", nil
}

func (m *mockNewsSocialProvider) FetchReddit(_ context.Context, _ string, _ []string, _ int) (string, error) {
	if m.redditErr != nil {
		return "", m.redditErr
	}
	if m.reddit != "" {
		return m.reddit, nil
	}
	return "Mock reddit posts", nil
}

type roleBehavior struct {
	response string
	err      error
	panic    bool
}

type behavioralMockLLM struct {
	mu               sync.Mutex
	roles            map[string]roleBehavior
	structuredErr    error
	structuredFill   func(req provider.LLMRequest, target interface{}) error
	generateCalls    int
	structuredCalls  int
	lastSystemPrompt string
	lastUserPrompt   string
}

func (m *behavioralMockLLM) roleFromPrompt(systemPrompt string) string {
	lower := strings.ToLower(systemPrompt)
	switch {
	case strings.Contains(lower, "market analyst"):
		return "market"
	case strings.Contains(lower, "sentiment analyst"):
		return "sentiment"
	case strings.Contains(lower, "news analyst"):
		return "news"
	case strings.Contains(lower, "fundamentals analyst"):
		return "fundamentals"
	case strings.Contains(lower, "bull analyst"):
		return "bull"
	case strings.Contains(lower, "bear analyst"):
		return "bear"
	case strings.Contains(lower, "research manager"):
		return "research_manager"
	case strings.Contains(lower, "portfolio manager"):
		return "portfolio_manager"
	case strings.Contains(lower, "options strategist"):
		return "options"
	case strings.Contains(lower, "aggressive risk"):
		return "aggressive"
	case strings.Contains(lower, "conservative risk"):
		return "conservative"
	case strings.Contains(lower, "neutral risk"):
		return "neutral"
	case strings.Contains(lower, "trader"):
		return "trader"
	default:
		return "default"
	}
}

func (m *behavioralMockLLM) Generate(_ context.Context, req provider.LLMRequest) (string, error) {
	m.mu.Lock()
	m.generateCalls++
	m.lastSystemPrompt = req.SystemPrompt
	m.lastUserPrompt = req.UserPrompt
	m.mu.Unlock()

	role := m.roleFromPrompt(req.SystemPrompt)
	if behavior, ok := m.roles[role]; ok {
		if behavior.panic {
			panic("simulated analyst panic")
		}
		if behavior.err != nil {
			return "", behavior.err
		}
		if behavior.response != "" {
			return behavior.response, nil
		}
	}
	return "Mock analyst report for " + role, nil
}

func (m *behavioralMockLLM) GenerateStructured(_ context.Context, req provider.LLMRequest, target interface{}) error {
	m.mu.Lock()
	m.structuredCalls++
	m.lastSystemPrompt = req.SystemPrompt
	m.lastUserPrompt = req.UserPrompt
	structuredErr := m.structuredErr
	structuredFill := m.structuredFill
	m.mu.Unlock()

	if structuredErr != nil {
		return structuredErr
	}
	if structuredFill != nil {
		return structuredFill(req, target)
	}

	role := m.roleFromPrompt(req.SystemPrompt)
	var raw string
	switch role {
	case "research_manager":
		raw = `{"recommendation":"Buy","rationale":"Strong fundamentals","strategic_actions":"Accumulate on dips"}`
	case "trader":
		raw = `{"action":"Buy","reasoning":"Momentum breakout","entry_price":150.25,"stop_loss":140.00,"position_sizing":"10%"}`
	case "portfolio_manager":
		raw = `{"rating":"Overweight","executive_summary":"High conviction buy","investment_thesis":"Ecosystem moat","price_target":200.00,"time_horizon":"6 months"}`
	default:
		return errors.New("unexpected structured call for role: " + role)
	}
	return json.Unmarshal([]byte(raw), target)
}

func sampleCandles(count int, end time.Time) []dataflow.Candle {
	candles := make([]dataflow.Candle, count)
	for i := 0; i < count; i++ {
		close := 100.0 + float64(i)*0.5
		candles[i] = dataflow.Candle{
			Time:   end.AddDate(0, 0, -(count - 1 - i)),
			Open:   close - 1,
			High:   close + 2,
			Low:    close - 2,
			Close:  close,
			Volume: 1_000_000,
		}
	}
	return candles
}

func newTestOrchestrator(llm provider.LLMProvider, data *mockDataProvider, news *mockNewsSocialProvider, maxDebate, maxRisk int) *TradingOrchestrator {
	if data == nil {
		data = &mockDataProvider{candles: sampleCandles(60, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))}
	}
	if news == nil {
		news = &mockNewsSocialProvider{}
	}
	cfg := configWithDebateRounds(maxDebate, maxRisk)
	return NewTradingOrchestrator(
		cfg,
		nil,
		data,
		llm,
		indicators.NewDynamicIndicatorResolver(indicators.NewIndicatorCache()),
		news,
	)
}

func configWithDebateRounds(maxDebate, maxRisk int) *config.Config {
	cfg := &config.Config{
		MaxDebateRounds:      maxDebate,
		MaxRiskDiscussRounds: maxRisk,
	}
	if maxDebate == 0 {
		cfg.MaxDebateRounds = 1
	}
	if maxRisk == 0 {
		cfg.MaxRiskDiscussRounds = 1
	}
	return cfg
}

func testTradingState() *checkpoint.TradingState {
	return &checkpoint.TradingState{
		Ticker:         "AAPL",
		TradeDate:      "2024-06-01",
		AnalystReports: make(map[string]string),
		Metadata:       make(map[string]string),
	}
}
