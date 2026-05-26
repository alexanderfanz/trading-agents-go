package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"trading-agents-go/internal/cli"
	"trading-agents-go/internal/config"
	"trading-agents-go/internal/dataflow"
	"trading-agents-go/internal/indicators"
	"trading-agents-go/pkg/provider"
)

type stubDataProvider struct {
	candles      []dataflow.Candle
	fundamentals string
	ohlcvErr     error
	fundErr      error
}

func (s *stubDataProvider) FetchOHLCV(ctx context.Context, ticker string, start, end, tradeDate time.Time) ([]dataflow.Candle, error) {
	if s.ohlcvErr != nil {
		return nil, s.ohlcvErr
	}
	return s.candles, nil
}

func (s *stubDataProvider) FetchFundamentals(ctx context.Context, ticker string, tradeDate time.Time) (string, error) {
	if s.fundErr != nil {
		return "", s.fundErr
	}
	return s.fundamentals, nil
}

type stubNewsSocial struct {
	news        string
	globalNews  string
	stocktwits  string
	reddit      string
	newsErr     error
	globalErr   error
	stocktwitsErr error
	redditErr   error
}

func (s *stubNewsSocial) FetchNews(ctx context.Context, ticker string, start, end time.Time) (string, error) {
	if s.newsErr != nil {
		return "", s.newsErr
	}
	return s.news, nil
}

func (s *stubNewsSocial) FetchGlobalNews(ctx context.Context, currDate time.Time, lookBackDays, limit int) (string, error) {
	if s.globalErr != nil {
		return "", s.globalErr
	}
	return s.globalNews, nil
}

func (s *stubNewsSocial) FetchStockTwits(ctx context.Context, ticker string, limit int) (string, error) {
	if s.stocktwitsErr != nil {
		return "", s.stocktwitsErr
	}
	return s.stocktwits, nil
}

func (s *stubNewsSocial) FetchReddit(ctx context.Context, ticker string, subreddits []string, limitPerSub int) (string, error) {
	if s.redditErr != nil {
		return "", s.redditErr
	}
	return s.reddit, nil
}

type failingLLM struct{}

func (f *failingLLM) Generate(context.Context, provider.LLMRequest) (string, error) {
	return "", errors.New("forced LLM failure")
}

func (f *failingLLM) GenerateStructured(context.Context, provider.LLMRequest, interface{}) error {
	return errors.New("forced LLM failure")
}

func buildTestCandles(tradeDate time.Time, count int) []dataflow.Candle {
	candles := make([]dataflow.Candle, 0, count)
	for i := count; i >= 1; i-- {
		d := tradeDate.AddDate(0, 0, -i)
		candles = append(candles, dataflow.Candle{
			Time:   d,
			Open:   150,
			High:   155,
			Low:    149,
			Close:  152,
			Volume: 1_000_000,
		})
	}
	return candles
}

func testOrchestrator(t *testing.T, llm provider.LLMProvider) *TradingOrchestrator {
	t.Helper()
	cfg := &config.Config{
		MaxDebateRounds:      1,
		MaxRiskDiscussRounds: 1,
		OutputLanguage:       "English",
	}
	cache := indicators.NewIndicatorCache()
	resolver := indicators.NewDynamicIndicatorResolver(cache)
	return NewTradingOrchestrator(cfg, nil, &stubDataProvider{}, llm, resolver, &stubNewsSocial{})
}

func TestRenderResearchPlan(t *testing.T) {
	got := RenderResearchPlan(ResearchPlan{
		Recommendation:   "Buy",
		Rationale:        "Strong momentum",
		StrategicActions: "Scale in on dips",
	})
	for _, want := range []string{"**Recommendation**: Buy", "**Rationale**: Strong momentum", "**Strategic Actions**: Scale in on dips"} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderResearchPlan missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTraderProposal(t *testing.T) {
	entry := 182.5
	stop := 174.0
	size := "15%"
	got := RenderTraderProposal(TraderProposal{
		Action:         "Buy",
		Reasoning:      "Breakout confirmed",
		EntryPrice:     &entry,
		StopLoss:       &stop,
		PositionSizing: &size,
	})
	for _, want := range []string{
		"**Action**: Buy",
		"**Entry Price**: 182.50",
		"**Stop Loss**: 174.00",
		"**Position Sizing**: 15%",
		"FINAL TRANSACTION PROPOSAL: **BUY**",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderTraderProposal missing %q:\n%s", want, got)
		}
	}
}

