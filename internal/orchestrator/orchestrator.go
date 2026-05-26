package orchestrator

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"trading-agents-go/internal/checkpoint"
	"trading-agents-go/internal/cli"
	"trading-agents-go/internal/config"
	"trading-agents-go/internal/dataflow"
	"trading-agents-go/internal/indicators"
	"trading-agents-go/internal/memory"
	"trading-agents-go/internal/report"
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
	
	InstitutionalToneInstruction = "\n\nCRITICAL INSTRUCTIONS ON TONE:\n" +
		"1. **Institutional Professionalism**: Your tone must be strictly professional, dry, objective, and highly quantitative.\n" +
		"2. **Forbidden Vocabulary**: Do NOT use emotive, colloquial, or retail-trading slang. Forbidden phrases include (but are not limited to): \"load the boat\", \"generational buying opportunity\", \"scared money makes no money\", \"to the moon\", \"trapdoor\", \"blood in the streets.\"\n" +
		"3. **Expressing Conviction Professionally**: You may express extreme confidence (e.g., Bull or Aggressive) or extreme caution (Bear or Conservative), but you must do so using institutional terminology. Instead of \"This is a generational buy,\" use \"The risk/reward asymmetry here presents a highly compelling entry point for maximum standard deviation allocation.\"\n" +
		"4. **Data Over Emotion**: Let the math do the talking. Support your aggressive or defensive stances with Beta calculations, historical drawdowns, liquidity ratios, and risk-adjusted return metrics, not dramatic rhetoric."

	BullInstruction = "You are a Bull Analyst. Build a strong, evidence-based bullish case highlighting growth opportunities, competitive moats, and positive momentum. Engage directly with the bear analyst's counter-arguments." + InstitutionalToneInstruction
	
	BearInstruction = "You are a Bear Analyst. Build a strong, evidence-based bearish case highlighting risks, threats, competitive weaknesses, and macro challenges. Counter the bull's points with rigorous evidence." + InstitutionalToneInstruction
	
	ResearchManagerInstruction = "You are the Research Manager and debate facilitator. Evaluate the bull/bear debate and produce a structured investment plan in JSON format matching the ResearchPlan schema. Be decisive; commit to Buy/Sell if the strongest arguments warrant it."
	
	TraderInstruction = "You are the Trader. Convert the investment plan and analyst reports into a concrete transaction proposal. Specify stop-loss, entry targets, and position sizing guidelines in JSON format matching the TraderProposal schema."
	
	OptionsStrategistInstruction = "You are the Head Options Strategist and Execution Manager. The fundamental and technical analysts have established a directional bias and identified key support/resistance levels and implied volatility parameters.\n\n" +
		"CRITICAL INSTRUCTIONS:\n" +
		"1. **Beyond Vanilla Equity**: Do not limit your execution plan to simply buying or selling the underlying stock at a limit or market price.\n" +
		"2. **Options for Entry**: If the consensus is to BUY at a lower support level, suggest yield-generating entry strategies. For example, evaluate selling Cash-Secured Puts (CSPs) at the target support strike. Calculate the premium collected and the adjusted cost basis.\n" +
		"3. **Options for Exit/Yield**: If the consensus is to hold the asset, evaluate selling Covered Calls at the identified resistance levels to capture theta decay and reduce the cost basis.\n" +
		"4. **Hedging High Beta**: If the asset has a high Beta or downside risk is elevated, suggest protective put spreads or collars to cap the downside risk.\n" +
		"5. **Format**: Present your strategy in a clear table: Strategy Type | Strike Price | Expiration Target | Estimated Premium | Adjusted Break-Even." + InstitutionalToneInstruction
	
	AggressiveRiskInstruction = "You are the Aggressive Risk analyst. Critique the transaction proposal. Suggest higher sizing if trends support it; look for opportunities to maximize gains." + InstitutionalToneInstruction
	
	ConservativeRiskInstruction = "You are the Conservative Risk analyst. Critique the transaction proposal from a defensive standpoint. Recommend capital preservation, tighter stop-losses, and reduced size." + InstitutionalToneInstruction
	
	NeutralRiskInstruction = "You are the Neutral Risk analyst. Balance both aggressive and conservative feedback to outline an objective risk-reward profile." + InstitutionalToneInstruction
	
	PortfolioManagerInstruction = "You are the Portfolio Manager. Synthesize the risk analysts' debate and the trader proposal. Produce the final position rating (Buy / Overweight / Hold / Underweight / Sell) and sizing thesis in JSON format matching the PortfolioDecision schema."
)

