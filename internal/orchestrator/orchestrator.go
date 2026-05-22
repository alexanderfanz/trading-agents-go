package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"trading-agents-go/internal/checkpoint"
	"trading-agents-go/internal/cli"
	"trading-agents-go/internal/config"
	"trading-agents-go/internal/dataflow"
	"trading-agents-go/internal/indicators"
	"trading-agents-go/pkg/provider"
)

// ResearchPlan represents the structured investment plan produced by the Research Manager.
type ResearchPlan struct {
	Recommendation string `json:"recommendation"` // Buy / Overweight / Hold / Underweight / Sell
	Rationale      string `json:"rationale"`
	StrategicActions string `json:"strategic_actions"`
}

// RenderResearchPlan converts the ResearchPlan to a markdown string.
func RenderResearchPlan(plan ResearchPlan) string {
	return fmt.Sprintf("**Recommendation**: %s\n\n**Rationale**: %s\n\n**Strategic Actions**: %s",
		plan.Recommendation, plan.Rationale, plan.StrategicActions)
}

// TraderProposal represents the structured transaction proposal produced by the Trader.
type TraderProposal struct {
	Action         string   `json:"action"` // Buy / Hold / Sell
	Reasoning      string   `json:"reasoning"`
	EntryPrice     *float64 `json:"entry_price,omitempty"`
	StopLoss       *float64 `json:"stop_loss,omitempty"`
	PositionSizing *string  `json:"position_sizing,omitempty"`
}

// RenderTraderProposal converts the TraderProposal to a markdown string.
func RenderTraderProposal(proposal TraderProposal) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("**Action**: %s", proposal.Action))
	parts = append(parts, "")
	parts = append(parts, fmt.Sprintf("**Reasoning**: %s", proposal.Reasoning))
	if proposal.EntryPrice != nil {
		parts = append(parts, "", fmt.Sprintf("**Entry Price**: %.2f", *proposal.EntryPrice))
	}
	if proposal.StopLoss != nil {
		parts = append(parts, "", fmt.Sprintf("**Stop Loss**: %.2f", *proposal.StopLoss))
	}
	if proposal.PositionSizing != nil && *proposal.PositionSizing != "" {
		parts = append(parts, "", fmt.Sprintf("**Position Sizing**: %s", *proposal.PositionSizing))
	}
	parts = append(parts, "", fmt.Sprintf("FINAL TRANSACTION PROPOSAL: **%s**", strings.ToUpper(proposal.Action)))
	return strings.Join(parts, "\n")
}

// PortfolioDecision represents the structured position sizing produced by the Portfolio Manager.
type PortfolioDecision struct {
	Rating           string   `json:"rating"` // Buy / Overweight / Hold / Underweight / Sell
	ExecutiveSummary string   `json:"executive_summary"`
	InvestmentThesis string   `json:"investment_thesis"`
	PriceTarget      *float64 `json:"price_target,omitempty"`
	TimeHorizon      *string  `json:"time_horizon,omitempty"`
}

// RenderPMDecision converts the PortfolioDecision to a markdown string.
func RenderPMDecision(decision PortfolioDecision) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("**Rating**: %s", decision.Rating))
	parts = append(parts, "")
	parts = append(parts, fmt.Sprintf("**Executive Summary**: %s", decision.ExecutiveSummary))
	parts = append(parts, "")
	parts = append(parts, fmt.Sprintf("**Investment Thesis**: %s", decision.InvestmentThesis))
	if decision.PriceTarget != nil {
		parts = append(parts, "", fmt.Sprintf("**Price Target**: %.2f", *decision.PriceTarget))
	}
	if decision.TimeHorizon != nil && *decision.TimeHorizon != "" {
		parts = append(parts, "", fmt.Sprintf("**Time Horizon**: %s", *decision.TimeHorizon))
	}
	return strings.Join(parts, "\n")
}

