package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"trading-agents-go/internal/dataflow"
	"trading-agents-go/pkg/provider"
)

// ReflectionPrompt is the system prompt that constrains the LLM to generate compact, highly structured 2-4 sentence prose reflections.
const ReflectionPrompt = `You are a trading analyst reviewing your own past decision now that the outcome is known.
Write exactly 2-4 sentences of plain prose (no bullets, no headers, no markdown).

Cover in order:
1. Was the directional call correct? (cite the alpha figure)
2. Which part of the investment thesis held or failed?
3. One concrete lesson to apply to the next similar analysis.

Be specific and terse. Your output will be stored verbatim in a decision log and re-read by future analysts, so every word must earn its place.`

// Reflector coordinates reflection generation via unified LLM interface.
type Reflector struct {
	llm provider.LLMProvider
}

// NewReflector creates a new Reflector.
func NewReflector(llm provider.LLMProvider) *Reflector {
	return &Reflector{llm: llm}
}

// ReflectOnFinalDecision generates a compact hindsight reflection on a trade outcome.
func (r *Reflector) ReflectOnFinalDecision(ctx context.Context, finalDecision string, rawReturn, alphaReturn float64, benchmarkName string) (string, error) {
	userPrompt := fmt.Sprintf("Raw return: %s\nAlpha vs %s: %s\n\nFinal Decision:\n%s",
		formatPercent(rawReturn),
		benchmarkName,
		formatPercent(alphaReturn),
		finalDecision,
	)

	req := provider.LLMRequest{
		SystemPrompt: ReflectionPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
	}

	return r.llm.Generate(ctx, req)
}

// ResolveBenchmark resolves regional baseline indices based on ticker suffix.
func ResolveBenchmark(ticker string, customBenchmark string) string {
	if customBenchmark != "" {
		return customBenchmark
	}
	benchmarkMap := map[string]string{
		".NS":  "^NSEI",
		".BO":  "^BSESN",
		".T":   "^N225",
		".HK":  "^HSI",
		".L":   "^FTSE",
		".TO":  "^GSPTSE",
		".AX":  "^AXJO",
	}
	tickerUpper := strings.ToUpper(ticker)
	for suffix, bench := range benchmarkMap {
		if strings.HasSuffix(tickerUpper, suffix) {
			return bench
		}
	}
	return "SPY"
}

// ResolvePendingEntries fetches returns, calculates alpha, generates reflections, and commits them in an atomic batch.
func ResolvePendingEntries(ctx context.Context, ticker string, log *TradingMemoryLog, reflector *Reflector, dataProvider dataflow.DataProvider, customBenchmark string) error {
	pending, err := log.GetPendingEntries()
	if err != nil {
		return fmt.Errorf("failed to load pending entries: %w", err)
	}

	var tickerPending []JournalEntry
	for _, entry := range pending {
		if entry.Ticker == ticker {
			tickerPending = append(tickerPending, entry)
		}
	}

	if len(tickerPending) == 0 {
		return nil
	}

	benchmark := ResolveBenchmark(ticker, customBenchmark)
	holdingDays := 5

	var updates []OutcomeUpdate

	for _, entry := range tickerPending {
		tradeDateParsed, err := time.Parse("2006-01-02", entry.Date)
		if err != nil {
			continue // skip invalid date formats in logs
		}

		// Let's filter out trades that are still ongoing in real time.
		if tradeDateParsed.AddDate(0, 0, holdingDays).After(time.Now()) {
			continue // wait until the holding period completes
		}

		start := tradeDateParsed
		end := start.AddDate(0, 0, holdingDays+7) // buffer for holidays/weekends

		stockCandles, err := dataProvider.FetchOHLCV(ctx, ticker, start, end, end)
		if err != nil {
			continue
		}
		benchCandles, err := dataProvider.FetchOHLCV(ctx, benchmark, start, end, end)
		if err != nil {
			continue
		}

		if len(stockCandles) < 2 || len(benchCandles) < 2 {
			continue // not enough price data available yet
		}

		// Helper to find index of target date or nearest trading day on/after it
		findStartIdx := func(candles []dataflow.Candle, target time.Time) int {
			tDay := target.Truncate(24 * time.Hour)
			for i, c := range candles {
				cDay := c.Time.Truncate(24 * time.Hour)
				if cDay.Equal(tDay) || cDay.After(tDay) {
					return i
				}
			}
			return -1
		}

		stockStartIdx := findStartIdx(stockCandles, tradeDateParsed)
		benchStartIdx := findStartIdx(benchCandles, tradeDateParsed)

		if stockStartIdx == -1 || benchStartIdx == -1 {
			continue
		}

		availableStock := len(stockCandles) - 1 - stockStartIdx
		availableBench := len(benchCandles) - 1 - benchStartIdx

		// Ensure we don't resolve early if we don't have enough trading days
		if availableStock < holdingDays || availableBench < holdingDays {
			continue
		}

		actualDays := holdingDays
		if availableStock < actualDays {
			actualDays = availableStock
		}
		if availableBench < actualDays {
			actualDays = availableBench
		}

		if actualDays <= 0 {
			continue
		}

		startPrice := stockCandles[stockStartIdx].Close
		endPrice := stockCandles[stockStartIdx+actualDays].Close
		if startPrice <= 0 {
			continue
		}
		rawReturn := (endPrice - startPrice) / startPrice

		benchStartPrice := benchCandles[benchStartIdx].Close
		benchEndPrice := benchCandles[benchStartIdx+actualDays].Close
		if benchStartPrice <= 0 {
			continue
		}
		benchReturn := (benchEndPrice - benchStartPrice) / benchStartPrice

		alphaReturn := rawReturn - benchReturn

		reflection, err := reflector.ReflectOnFinalDecision(ctx, entry.Decision, rawReturn, alphaReturn, benchmark)
		if err != nil {
			continue
		}

		updates = append(updates, OutcomeUpdate{
			Ticker:      ticker,
			TradeDate:   entry.Date,
			RawReturn:   rawReturn,
			AlphaReturn: alphaReturn,
			HoldingDays: actualDays,
			Reflection:  reflection,
		})
	}

	if len(updates) > 0 {
		if err := log.BatchUpdateWithOutcomes(updates); err != nil {
			return fmt.Errorf("failed to commit resolved outcomes: %w", err)
		}
	}

	return nil
}