func TestRenderPMDecision(t *testing.T) {
	target := 215.0
	horizon := "3-6 months"
	got := RenderPMDecision(PortfolioDecision{
		Rating:           "Overweight",
		ExecutiveSummary: "High conviction",
		InvestmentThesis: "Ecosystem moat",
		PriceTarget:      &target,
		TimeHorizon:      &horizon,
	})
	for _, want := range []string{
		"**Rating**: Overweight",
		"**Price Target**: 215.00",
		"**Time Horizon**: 3-6 months",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderPMDecision missing %q:\n%s", want, got)
		}
	}
}

func TestAnalystPanicError(t *testing.T) {
	err := &AnalystPanicError{
		AnalystName: "NewsAnalyst",
		PanicValue:  "simulated panic",
		StackTrace:  "goroutine 1",
		Timestamp:   time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC),
	}
	msg := err.Error()
	if !strings.Contains(msg, "NewsAnalyst") || !strings.Contains(msg, "simulated panic") {
		t.Errorf("unexpected error message: %s", msg)
	}
}

func TestSafeReportMap_Concurrent(t *testing.T) {
	m := NewSafeReportMap()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("Analyst%d", i%4)
			m.Store(name, fmt.Sprintf("report-%d", i), time.Duration(i)*time.Millisecond)
		}(i)
	}
	wg.Wait()

	reports := m.GetReports()
	if len(reports) == 0 {
		t.Fatal("expected reports after concurrent stores")
	}
	latencies := m.GetLatencies()
	if len(latencies) != len(reports) {
		t.Fatalf("latency map size %d != report map size %d", len(latencies), len(reports))
	}
}

func TestAddDebateMessage(t *testing.T) {
	state := &TradingState{
		InvestmentDebate: InvestDebateState{History: "### Starting Debate Room\n"},
	}
	AddDebateMessage(state, "Bull Analyst", "Growth thesis holds.")
	AddDebateMessage(state, "Bear Analyst", "Valuation is stretched.")

	if state.InvestmentDebate.Count != 2 {
		t.Fatalf("expected debate count 2, got %d", state.InvestmentDebate.Count)
	}
	if !strings.Contains(state.InvestmentDebate.History, "Bull Analyst: Growth thesis holds.") {
		t.Errorf("missing bull message: %s", state.InvestmentDebate.History)
	}
	if !strings.Contains(state.InvestmentDebate.History, "Bear Analyst: Valuation is stretched.") {
		t.Errorf("missing bear message: %s", state.InvestmentDebate.History)
	}
}

func TestRunConcurrentAnalysts_Success(t *testing.T) {
	tradeDate := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	orch := testOrchestrator(t, provider.NewMockProvider("AAPL"))
	orch.dataProvider = &stubDataProvider{
		candles:      buildTestCandles(tradeDate, 250),
		fundamentals: "Revenue (TTM): 380000.00",
	}
	orch.newsSocialProvider = &stubNewsSocial{
		news:       "Apple launches product",
		globalNews: "Fed holds rates steady",
		stocktwits: "Bullish: 3 (75%)",
		reddit:     "r/stocks — 2 recent posts",
	}

	state := &TradingState{Ticker: "AAPL", TradeDate: "2026-05-25", AnalystReports: make(map[string]string)}
	summary, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err != nil {
		t.Fatalf("RunConcurrentAnalysts: %v", err)
	}
	if !strings.Contains(summary, "Analyst Run Summary") {
		t.Errorf("expected summary header, got: %s", summary)
	}
	if len(state.AnalystReports) < 2 {
		t.Fatalf("expected at least 2 analyst reports, got %d", len(state.AnalystReports))
	}
	for _, name := range []string{"Market", "Sentiment", "News", "Fundamentals"} {
		if _, ok := state.AnalystReports[name]; !ok {
			t.Errorf("missing report for %s", name)
		}
	}
}