// AnalystPanicError represents a structured capture of a panicked analyst goroutine.
type AnalystPanicError struct {
	AnalystName string      // Identifier of the failing analyst (e.g., "NewsAnalyst")
	PanicValue  interface{} // The raw interface value returned by recover()
	StackTrace  string      // Captured runtime stack trace using debug.Stack()
	Timestamp   time.Time   // Exact time of the recovery action
}

// Error implements the standard error interface.
func (e *AnalystPanicError) Error() string {
	return fmt.Sprintf("[%s] Panic trapped inside %s: %v\nStack Trace:\n%s",
		e.Timestamp.Format(time.RFC3339), e.AnalystName, e.PanicValue, e.StackTrace)
}

// System Instructions / Prompts for each specialized agent role
const (
	MarketAnalystInstruction = "You are a professional Market Analyst. Your role is to examine the provided indicators and historical price data to write a detailed, highly nuanced trend report. Identify key trends, support levels, and momentum patterns. Support your findings with indicator values. At the end of your report, append a markdown table summarizing key indicators and their sentiment signals."
	
	SentimentAnalystInstruction = "You are a Sentiment Analyst. Evaluate news, Reddit, and StockTwits sentiment for the ticker. Provide a detailed summary of investor expectations and sentiment momentum."
	
	NewsAnalystInstruction = "You are a News Analyst. Analyze recent world affairs, macroeconomic events, and corporate insider transactions. Highlight major catalysts or news-driven risks."
	
	FundamentalsAnalystInstruction = "You are a Fundamentals Analyst. Evaluate the company's financial health, profit margins, balance sheet metrics, and growth trajectory using the provided financial details."
	
	BullInstruction = "You are a Bull Analyst. Build a strong, evidence-based bullish case highlighting growth opportunities, competitive moats, and positive momentum. Engage directly with the bear analyst's counter-arguments."
	
	BearInstruction = "You are a Bear Analyst. Build a strong, evidence-based bearish case highlighting risks, threats, competitive weaknesses, and macro challenges. Counter the bull's points with rigorous evidence."
	
	ResearchManagerInstruction = "You are the Research Manager and debate facilitator. Evaluate the bull/bear debate and produce a structured investment plan in JSON format matching the ResearchPlan schema. Be decisive; commit to Buy/Sell if the strongest arguments warrant it."
	
	TraderInstruction = "You are the Trader. Convert the investment plan and analyst reports into a concrete transaction proposal. Specify stop-loss, entry targets, and position sizing guidelines in JSON format matching the TraderProposal schema."
	
	AggressiveRiskInstruction = "You are the Aggressive Risk analyst. Critique the transaction proposal. Suggest higher sizing if trends support it; look for opportunities to maximize gains."
	
	ConservativeRiskInstruction = "You are the Conservative Risk analyst. Critique the transaction proposal from a defensive standpoint. Recommend capital preservation, tighter stop-losses, and reduced size."
	
	NeutralRiskInstruction = "You are the Neutral Risk analyst. Balance both aggressive and conservative feedback to outline an objective risk-reward profile."
	
	PortfolioManagerInstruction = "You are the Portfolio Manager. Synthesize the risk analysts' debate and the trader proposal. Produce the final position rating (Buy / Overweight / Hold / Underweight / Sell) and sizing thesis in JSON format matching the PortfolioDecision schema."
)

// TradingOrchestrator coordinates the pipeline lifecycle and multi-agent loops.
type TradingOrchestrator struct {
	cfg               *config.Config
	checkpointer      *checkpoint.StateCheckpointer
	dataProvider      dataflow.DataProvider
	llmProvider       provider.LLMProvider
	indicatorResolver *indicators.DynamicIndicatorResolver
}

// NewTradingOrchestrator instantiates a new orchestrator.
func NewTradingOrchestrator(
	cfg *config.Config,
	checkpointer *checkpoint.StateCheckpointer,
	dataProvider dataflow.DataProvider,
	llmProvider provider.LLMProvider,
	indicatorResolver *indicators.DynamicIndicatorResolver,
) *TradingOrchestrator {
	return &TradingOrchestrator{
		cfg:               cfg,
		checkpointer:      checkpointer,
		dataProvider:      dataProvider,
		llmProvider:       llmProvider,
		indicatorResolver: indicatorResolver,
	}
}

