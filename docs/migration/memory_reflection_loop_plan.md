# Migration Plan: Append-Only Memory Log & Outcome Reflection Loop

This document outlines the detailed implementation plan to bring the highly adaptive append-only memory logger, Yahoo Finance outcome price verification, and historical reflection loop from the sibling Python repository (`TradingAgents`) into the Go repository (`trading-agents-go`).

---

## User Review Required

> [!NOTE]
> We will reuse the `DataProvider` interface to fetch Yahoo Finance historical candles, passing the end of the holding period as the look-ahead filter parameter (`tradeDate`). This allows us to cleanly reuse the existing HTTP client and caching layer without duplicating networking logic.

> [!IMPORTANT]
> To prevent early resolution of pending trades (e.g., when running a backtest or live trading on a date shortly after the trade), the resolver will verify that a full `holdingDays` of price data has actually elapsed in real calendar time (`tradeDate + holdingDays < Today`). If not, it will skip resolution for that entry and retry in subsequent runs.

---

## Proposed Changes

### Component: Memory Log & Reflection Package (`internal/memory`)

We will create a new package `internal/memory` with three core Go files: `rating.go`, `journal.go`, and `reflector.go`. This mirrors the modular design of the Python reference implementation.

#### [NEW] [rating.go](file:///Users/alex/repos/personal/trading-agents-go/internal/memory/rating.go)
- Implements a heuristic two-pass rating parser `ParseRating(text string) string` matching the Python implementation:
  1. Regular expression pass looking for an explicit `rating` label: `(?i)rating.*?[:\-][\s*]*(\w+)` and matching against the 5-tier vocabulary (`Buy`, `Overweight`, `Hold`, `Underweight`, `Sell`).
  2. Word-splitting pass to find the first occurrence of any 5-tier rating word.
  3. Defaults to `"Hold"`.

#### [NEW] [journal.go](file:///Users/alex/repos/personal/trading-agents-go/internal/memory/journal.go)
- Defintes the `JournalEntry` struct to hold parsed data:
  ```go
  type JournalEntry struct {
      Date        string
      Ticker      string
      Rating      string
      Pending     bool
      RawReturn   string
      AlphaReturn string
      HoldingDays string
      Decision    string
      Reflection  string
  }
  ```
- Defines the `TradingMemoryLog` struct which handles append-only disk operations under a mutex:
  - `StoreDecision(ticker, tradeDate, finalTradeDecision string) error` - Parses the rating, checks if a pending entry already exists for `(ticker, tradeDate)` (idempotency), and appends it to the file.
  - `LoadEntries() ([]JournalEntry, error)` - Parsed blocks using the `\n\n<!-- ENTRY_END -->\n\n` separator.
  - `GetPendingEntries() ([]JournalEntry, error)` - Returns all pending entries.
  - `GetPastContext(ticker string, nSame, nCross int) (string, error)` - Extracts the `nSame` (5) most recent same-ticker decisions and `nCross` (3) most recent cross-ticker reflections, formatting them for prompt injection.
  - `BatchUpdateWithOutcomes(updates []OutcomeUpdate) error` - Performs atomic writes using temporary files (`.tmp`) and `os.Rename` to update pending logs with raw returns, alpha, holding days, and reflections. Handles entry rotation (`Prune` oldest resolved entries above `MemoryLogMaxEntries`).

#### [NEW] [reflector.go](file:///Users/alex/repos/personal/trading-agents-go/internal/memory/reflector.go)
- Defines `Reflector` which handles reflection prompt formatting and LLM invocation.
- `ReflectOnFinalDecision(ctx context.Context, finalDecision string, rawReturn, alphaReturn float64, benchmark string) (string, error)`
- Uses the standard system instruction:
  ```
  You are a trading analyst reviewing your own past decision now that the outcome is known.
  Write exactly 2-4 sentences of plain prose (no bullets, no headers, no markdown).

  Cover in order:
  1. Was the directional call correct? (cite the alpha figure)
  2. Which part of the investment thesis held or failed?
  3. One concrete lesson to apply to the next similar analysis.

  Be specific and terse. Your output will be stored verbatim in a decision log and re-read by future analysts, so every word must earn its place.
  ```
- Defines the resolution runner:
  - `ResolvePendingEntries(ctx context.Context, ticker string, log *TradingMemoryLog, reflector *Reflector, provider dataflow.DataProvider, customBenchmark string) error`
  - Fetches pricing using `FetchOHLCV`.
  - Determines regional benchmarks (e.g., Tokyo `.T` to `^N225`, US to `SPY`).
  - Measures calendar time elapsed to ensure completion before locking in outcomes.
  - Computes returns and alpha, generates reflections via LLM, and writes updates atomically.

---

### Component: Orchestrator (`internal/orchestrator`)

We will integrate Phase A outcome resolution and Phase D prompt injection into the orchestrator.

#### [MODIFY] [orchestrator.go](file:///Users/alex/repos/personal/trading-agents-go/internal/orchestrator/orchestrator.go)
- **Phase A (Setup):** Right after loading/creating the state checkpoint, we will resolve any pending entries for the current ticker:
  ```go
  if o.cfg.MemoryLogPath != "" {
      log := memory.NewTradingMemoryLog(o.cfg.MemoryLogPath, o.cfg.MemoryLogMaxEntries)
      ref := memory.NewReflector(o.llmProvider)
      _ = memory.ResolvePendingEntries(ctx, ticker, log, ref, o.dataProvider, o.cfg.BenchmarkTicker)
  }
  ```
- **Initial Setup (Metadata):** If starting fresh (StepIndex == 0), load `past_context` from the memory log and save it inside `state.Metadata["past_context"]` to preserve it across potential checkpoint/resume boundary runs.
- **Phase D (Portfolio Manager):** Extract `pastContext` from `state.Metadata["past_context"]` and inject it dynamically into `pmPrompt` using the same lessons block layout:
  ```go
  lessonsLine := ""
  if pastContext != "" {
      lessonsLine = fmt.Sprintf("- Lessons from prior decisions and outcomes:\n%s\n", pastContext)
  }
  ```
- **Finalization:** Replace the placeholder file append logger at the end of the strategy execution with a call to `StoreDecision`.

---

## Verification Plan

### Automated Tests
We will write a comprehensive unit test suite inside `internal/memory/journal_test.go` and `internal/memory/rating_test.go`:
1. **Heuristic Rating Parsing:**
   - Test text containing `"Rating: Buy"`
   - Test bold markdown `"Rating: **Sell**"`
   - Test free prose text containing rating words.
   - Test fallback to default `"Hold"`.
2. **Markdown Parsing & Serialization:**
   - Write standard pending and resolved blocks to a temporary file.
   - Verify parsing yields exact fields.
3. **Atomic Writes & Safe Concurrency:**
   - Spawn multiple concurrent routines reading/writing decisions and updates.
   - Assert no race conditions or corrupted markdown logs occur.
4. **Outcome calculation:**
   - Mock price data arrays to test boundary conditions (weekend/holiday gap, early resolution prevention).