func TestRunConcurrentAnalysts_CriticalFailure(t *testing.T) {
	tradeDate := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	orch := testOrchestrator(t, &failingLLM{})
	orch.dataProvider = &stubDataProvider{
		candles:      buildTestCandles(tradeDate, 250),
		fundamentals: "Revenue (TTM): 380000.00",
	}
	orch.newsSocialProvider = &stubNewsSocial{news: "headline"}

	state := &TradingState{Ticker: "AAPL", TradeDate: "2026-05-25", AnalystReports: make(map[string]string)}
	_, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err == nil {
		t.Fatal("expected critical pipeline failure")
	}
	if !strings.Contains(err.Error(), "critical pipeline failure") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunConcurrentAnalysts_InvalidTradeDate(t *testing.T) {
	orch := testOrchestrator(t, provider.NewMockProvider("AAPL"))
	state := &TradingState{Ticker: "AAPL", TradeDate: "not-a-date"}
	_, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err == nil || !strings.Contains(err.Error(), "invalid trade date") {
		t.Fatalf("expected invalid trade date error, got: %v", err)
	}
}

func TestRunResearchDebate(t *testing.T) {
	orch := testOrchestrator(t, provider.NewMockProvider("AAPL"))
	state := &TradingState{
		Ticker:    "AAPL",
		TradeDate: "2026-05-25",
		AnalystReports: map[string]string{
			"Market":       "bullish technicals",
			"Sentiment":    "positive retail flow",
			"News":         "product catalyst",
			"Fundamentals": "strong margins",
		},
		InvestmentDebate: InvestDebateState{},
	}

	rendered, cliState, err := orch.RunResearchDebate(context.Background(), state)
	if err != nil {
		t.Fatalf("RunResearchDebate: %v", err)
	}
	if !strings.Contains(rendered, "**Recommendation**: Buy") {
		t.Errorf("expected rendered buy plan, got:\n%s", rendered)
	}
	if cliState != cli.StateBullish {
		t.Errorf("expected bullish CLI state, got %v", cliState)
	}
	if state.InvestmentPlan == "" {
		t.Fatal("expected investment plan on state")
	}
	if len(state.BullDebateHistory) != 1 || len(state.BearDebateHistory) != 1 {
		t.Fatalf("expected 1 bull and 1 bear round, got bull=%d bear=%d",
			len(state.BullDebateHistory), len(state.BearDebateHistory))
	}
}

func TestRunRiskAndSizing(t *testing.T) {
	orch := testOrchestrator(t, provider.NewMockProvider("AAPL"))
	state := &TradingState{
		Ticker:    "AAPL",
		TradeDate: "2026-05-25",
		AnalystReports: map[string]string{
			"Market":       "bullish technicals",
			"Fundamentals": "strong margins",
		},
		InvestmentPlan: "**Recommendation**: Buy",
		RiskDebate:     RiskDebateState{},
		Metadata:       map[string]string{"past_context": "Prior decision: Hold — outcome pending"},
	}

	rendered, cliState, err := orch.RunRiskAndSizing(context.Background(), state)
	if err != nil {
		t.Fatalf("RunRiskAndSizing: %v", err)
	}
	if !strings.Contains(rendered, "**Rating**: Overweight") {
		t.Errorf("expected PM decision in output, got:\n%s", rendered)
	}
	if cliState != cli.StateBullish {
		t.Errorf("expected bullish CLI state, got %v", cliState)
	}
	if state.FinalTradeDecision == "" {
		t.Fatal("expected final trade decision on state")
	}
	if state.TraderInvestmentPlan == "" || state.OptionsStrategy == "" {
		t.Fatal("expected trader and options outputs on state")
	}
	if len(state.AggressiveRiskHistory) != 1 || len(state.ConservativeRiskHistory) != 1 || len(state.NeutralRiskHistory) != 1 {
		t.Fatalf("expected one round of each risk analyst, got agg=%d con=%d neu=%d",
			len(state.AggressiveRiskHistory), len(state.ConservativeRiskHistory), len(state.NeutralRiskHistory))
	}
	if len(state.SignalLogs) != 1 {
		t.Fatalf("expected one signal log entry, got %d", len(state.SignalLogs))
	}
}

func TestRunResearchDebate_StructuredFallback(t *testing.T) {
	orch := testOrchestrator(t, &failingLLM{})
	state := &TradingState{
		Ticker:    "AAPL",
		TradeDate: "2026-05-25",
		AnalystReports: map[string]string{
			"Market": "x", "Sentiment": "x", "News": "x", "Fundamentals": "x",
		},
		InvestmentDebate: InvestDebateState{},
	}
	_, _, err := orch.RunResearchDebate(context.Background(), state)
	if err == nil {
		t.Fatal("expected debate failure when all LLM calls fail")
	}
}

// Ensure stub types satisfy interfaces at compile time.
var (
	_ dataflow.DataProvider      = (*stubDataProvider)(nil)
	_ dataflow.NewsSocialProvider = (*stubNewsSocial)(nil)
	_ provider.LLMProvider       = (*failingLLM)(nil)
)