// Execute runs the complete strategy execution pipeline with full fault tolerance and checkpoint resumption.
func (o *TradingOrchestrator) Execute(ctx context.Context, ticker string, tradeDate string, cliController *cli.CLIController) (string, error) {
	// 1. Phase A: Setup & Checkpoint Recovery
	state, stepIndex, err := o.checkpointer.Load(ctx, ticker, tradeDate)
	if err != nil {
		return "", fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if stepIndex >= 0 {
		if cliController.IsTTY {
			fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
				fmt.Sprintf("🔄 RESUMING STRATEGY EXECUTION FROM STEP %d FOR %s ON %s", stepIndex, ticker, tradeDate),
			))
		} else {
			fmt.Printf("[INFO] Resuming execution from step %d for %s on %s\n", stepIndex, ticker, tradeDate)
		}
	} else {
		state = &checkpoint.TradingState{
			Ticker:    ticker,
			TradeDate: tradeDate,
			StepIndex: 0,
			Portfolio: checkpoint.PortfolioState{
				Cash:        100000.0,
				TotalEquity: 100000.0,
				Holdings:    make(map[string]float64),
			},
			AnalystReports: make(map[string]string),
			Metadata:       make(map[string]string),
		}
	}

	// 2. Phase B: Concurrent Market Analysis (Step 0)
	if state.StepIndex <= 0 {
		_, err = cliController.RunStep(ctx, "Concurrent Market Analysis", func() (string, cli.CLIState, error) {
			summary, err := o.RunConcurrentAnalysts(ctx, state)
			if err != nil {
				return "", cli.StateNeutral, err
			}
			state.Lock()
			state.StepIndex = 1
			state.Unlock()

			if err := o.checkpointer.Save(ctx, state); err != nil {
				return "", cli.StateNeutral, err
			}
			return summary, cli.StateSystemAction, nil
		})
		if err != nil {
			return "", err
		}
	}

	// 3. Phase C: Research Debate & Consensus (Step 1)
	if state.StepIndex <= 1 {
		_, err = cliController.RunStep(ctx, "Research Debate & Consensus", func() (string, cli.CLIState, error) {
			renderedPlan, cliState, err := o.RunResearchDebate(ctx, state)
			if err != nil {
				return "", cli.StateNeutral, err
			}
			state.Lock()
			state.StepIndex = 2
			state.Unlock()

			if err := o.checkpointer.Save(ctx, state); err != nil {
				return "", cli.StateNeutral, err
			}
			return renderedPlan, cliState, nil
		})
		if err != nil {
			return "", err
		}
	}

	// 4. Phase D: Risk Assessment & Sizing debate (Step 2)
	if state.StepIndex <= 2 {
		_, err = cliController.RunStep(ctx, "Risk Assessment & Position Sizing", func() (string, cli.CLIState, error) {
			renderedDecision, cliState, err := o.RunRiskAndSizing(ctx, state)
			if err != nil {
				return "", cli.StateNeutral, err
			}
			state.Lock()
			state.StepIndex = 3
			state.Unlock()

			if err := o.checkpointer.Save(ctx, state); err != nil {
				return "", cli.StateNeutral, err
			}
			return renderedDecision, cliState, nil
		})
		if err != nil {
			return "", err
		}
	}

	// 5. Phase E: Finalization & Cleanup
	if err := o.checkpointer.Clear(ctx, ticker, tradeDate); err != nil {
		return "", fmt.Errorf("failed to clear checkpoint: %w", err)
	}

	if o.cfg.MemoryLogPath != "" {
		_ = os.MkdirAll(filepath.Dir(o.cfg.MemoryLogPath), 0755)
		f, err := os.OpenFile(o.cfg.MemoryLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			state.RLock()
			decisionLog := fmt.Sprintf("\n## Decision for %s on %s\n%s\n", ticker, tradeDate, state.FinalTradeDecision)
			state.RUnlock()
			_, _ = f.WriteString(decisionLog)
		}
	}

	state.RLock()
	finalDecision := state.FinalTradeDecision
	state.RUnlock()

	return finalDecision, nil
}

