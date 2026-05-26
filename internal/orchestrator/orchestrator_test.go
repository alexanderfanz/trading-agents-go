package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"trading-agents-go/internal/checkpoint"
	"trading-agents-go/internal/cli"
	"trading-agents-go/internal/memory"
	"trading-agents-go/pkg/provider"
)

func TestRenderResearchPlan_Golden(t *testing.T) {
	plan := ResearchPlan{
		Recommendation:   "Buy",
		Rationale:        "Strong momentum",
		StrategicActions: "Accumulate on dips",
	}
	want := "**Recommendation**: Buy\n\n**Rationale**: Strong momentum\n\n**Strategic Actions**: Accumulate on dips"
	if got := RenderResearchPlan(plan); got != want {
		t.Fatalf("RenderResearchPlan() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderTraderProposal_Golden(t *testing.T) {
	entry := 150.25
	stop := 140.00
	sizing := "10%"
	proposal := TraderProposal{
		Action:         "Buy",
		Reasoning:      "Breakout confirmed",
		EntryPrice:     &entry,
		StopLoss:       &stop,
		PositionSizing: &sizing,
	}
	want := "**Action**: Buy\n\n**Reasoning**: Breakout confirmed\n\n**Entry Price**: 150.25\n\n**Stop Loss**: 140.00\n\n**Position Sizing**: 10%\n\nFINAL TRANSACTION PROPOSAL: **BUY**"
	if got := RenderTraderProposal(proposal); got != want {
		t.Fatalf("RenderTraderProposal() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderPMDecision_Golden(t *testing.T) {
	target := 200.00
	horizon := "6 months"
	decision := PortfolioDecision{
		Rating:           "Overweight",
		ExecutiveSummary: "High conviction",
		InvestmentThesis: "Ecosystem moat",
		PriceTarget:      &target,
		TimeHorizon:      &horizon,
	}
	want := "**Rating**: Overweight\n\n**Executive Summary**: High conviction\n\n**Investment Thesis**: Ecosystem moat\n\n**Price Target**: 200.00\n\n**Time Horizon**: 6 months"
	if got := RenderPMDecision(decision); got != want {
		t.Fatalf("RenderPMDecision() =\n%q\nwant\n%q", got, want)
	}
}

func TestAnalystPanicError(t *testing.T) {
	err := &AnalystPanicError{
		AnalystName: "Market",
		PanicValue:  "boom",
		StackTrace:  "stack-trace",
		Timestamp:   time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
	}
	msg := err.Error()
	if !strings.Contains(msg, "Market") || !strings.Contains(msg, "boom") || !strings.Contains(msg, "stack-trace") {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestRunConcurrentAnalysts_AllSucceed(t *testing.T) {
	mockLLM := &behavioralMockLLM{}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()

	summary, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err != nil {
		t.Fatalf("RunConcurrentAnalysts failed: %v", err)
	}
	if !strings.Contains(summary, "Analyst Run Summary") {
		t.Fatalf("expected summary header, got: %q", summary)
	}

	state.RLock()
	reports := state.AnalystReports
	state.RUnlock()
	if len(reports) != 4 {
		t.Fatalf("expected 4 analyst reports, got %d", len(reports))
	}
	for _, name := range []string{"Market", "Sentiment", "News", "Fundamentals"} {
		if reports[name] == "" {
			t.Fatalf("missing report for %s", name)
		}
	}
}

func TestRunConcurrentAnalysts_PartialSuccess(t *testing.T) {
	mockLLM := &behavioralMockLLM{
		roles: map[string]roleBehavior{
			"news":         {err: errors.New("news analyst unavailable")},
			"fundamentals": {err: errors.New("fundamentals analyst unavailable")},
		},
	}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()

	summary, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err != nil {
		t.Fatalf("expected partial success, got error: %v", err)
	}
	if !strings.Contains(summary, "Analyst Error") {
		t.Fatalf("expected error lines in summary, got: %q", summary)
	}

	state.RLock()
	reports := state.AnalystReports
	state.RUnlock()
	if len(reports) != 2 {
		t.Fatalf("expected 2 successful reports, got %d", len(reports))
	}
}

func TestRunConcurrentAnalysts_CriticalFailure(t *testing.T) {
	mockLLM := &behavioralMockLLM{
		roles: map[string]roleBehavior{
			"market":       {err: errors.New("market down")},
			"sentiment":    {err: errors.New("sentiment down")},
			"news":         {err: errors.New("news down")},
			"fundamentals": {err: errors.New("fundamentals down")},
		},
	}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()

	_, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err == nil {
		t.Fatal("expected critical failure error")
	}
	if !strings.Contains(err.Error(), "critical pipeline failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConcurrentAnalysts_PanicRecovery(t *testing.T) {
	mockLLM := &behavioralMockLLM{
		roles: map[string]roleBehavior{
			"news": {panic: true},
		},
	}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()

	summary, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err != nil {
		t.Fatalf("expected recovery with 3/4 analysts, got: %v", err)
	}
	if !strings.Contains(summary, "News Analyst") || !strings.Contains(summary, "Panicked") {
		t.Fatalf("expected panic recovery line in summary, got: %q", summary)
	}

	state.RLock()
	reports := state.AnalystReports
	state.RUnlock()
	if len(reports) != 3 {
		t.Fatalf("expected 3 reports after panic, got %d", len(reports))
	}
}

func TestRunConcurrentAnalysts_InvalidTradeDate(t *testing.T) {
	orch := newTestOrchestrator(&behavioralMockLLM{}, nil, nil, 1, 1)
	state := testTradingState()
	state.TradeDate = "not-a-date"

	_, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err == nil || !strings.Contains(err.Error(), "invalid trade date") {
		t.Fatalf("expected invalid trade date error, got: %v", err)
	}
}

func TestRunResearchDebate_StructuredOutput(t *testing.T) {
	mockLLM := &behavioralMockLLM{}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{
		"Market":       "market report",
		"Sentiment":    "sentiment report",
		"News":         "news report",
		"Fundamentals": "fundamentals report",
	}

	rendered, cliState, err := orch.RunResearchDebate(context.Background(), state)
	if err != nil {
		t.Fatalf("RunResearchDebate failed: %v", err)
	}
	if cliState != cli.StateBullish {
		t.Fatalf("cliState = %v, want bullish", cliState)
	}
	want := "**Recommendation**: Buy\n\n**Rationale**: Strong fundamentals\n\n**Strategic Actions**: Accumulate on dips"
	if rendered != want {
		t.Fatalf("rendered plan =\n%q\nwant\n%q", rendered, want)
	}

	state.RLock()
	defer state.RUnlock()
	if state.InvestmentPlan != want {
		t.Fatalf("state.InvestmentPlan mismatch")
	}
	if !strings.Contains(state.InvestmentDebate.History, "Bull Analyst:") {
		t.Fatal("expected bull debate in history")
	}
	if !strings.Contains(state.InvestmentDebate.History, "Bear Analyst:") {
		t.Fatal("expected bear debate in history")
	}
}

func TestRunResearchDebate_StructuredFallback(t *testing.T) {
	mockLLM := &behavioralMockLLM{structuredErr: errors.New("json parse failure")}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{
		"Market": "m", "Sentiment": "s", "News": "n", "Fundamentals": "f",
	}

	rendered, cliState, err := orch.RunResearchDebate(context.Background(), state)
	if err != nil {
		t.Fatalf("RunResearchDebate failed: %v", err)
	}
	if cliState != cli.StateNeutral {
		t.Fatalf("fallback Hold should be neutral, got %v", cliState)
	}
	if !strings.Contains(rendered, memory.HoldConst) {
		t.Fatalf("expected Hold fallback in rendered plan: %q", rendered)
	}
	if !strings.Contains(rendered, "Fallback synthesis") {
		t.Fatalf("expected fallback rationale: %q", rendered)
	}
}

func TestRunResearchDebate_MaxDebateRounds(t *testing.T) {
	mockLLM := &behavioralMockLLM{}
	orch := newTestOrchestrator(mockLLM, nil, nil, 2, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{
		"Market": "m", "Sentiment": "s", "News": "n", "Fundamentals": "f",
	}

	_, _, err := orch.RunResearchDebate(context.Background(), state)
	if err != nil {
		t.Fatalf("RunResearchDebate failed: %v", err)
	}

	// 2 rounds => 2 bull + 2 bear Generate calls before manager structured call.
	if mockLLM.generateCalls != 4 {
		t.Fatalf("expected 4 generate calls for 2 debate rounds, got %d", mockLLM.generateCalls)
	}
	if mockLLM.structuredCalls != 1 {
		t.Fatalf("expected 1 structured call, got %d", mockLLM.structuredCalls)
	}

	state.RLock()
	count := state.InvestmentDebate.Count
	state.RUnlock()
	if count != 4 {
		t.Fatalf("debate count = %d, want 4", count)
	}
}

func TestRunRiskAndSizing_FullWorkflow(t *testing.T) {
	mockLLM := &behavioralMockLLM{}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{"Market": "market", "Fundamentals": "fundamentals"}
	state.InvestmentPlan = "Buy AAPL"
	state.Metadata["past_context"] = "Prior decision: Hold due to macro uncertainty."

	rendered, cliState, err := orch.RunRiskAndSizing(context.Background(), state)
	if err != nil {
		t.Fatalf("RunRiskAndSizing failed: %v", err)
	}
	if cliState != cli.StateBullish {
		t.Fatalf("cliState = %v, want bullish for Overweight", cliState)
	}
	if !strings.Contains(rendered, "**Rating**: Overweight") {
		t.Fatalf("unexpected PM decision render: %q", rendered)
	}

	state.RLock()
	defer state.RUnlock()

	if state.TraderInvestmentPlan == "" {
		t.Fatal("expected trader investment plan to be set")
	}
	if state.OptionsStrategy == "" {
		t.Fatal("expected options strategy to be set")
	}
	if !strings.Contains(state.RiskDebate.History, "Aggressive Risk:") {
		t.Fatal("expected aggressive risk in debate history")
	}
	if !strings.Contains(state.RiskDebate.History, "Conservative Risk:") {
		t.Fatal("expected conservative risk in debate history")
	}
	if !strings.Contains(state.RiskDebate.History, "Neutral Risk:") {
		t.Fatal("expected neutral risk in debate history")
	}
	if state.FinalTradeDecision != rendered {
		t.Fatal("final trade decision should match rendered PM output")
	}
	if len(state.SignalLogs) != 1 {
		t.Fatalf("expected 1 signal log entry, got %d", len(state.SignalLogs))
	}
	if state.SignalLogs[0].Action != "Overweight" {
		t.Fatalf("signal action = %q", state.SignalLogs[0].Action)
	}
	if mockLLM.lastUserPrompt == "" || !strings.Contains(mockLLM.lastUserPrompt, "Prior decision: Hold") {
		t.Fatal("PM prompt should include past_context from journal metadata")
	}
}

func TestRunRiskAndSizing_StructuredFallbacks(t *testing.T) {
	mockLLM := &behavioralMockLLM{structuredErr: errors.New("structured failure")}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{"Market": "m", "Fundamentals": "f"}
	state.InvestmentPlan = "Hold"

	rendered, cliState, err := orch.RunRiskAndSizing(context.Background(), state)
	if err != nil {
		t.Fatalf("RunRiskAndSizing failed: %v", err)
	}
	if cliState != cli.StateNeutral {
		t.Fatalf("fallback should be neutral, got %v", cliState)
	}
	if !strings.Contains(rendered, memory.HoldConst) {
		t.Fatalf("expected Hold fallback in PM decision: %q", rendered)
	}
	if !strings.Contains(state.TraderInvestmentPlan, memory.HoldConst) {
		t.Fatalf("expected Hold fallback trader proposal: %q", state.TraderInvestmentPlan)
	}
}

func TestRunRiskAndSizing_MaxRiskDiscussRounds(t *testing.T) {
	mockLLM := &behavioralMockLLM{}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 2)
	state := testTradingState()
	state.AnalystReports = map[string]string{"Market": "m", "Fundamentals": "f"}
	state.InvestmentPlan = "Buy"

	_, _, err := orch.RunRiskAndSizing(context.Background(), state)
	if err != nil {
		t.Fatalf("RunRiskAndSizing failed: %v", err)
	}

	state.RLock()
	count := state.RiskDebate.Count
	agg := len(state.AggressiveRiskHistory)
	con := len(state.ConservativeRiskHistory)
	neu := len(state.NeutralRiskHistory)
	state.RUnlock()

	if count != 6 {
		t.Fatalf("risk debate count = %d, want 6 (3 roles x 2 rounds)", count)
	}
	if agg != 2 || con != 2 || neu != 2 {
		t.Fatalf("history lengths agg=%d con=%d neu=%d, want 2 each", agg, con, neu)
	}
}

func TestRunResearchDebate_BullFailure(t *testing.T) {
	mockLLM := &behavioralMockLLM{
		roles: map[string]roleBehavior{
			"bull": {err: errors.New("bull unavailable")},
		},
	}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{"Market": "m", "Sentiment": "s", "News": "n", "Fundamentals": "f"}

	_, _, err := orch.RunResearchDebate(context.Background(), state)
	if err == nil || !strings.Contains(err.Error(), "bull analyst failed") {
		t.Fatalf("expected bull failure, got: %v", err)
	}
}

func TestNewTradingOrchestrator(t *testing.T) {
	cfg := configWithDebateRounds(1, 1)
	llm := provider.NewMockProvider("AAPL")
	orch := NewTradingOrchestrator(cfg, nil, &mockDataProvider{}, llm, nil, &mockNewsSocialProvider{})
	if orch == nil || orch.cfg != cfg || orch.llmProvider != llm {
		t.Fatal("NewTradingOrchestrator did not wire dependencies")
	}
}

func TestRunConcurrentAnalysts_OHLCVFailure(t *testing.T) {
	data := &mockDataProvider{ohlcvErr: errors.New("no data")}
	orch := newTestOrchestrator(&behavioralMockLLM{}, data, nil, 1, 1)
	state := testTradingState()

	_, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err == nil || !strings.Contains(err.Error(), "failed to fetch OHLCV") {
		t.Fatalf("expected OHLCV error, got: %v", err)
	}
}

func TestRunConcurrentAnalysts_NewsFetchWarnings(t *testing.T) {
	news := &mockNewsSocialProvider{
		newsErr:       errors.New("news down"),
		globalNewsErr: errors.New("global down"),
		stocktwitsErr: errors.New("st down"),
		redditErr:     errors.New("reddit down"),
	}
	orch := newTestOrchestrator(&behavioralMockLLM{}, nil, news, 1, 1)
	state := testTradingState()

	summary, err := orch.RunConcurrentAnalysts(context.Background(), state)
	if err != nil {
		t.Fatalf("news fetch warnings should not abort pipeline: %v", err)
	}
	if !strings.Contains(summary, "Completed successfully") {
		t.Fatalf("expected successful analysts in summary: %q", summary)
	}
}

func TestRunResearchDebate_BearishCLIState(t *testing.T) {
	mockLLM := &behavioralMockLLM{
		structuredFill: func(_ provider.LLMRequest, target interface{}) error {
			plan, ok := target.(*ResearchPlan)
			if !ok {
				t.Fatal("expected ResearchPlan target")
			}
			*plan = ResearchPlan{Recommendation: "Sell", Rationale: "Overvalued", StrategicActions: "Reduce exposure"}
			return nil
		},
	}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{"Market": "m", "Sentiment": "s", "News": "n", "Fundamentals": "f"}

	_, cliState, err := orch.RunResearchDebate(context.Background(), state)
	if err != nil {
		t.Fatalf("RunResearchDebate failed: %v", err)
	}
	if cliState != cli.StateBearish {
		t.Fatalf("cliState = %v, want bearish", cliState)
	}
}

func TestRunRiskAndSizing_OptionsFailureFallback(t *testing.T) {
	mockLLM := &behavioralMockLLM{
		roles: map[string]roleBehavior{
			"options": {err: errors.New("options agent down")},
		},
	}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{"Market": "m", "Fundamentals": "f"}
	state.InvestmentPlan = "Buy"

	_, _, err := orch.RunRiskAndSizing(context.Background(), state)
	if err != nil {
		t.Fatalf("options failure should not abort risk sizing: %v", err)
	}

	state.RLock()
	opts := state.OptionsStrategy
	state.RUnlock()
	if !strings.Contains(opts, "Fallback options strategy") {
		t.Fatalf("expected options fallback, got: %q", opts)
	}
}

func TestRunRiskAndSizing_RiskAgentFailure(t *testing.T) {
	mockLLM := &behavioralMockLLM{
		roles: map[string]roleBehavior{
			"aggressive": {err: errors.New("aggressive risk down")},
		},
	}
	orch := newTestOrchestrator(mockLLM, nil, nil, 1, 1)
	state := testTradingState()
	state.AnalystReports = map[string]string{"Market": "m", "Fundamentals": "f"}
	state.InvestmentPlan = "Buy"

	_, _, err := orch.RunRiskAndSizing(context.Background(), state)
	if err == nil {
		t.Fatal("expected aggressive risk failure to propagate")
	}
}

func TestAddDebateMessage_WithExistingHistory(t *testing.T) {
	state := &checkpoint.TradingState{
		InvestmentDebate: InvestDebateState{History: "### Starting Debate Room\n"},
	}
	AddDebateMessage(state, "Bull Analyst", "opening")
	state.RLock()
	got := state.InvestmentDebate.History
	state.RUnlock()
	if !strings.Contains(got, "Bull Analyst: opening") {
		t.Fatalf("history = %q", got)
	}
}