// TradingOrchestrator coordinates the pipeline lifecycle and multi-agent loops.
type TradingOrchestrator struct {
	cfg                *config.Config
	checkpointer       *checkpoint.StateCheckpointer
	dataProvider       dataflow.DataProvider
	llmProvider        provider.LLMProvider
	indicatorResolver  *indicators.DynamicIndicatorResolver
	newsSocialProvider dataflow.NewsSocialProvider
}

// NewTradingOrchestrator instantiates a new orchestrator.
func NewTradingOrchestrator(
	cfg *config.Config,
	checkpointer *checkpoint.StateCheckpointer,
	dataProvider dataflow.DataProvider,
	llmProvider provider.LLMProvider,
	indicatorResolver *indicators.DynamicIndicatorResolver,
	newsSocialProvider dataflow.NewsSocialProvider,
) *TradingOrchestrator {
	return &TradingOrchestrator{
		cfg:                cfg,
		checkpointer:       checkpointer,
		dataProvider:       dataProvider,
		llmProvider:        llmProvider,
		indicatorResolver:  indicatorResolver,
		newsSocialProvider: newsSocialProvider,
	}
}

// Execute runs the complete strategy execution pipeline with full fault tolerance and checkpoint resumption.
func (o *TradingOrchestrator) Execute(ctx context.Context, ticker string, tradeDate string, cliController *cli.CLIController) (string, error) {
	// 1. Phase A: Setup & Checkpoint Recovery
	state, stepIndex, err := o.checkpointer.Load(ctx, ticker, tradeDate)
	if err != nil {
		return "", fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Resolve any pending memory log entries before the pipeline runs (Phase A outcome resolution)
	if o.cfg.MemoryLogPath != "" {
		log := memory.NewTradingMemoryLog(o.cfg.MemoryLogPath, o.cfg.MemoryLogMaxEntries)
		ref := memory.NewReflector(o.llmProvider)
		_ = memory.ResolvePendingEntries(ctx, ticker, log, ref, o.dataProvider, o.cfg.BenchmarkTicker)
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

		if o.cfg.MemoryLogPath != "" {
			log := memory.NewTradingMemoryLog(o.cfg.MemoryLogPath, o.cfg.MemoryLogMaxEntries)
			pastContext, err := log.GetPastContext(ticker, 5, 3)
			if err == nil && pastContext != "" {
				state.Metadata["past_context"] = pastContext
			}
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
	var reportData *report.ReportData
	state.RLock()
	if o.cfg.CreateLocalReports {
		reportData = &report.ReportData{
			Ticker:             ticker,
			TradeDate:          tradeDate,
			Timestamp:          time.Now(),
			MarketReport:       state.AnalystReports["Market"],
			SentimentReport:    state.AnalystReports["Sentiment"],
			NewsReport:         state.AnalystReports["News"],
			FundamentalsReport: state.AnalystReports["Fundamentals"],
			BullDebate:         strings.Join(state.BullDebateHistory, "\n\n---\n\n"),
			BearDebate:         strings.Join(state.BearDebateHistory, "\n\n---\n\n"),
			ResearchPlan:       state.InvestmentPlan,
			TraderProposal:     state.TraderInvestmentPlan,
			OptionsStrategy:    state.OptionsStrategy,
			AggressiveRisk:     strings.Join(state.AggressiveRiskHistory, "\n\n---\n\n"),
			ConservativeRisk:   strings.Join(state.ConservativeRiskHistory, "\n\n---\n\n"),
			NeutralRisk:        strings.Join(state.NeutralRiskHistory, "\n\n---\n\n"),
			FinalDecision:      state.FinalTradeDecision,
		}
	}
	state.RUnlock()

	if reportData != nil {
		if err := report.GenerateLocalReports(reportData, o.cfg.LocalReportsDir); err != nil {
			fmt.Printf("[WARNING] Failed to generate local reports: %v\n", err)
		}
	}

	if err := o.checkpointer.Clear(ctx, ticker, tradeDate); err != nil {
		return "", fmt.Errorf("failed to clear checkpoint: %w", err)
	}

	if o.cfg.MemoryLogPath != "" {
		log := memory.NewTradingMemoryLog(o.cfg.MemoryLogPath, o.cfg.MemoryLogMaxEntries)
		state.RLock()
		finalDecision := state.FinalTradeDecision
		state.RUnlock()
		if err := log.StoreDecision(ticker, tradeDate, finalDecision); err != nil {
			fmt.Printf("[WARNING] Failed to store decision in memory log: %v\n", err)
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

	// Scrape corporate and global news, StockTwits, and Reddit concurrently
	var (
		newsBlock       string
		globalNewsBlock string
		stocktwitsBlock string
		redditBlock     string
		newsErr         error
		gNewsErr        error
		stErr           error
		rdErr           error
	)

	var scrapeWg sync.WaitGroup
	scrapeWg.Add(4)

	go func() {
		defer scrapeWg.Done()
		lookbackStart := tradeDateParsed.AddDate(0, 0, -7)
		newsBlock, newsErr = o.newsSocialProvider.FetchNews(ctx, state.Ticker, lookbackStart, tradeDateParsed)
	}()

	go func() {
		defer scrapeWg.Done()
		globalNewsBlock, gNewsErr = o.newsSocialProvider.FetchGlobalNews(ctx, tradeDateParsed, 7, 10)
	}()

	go func() {
		defer scrapeWg.Done()
		stocktwitsBlock, stErr = o.newsSocialProvider.FetchStockTwits(ctx, state.Ticker, 30)
	}()

	go func() {
		defer scrapeWg.Done()
		redditBlock, rdErr = o.newsSocialProvider.FetchReddit(ctx, state.Ticker, []string{"wallstreetbets", "stocks", "investing"}, 5)
	}()

	scrapeWg.Wait()

	if newsErr != nil {
		fmt.Printf("[WARNING] FetchNews failed for %s: %v\n", state.Ticker, newsErr)
	}
	if gNewsErr != nil {
		fmt.Printf("[WARNING] FetchGlobalNews failed: %v\n", gNewsErr)
	}
	if stErr != nil {
		fmt.Printf("[WARNING] FetchStockTwits failed for %s: %v\n", state.Ticker, stErr)
	}
	if rdErr != nil {
		fmt.Printf("[WARNING] FetchReddit failed for %s: %v\n", state.Ticker, rdErr)
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
		return o.runSentimentAnalyst(ctx, state, newsBlock, stocktwitsBlock, redditBlock)
	})
	go executeSafe("News", func() (string, error) {
		return o.runNewsAnalyst(ctx, state, newsBlock, globalNewsBlock)
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

	agent := o.createAgent("Market Analyst", "Market Analyst", MarketAnalystInstruction)
	return agent.Call(ctx, prompt)
}

// runSentimentAnalyst invokes the sentiment analyst prompt.
func (o *TradingOrchestrator) runSentimentAnalyst(ctx context.Context, state *checkpoint.TradingState, newsBlock, stocktwitsBlock, redditBlock string) (string, error) {
	prompt := fmt.Sprintf(`You are a financial market sentiment analyst. Your task is to produce a comprehensive sentiment report for %s covering the period up to %s, drawing on three complementary data sources that have already been collected for you.

## Data sources (pre-fetched, in this prompt)

### News headlines — Yahoo Finance, past 7 days
Institutional framing. Fact-driven, slower-moving signal.

<start_of_news>
%s
<end_of_news>

### StockTwits messages — retail-trader social platform indexed by cashtag
Fast-moving signal. Each message carries a user-labeled sentiment tag (Bullish / Bearish / no-label) plus the message body.

<start_of_stocktwits>
%s
<end_of_stocktwits>

### Reddit posts — r/wallstreetbets, r/stocks, r/investing (past 7 days)
Community discussion. Engagement signal via upvote score and comment count. Subreddit character matters.

<start_of_reddit>
%s
<end_of_reddit>

## How to analyze this data (best practices)

1. **Read the StockTwits Bullish/Bearish ratio as a leading retail-sentiment signal.** A 70/30 bullish/bearish split is moderately bullish; >=90/10 may indicate over-extension and contrarian risk; 50/50 is uncertainty. Base rates on the actual message count, not percentages alone.
2. **Look for cross-source divergences.** If news framing is bearish but StockTwits is overwhelmingly bullish, that mismatch is itself a signal.
3. **Weight Reddit posts by engagement.** High score and comment count posts reflect community attention; low score posts are noise.
4. **Distinguish opinion from event.** A news headline is an event; a StockTwits post is opinion.
5. **Identify recurring narrative themes.** What topic keeps coming up across sources?
6. **Be honest about data limits.** If StockTwits returned only a handful of messages, or one or more sources returned an "<unavailable>" placeholder, flag this caveat explicitly.
7. **Identify catalysts and risks** that emerge across sources.
8. **Past sentiment is not predictive.** Frame conclusions as signals for the trader to weigh, not as a price call.

## Output

Produce a sentiment report covering, in order:

1. **Overall sentiment direction** — Bullish / Bearish / Neutral / Mixed — with a brief confidence note based on data quality and sample size.
2. **Source-by-source breakdown** — what each of news / StockTwits / Reddit is telling you, with specific evidence (cite message counts, ratios, notable posts).
3. **Divergences, alignments, and key narratives** across sources.
4. **Catalysts and risks** surfaced by the data.
5. **Markdown table** at the end summarizing key sentiment signals, their direction, source, and supporting evidence.`, state.Ticker, state.TradeDate, newsBlock, stocktwitsBlock, redditBlock)
	agent := o.createAgent("Sentiment Analyst", "Sentiment Analyst", SentimentAnalystInstruction)
	return agent.Call(ctx, prompt)
}

// runNewsAnalyst invokes the news analyst prompt.
func (o *TradingOrchestrator) runNewsAnalyst(ctx context.Context, state *checkpoint.TradingState, newsBlock, globalNewsBlock string) (string, error) {
	prompt := fmt.Sprintf(`You are a news researcher tasked with analyzing recent news and trends over the past week. Please write a comprehensive report of the current state of the world that is relevant for trading and macroeconomics for %s on %s.

## Data sources (pre-fetched, in this prompt)

### Corporate specific news — past 7 days
<start_of_news>
%s
<end_of_news>

### Global Macroeconomic & Market news — past 7 days
<start_of_global_news>
%s
<end_of_global_news>

Provide specific, actionable insights with supporting evidence to help traders make informed decisions. Make sure to append a Markdown table at the end of the report to organize key points in the report, making it organized and easy to read.`, state.Ticker, state.TradeDate, newsBlock, globalNewsBlock)
	agent := o.createAgent("News Analyst", "News Analyst", NewsAnalystInstruction)
	return agent.Call(ctx, prompt)
}

// runFundamentalsAnalyst invokes the fundamentals analyst prompt.
func (o *TradingOrchestrator) runFundamentalsAnalyst(ctx context.Context, state *checkpoint.TradingState, fundamentalsStr string) (string, error) {
	prompt := fmt.Sprintf("Analyze fundamental statements, margins, cash flow details, and valuation metrics for %s on %s.\nDetails:\n%s", state.Ticker, state.TradeDate, fundamentalsStr)
	agent := o.createAgent("Fundamentals Analyst", "Fundamentals Analyst", FundamentalsAnalystInstruction)
	return agent.Call(ctx, prompt)
}

// RunResearchDebate executes the multi-turn Bull/Bear debate and Consensus Synthesis.
func (o *TradingOrchestrator) RunResearchDebate(ctx context.Context, state *checkpoint.TradingState) (string, cli.CLIState, error) {
	state.Lock()
	if state.InvestmentDebate.History == "" {
		state.InvestmentDebate.History = "### Starting Debate Room\n"
	}
	state.Unlock()

	bullAgent := o.createAgent("Bull Analyst", "Bull Analyst", BullInstruction)
	bearAgent := o.createAgent("Bear Analyst", "Bear Analyst", BearInstruction)
	managerAgent := o.createAgent("Research Manager", "Research Manager", ResearchManagerInstruction)

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
		state.Lock()
		state.BullDebateHistory = append(state.BullDebateHistory, bullOut)
		state.Unlock()

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
		state.Lock()
		state.BearDebateHistory = append(state.BearDebateHistory, bearOut)
		state.Unlock()
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
	traderAgent := o.createAgent("Trader", "Trader", TraderInstruction)
	optionsAgent := o.createAgent("Options Strategist", "Options Strategist", OptionsStrategistInstruction)
	aggRiskAgent := o.createAgent("Aggressive Risk", "Aggressive Risk", AggressiveRiskInstruction)
	conRiskAgent := o.createAgent("Conservative Risk", "Conservative Risk", ConservativeRiskInstruction)
	neuRiskAgent := o.createAgent("Neutral Risk", "Neutral Risk", NeutralRiskInstruction)
	pmAgent := o.createAgent("Portfolio Manager", "Portfolio Manager", PortfolioManagerInstruction)

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

	optionsPrompt := fmt.Sprintf(`Review the research plan, trader proposal, and analyst reports for %s to formulate an optimized options execution strategy.

Research Plan:
%s

Trader Proposal:
%s

Market Report:
%s

Fundamentals:
%s`, state.Ticker, planText, renderedProposal, market, fundamentals)

	optionsStrategy, err := optionsAgent.Call(ctx, optionsPrompt)
	if err != nil {
		optionsStrategy = "Fallback options strategy due to execution failure."
	}

	state.Lock()
	state.TraderInvestmentPlan = renderedProposal
	state.OptionsStrategy = optionsStrategy
	state.RiskDebate.History = "### Starting Risk Debate Room\nTrader Proposal:\n" + renderedProposal + "\n\nOptions Execution Strategy:\n" + optionsStrategy + "\n"
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
		state.AggressiveRiskHistory = append(state.AggressiveRiskHistory, aggOut)
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
		state.ConservativeRiskHistory = append(state.ConservativeRiskHistory, conOut)
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
		state.NeutralRiskHistory = append(state.NeutralRiskHistory, neuOut)
		state.Unlock()
	}

	state.RLock()
	history := state.RiskDebate.History
	pastContext := state.Metadata["past_context"]
	state.RUnlock()

	lessonsLine := ""
	if pastContext != "" {
		lessonsLine = fmt.Sprintf("\n- Lessons from prior decisions and outcomes:\n%s\n", pastContext)
	}

	pmPrompt := fmt.Sprintf(`Review the complete risk debate history and trader proposal for %s, and make the final portfolio sizing decision.
Trader Proposal:
%s
%sRisk Debate:
%s`, state.Ticker, renderedProposal, lessonsLine, history)

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