// RunConcurrentAnalysts executes the four analyst routines in parallel.
func (o *TradingOrchestrator) RunConcurrentAnalysts(ctx context.Context, state *checkpoint.TradingState) (string, error) {
	tradeDateParsed, err := time.Parse("2006-01-02", state.TradeDate)
	if err != nil {
		return "", fmt.Errorf("invalid trade date format: %w", err)
	}

	start := tradeDateParsed.AddDate(-5, 0, 0)
	candles, err := o.dataProvider.FetchOHLCV(ctx, state.Ticker, start, tradeDateParsed, tradeDateParsed)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OHLCV: %w", err)
	}

	fundamentalsStr, err := o.dataProvider.FetchFundamentals(ctx, state.Ticker, tradeDateParsed)
	if err != nil {
		fundamentalsStr = "Fundamentals data unavailable."
	}

	indicatorsMap, err := o.computeAllIndicators(ctx, candles, state.Ticker, tradeDateParsed)
	if err != nil {
		indicatorsMap = make(map[string]float64)
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var activeErrors []error
	var trappedPanics []*AnalystPanicError
	reportMap := NewSafeReportMap()

	executeSafe := func(analystName string, runFn func() (string, error)) {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				panicErr := &AnalystPanicError{
					AnalystName: analystName,
					PanicValue:  r,
					StackTrace:  stack,
					Timestamp:   time.Now(),
				}
				errMu.Lock()
				trappedPanics = append(trappedPanics, panicErr)
				errMu.Unlock()
			}
		}()

		startTime := time.Now()
		report, err := runFn()
		dur := time.Since(startTime)

		if err != nil {
			errMu.Lock()
			activeErrors = append(activeErrors, err)
			errMu.Unlock()
		} else {
			reportMap.Store(analystName, report, dur)
		}
	}

	wg.Add(4)
	go executeSafe("Market", func() (string, error) {
		return o.runMarketAnalyst(ctx, state, candles, indicatorsMap)
	})
	go executeSafe("Sentiment", func() (string, error) {
		return o.runSentimentAnalyst(ctx, state)
	})
	go executeSafe("News", func() (string, error) {
		return o.runNewsAnalyst(ctx, state)
	})
	go executeSafe("Fundamentals", func() (string, error) {
		return o.runFundamentalsAnalyst(ctx, state, fundamentalsStr)
	})

	wg.Wait()

	successCount := len(reportMap.GetReports())
	if successCount < 2 {
		var combinedErrs []string
		for _, e := range activeErrors {
			combinedErrs = append(combinedErrs, e.Error())
		}
		for _, p := range trappedPanics {
			combinedErrs = append(combinedErrs, p.Error())
		}
		return "", fmt.Errorf("critical pipeline failure: only %d/4 analysts succeeded. Errors: %s", successCount, strings.Join(combinedErrs, "; "))
	}

	state.Lock()
	state.AnalystReports = reportMap.GetReports()
	state.Unlock()

	var summaryParts []string
	summaryParts = append(summaryParts, "### Analyst Run Summary")
	latencies := reportMap.GetLatencies()
	for name, dur := range latencies {
		summaryParts = append(summaryParts, fmt.Sprintf("- **%s Analyst**: Completed successfully in %s", name, dur.Round(time.Millisecond)))
	}
	for _, p := range trappedPanics {
		summaryParts = append(summaryParts, fmt.Sprintf("- **%s Analyst**: Panicked! recovered successfully.", p.AnalystName))
	}
	for _, e := range activeErrors {
		summaryParts = append(summaryParts, fmt.Sprintf("- **Analyst Error**: %v", e))
	}

	return strings.Join(summaryParts, "\n"), nil
}

// computeAllIndicators calculates all SMA/EMA/RSI/MACD/Bollinger values.
func (o *TradingOrchestrator) computeAllIndicators(ctx context.Context, candles []dataflow.Candle, ticker string, tradeDate time.Time) (map[string]float64, error) {
	inds := []string{"close_50_sma", "close_200_sma", "close_10_ema", "macd", "macds", "macdh", "rsi", "boll", "boll_ub", "boll_lb", "atr", "mfi"}
	result := make(map[string]float64)
	for _, ind := range inds {
		val, err := o.indicatorResolver.Resolve(ctx, candles, ticker, ind, tradeDate)
		if err != nil {
			continue
		}
		result[ind] = val
	}
	return result, nil
}

// runMarketAnalyst invokes the market analyst prompt.
func (o *TradingOrchestrator) runMarketAnalyst(ctx context.Context, state *checkpoint.TradingState, candles []dataflow.Candle, indicatorMap map[string]float64) (string, error) {
	var indStr strings.Builder
	for k, v := range indicatorMap {
		indStr.WriteString(fmt.Sprintf("- %s: %.4f\n", k, v))
	}

	prompt := fmt.Sprintf(`Analyze the market indicators and historical prices for %s as of %s.
Indicators:
%s
Historical candles count: %d.
Write a detailed market trend report and append a markdown summary table at the end.`, state.Ticker, state.TradeDate, indStr.String(), len(candles))

	agent := NewAgent("Market Analyst", "Market Analyst", MarketAnalystInstruction, o.llmProvider)
	return agent.Call(ctx, prompt)
}

// runSentimentAnalyst invokes the sentiment analyst prompt.
func (o *TradingOrchestrator) runSentimentAnalyst(ctx context.Context, state *checkpoint.TradingState) (string, error) {
	prompt := fmt.Sprintf("Analyze sentiment trends, social media discussions, and general retail mood for %s on %s.", state.Ticker, state.TradeDate)
	agent := NewAgent("Sentiment Analyst", "Sentiment Analyst", SentimentAnalystInstruction, o.llmProvider)
	return agent.Call(ctx, prompt)
}

// runNewsAnalyst invokes the news analyst prompt.
func (o *TradingOrchestrator) runNewsAnalyst(ctx context.Context, state *checkpoint.TradingState) (string, error) {
	prompt := fmt.Sprintf("Analyze corporate news, regulatory announcements, macroeconomic events, and insider filings for %s on %s.", state.Ticker, state.TradeDate)
	agent := NewAgent("News Analyst", "News Analyst", NewsAnalystInstruction, o.llmProvider)
	return agent.Call(ctx, prompt)
}

// runFundamentalsAnalyst invokes the fundamentals analyst prompt.
func (o *TradingOrchestrator) runFundamentalsAnalyst(ctx context.Context, state *checkpoint.TradingState, fundamentalsStr string) (string, error) {
	prompt := fmt.Sprintf("Analyze fundamental statements, margins, cash flow details, and valuation metrics for %s on %s.\nDetails:\n%s", state.Ticker, state.TradeDate, fundamentalsStr)
	agent := NewAgent("Fundamentals Analyst", "Fundamentals Analyst", FundamentalsAnalystInstruction, o.llmProvider)
	return agent.Call(ctx, prompt)
}

// RunResearchDebate executes the multi-turn Bull/Bear debate and Consensus Synthesis.
func (o *TradingOrchestrator) RunResearchDebate(ctx context.Context, state *checkpoint.TradingState) (string, cli.CLIState, error) {
	state.Lock()
	if state.InvestmentDebate.History == "" {
		state.InvestmentDebate.History = "### Starting Debate Room\n"
	}
	state.Unlock()

	bullAgent := NewAgent("Bull Analyst", "Bull Analyst", BullInstruction, o.llmProvider)
	bearAgent := NewAgent("Bear Analyst", "Bear Analyst", BearInstruction, o.llmProvider)
	managerAgent := NewAgent("Research Manager", "Research Manager", ResearchManagerInstruction, o.llmProvider)

	for i := 0; i < o.cfg.MaxDebateRounds; i++ {
		state.RLock()
		history := state.InvestmentDebate.History
		market := state.AnalystReports["Market"]
		sentiment := state.AnalystReports["Sentiment"]
		news := state.AnalystReports["News"]
		fundamentals := state.AnalystReports["Fundamentals"]
		state.RUnlock()

		bullPrompt := fmt.Sprintf(`Provide a compelling bullish argument for %s.
Market Analyst Report: %s
Sentiment Report: %s
News Report: %s
Fundamentals Report: %s
Current Debate History:
%s`, state.Ticker, market, sentiment, news, fundamentals, history)

		bullOut, err := bullAgent.Call(ctx, bullPrompt)
		if err != nil {
			return "", cli.StateNeutral, fmt.Errorf("bull analyst failed: %w", err)
		}
		AddDebateMessage(state, "Bull Analyst", bullOut)

		state.RLock()
		history = state.InvestmentDebate.History
		state.RUnlock()

		bearPrompt := fmt.Sprintf(`Provide a compelling bearish argument counteracting the bull's points for %s.
Market Analyst Report: %s
Sentiment Report: %s
News Report: %s
Fundamentals Report: %s
Current Debate History:
%s`, state.Ticker, market, sentiment, news, fundamentals, history)

		bearOut, err := bearAgent.Call(ctx, bearPrompt)
		if err != nil {
			return "", cli.StateNeutral, fmt.Errorf("bear analyst failed: %w", err)
		}
		AddDebateMessage(state, "Bear Analyst", bearOut)
	}

	state.RLock()
	history := state.InvestmentDebate.History
	state.RUnlock()

	managerPrompt := fmt.Sprintf(`Synthesize the debate history and determine the final recommendation and strategic plan for %s.
Debate History:
%s`, state.Ticker, history)

	var plan ResearchPlan
	err := managerAgent.CallStructured(ctx, managerPrompt, &plan)
	if err != nil {
		plan = ResearchPlan{
			Recommendation: "Hold",
			Rationale:      "Fallback synthesis due to formatting issue.",
			StrategicActions: "Maintain current posture.",
		}
	}

	rendered := RenderResearchPlan(plan)

	state.Lock()
	state.InvestmentPlan = rendered
	state.InvestmentDebate.JudgeDecision = fmt.Sprintf("Recommendation: %s", plan.Recommendation)
	state.Unlock()

	cliState := cli.StateNeutral
	recUpper := strings.ToUpper(plan.Recommendation)
	if strings.Contains(recUpper, "BUY") || strings.Contains(recUpper, "OVERWEIGHT") {
		cliState = cli.StateBullish
	} else if strings.Contains(recUpper, "SELL") || strings.Contains(recUpper, "UNDERWEIGHT") {
		cliState = cli.StateBearish
	}

	return rendered, cliState, nil
}

// RunRiskAndSizing executes the sizing and risk management debate.
func (o *TradingOrchestrator) RunRiskAndSizing(ctx context.Context, state *checkpoint.TradingState) (string, cli.CLIState, error) {
	traderAgent := NewAgent("Trader", "Trader", TraderInstruction, o.llmProvider)
	aggRiskAgent := NewAgent("Aggressive Risk", "Aggressive Risk", AggressiveRiskInstruction, o.llmProvider)
	conRiskAgent := NewAgent("Conservative Risk", "Conservative Risk", ConservativeRiskInstruction, o.llmProvider)
	neuRiskAgent := NewAgent("Neutral Risk", "Neutral Risk", NeutralRiskInstruction, o.llmProvider)
	pmAgent := NewAgent("Portfolio Manager", "Portfolio Manager", PortfolioManagerInstruction, o.llmProvider)

	state.RLock()
	market := state.AnalystReports["Market"]
	fundamentals := state.AnalystReports["Fundamentals"]
	planText := state.InvestmentPlan
	state.RUnlock()

	traderPrompt := fmt.Sprintf(`Review the research plan and analyst reports for %s, and make a transaction proposal.
Research Plan:
%s
Market Report: %s
Fundamentals: %s`, state.Ticker, planText, market, fundamentals)

	var proposal TraderProposal
	err := traderAgent.CallStructured(ctx, traderPrompt, &proposal)
	if err != nil {
		proposal = TraderProposal{
			Action:    "Hold",
			Reasoning: "Fallback trader proposal due to call issues.",
		}
	}

	renderedProposal := RenderTraderProposal(proposal)

	state.Lock()
	state.TraderInvestmentPlan = renderedProposal
	state.RiskDebate.History = "### Starting Risk Debate Room\nTrader Proposal:\n" + renderedProposal + "\n"
	state.Unlock()

	for i := 0; i < o.cfg.MaxRiskDiscussRounds; i++ {
		state.RLock()
		history := state.RiskDebate.History
		state.RUnlock()

		aggPrompt := fmt.Sprintf(`Provide your risk feedback representing the AGGRESSIVE risk appetite for %s.
Current History:
%s`, state.Ticker, history)
		aggOut, err := aggRiskAgent.Call(ctx, aggPrompt)
		if err != nil {
			return "", cli.StateNeutral, err
		}
		state.Lock()
		state.RiskDebate.History += "\nAggressive Risk: " + aggOut
		state.RiskDebate.Count++
		state.Unlock()

		state.RLock()
		history = state.RiskDebate.History
		state.RUnlock()

		conPrompt := fmt.Sprintf(`Provide your risk feedback representing the CONSERVATIVE risk appetite for %s.
Current History:
%s`, state.Ticker, history)
		conOut, err := conRiskAgent.Call(ctx, conPrompt)
		if err != nil {
			return "", cli.StateNeutral, err
		}
		state.Lock()
		state.RiskDebate.History += "\nConservative Risk: " + conOut
		state.RiskDebate.Count++
		state.Unlock()

		state.RLock()
		history = state.RiskDebate.History
		state.RUnlock()

		neuPrompt := fmt.Sprintf(`Provide your risk feedback representing the NEUTRAL risk appetite for %s.
Current History:
%s`, state.Ticker, history)
		neuOut, err := neuRiskAgent.Call(ctx, neuPrompt)
		if err != nil {
			return "", cli.StateNeutral, err
		}
		state.Lock()
		state.RiskDebate.History += "\nNeutral Risk: " + neuOut
		state.RiskDebate.Count++
		state.Unlock()
	}

	state.RLock()
	history := state.RiskDebate.History
	state.RUnlock()

	pmPrompt := fmt.Sprintf(`Review the complete risk debate history and trader proposal for %s, and make the final portfolio sizing decision.
Trader Proposal:
%s
Risk Debate:
%s`, state.Ticker, renderedProposal, history)

	var decision PortfolioDecision
	err = pmAgent.CallStructured(ctx, pmPrompt, &decision)
	if err != nil {
		decision = PortfolioDecision{
			Rating:           "Hold",
			ExecutiveSummary: "Fallback decision due to unmarshaling failure.",
			InvestmentThesis: "Strict risk controls applied.",
		}
	}

	renderedDecision := RenderPMDecision(decision)

	state.Lock()
	state.FinalTradeDecision = renderedDecision

	price := 0.0
	if proposal.EntryPrice != nil {
		price = *proposal.EntryPrice
	}
	qty := 0.0
	state.SignalLogs = append(state.SignalLogs, checkpoint.SignalEntry{
		Timestamp: time.Now().Unix(),
		Action:    decision.Rating,
		Price:     price,
		Quantity:  qty,
		Reasoning: decision.ExecutiveSummary,
	})
	state.Unlock()

	cliState := cli.StateNeutral
	ratingUpper := strings.ToUpper(decision.Rating)
	if strings.Contains(ratingUpper, "BUY") || strings.Contains(ratingUpper, "OVERWEIGHT") {
		cliState = cli.StateBullish
	} else if strings.Contains(ratingUpper, "SELL") || strings.Contains(ratingUpper, "UNDERWEIGHT") {
		cliState = cli.StateBearish
	}

	return renderedDecision, cliState, nil
}
