# Local Reports Generation Implementation Plan

This document details the implementation requirements and technical specifications for replicating the local human-readable reports directory structure from the sibling Python repository (`TradingAgents`) into this Go repository (`trading-agents-go`).

---

## 1. Objective
Implement a local Markdown and directory-based report generation engine in the Go orchestrator. When a strategy workflow runs, it must output a structured `reports/` folder at the root of the Go project containing step-by-step Markdown logs and a beautiful, consolidated long-form report (`complete_report.md`).

---

## 2. Target Directory Structure
For each strategy execution, the system must create a folder at the project root with the format:
`reports/<TICKER>_<YYYYMMDD_HHMMSS>/` (e.g., `reports/AAPL_20260525_154100/`)

Within this folder, generate the following subdirectories and files:
```text
reports/<TICKER>_<TIMESTAMP>/
├── 1_analysts/
│   ├── market.md          # Market Analyst Report
│   ├── sentiment.md       # Sentiment Analyst Report
│   ├── news.md            # News Analyst Report
│   └── fundamentals.md    # Fundamentals Analyst Report
├── 2_research/
│   ├── bull.md            # Bull Analyst arguments (all rounds)
│   ├── bear.md            # Bear Analyst arguments (all rounds)
│   └── manager.md         # Research Manager plan (state.InvestmentPlan)
├── 3_trading/
│   └── trader.md          # Trader proposal (state.TraderInvestmentPlan)
├── 4_risk/
│   ├── aggressive.md      # Aggressive Risk Critique (all rounds)
│   ├── conservative.md    # Conservative Risk Critique (all rounds)
│   └── neutral.md         # Neutral Risk Critique (all rounds)
├── 5_portfolio/
│   └── decision.md        # Portfolio Manager Final Decision (state.FinalTradeDecision)
└── complete_report.md     # Unified Markdown document summarizing the entire run
```

---

## 3. Technical Specifications & Requirements

### 3.1. Configuration & CLI Flags
* **File to modify**: [config.go](file:///Users/alex/repos/personal/trading-agents-go/internal/config/config.go)
  * Add configuration parameters to control report generation:
    * `CreateLocalReports bool` (Default: `true`)
    * `LocalReportsDir string` (Default: `"reports"`)
* **File to modify**: [main.go](file:///Users/alex/repos/personal/trading-agents-go/cmd/tradingagents/main.go)
  * Bind these configurations to command line flags:
    * `--enable-local-reports` (bool, default `true`)
    * `--local-reports-dir` (string, default `"reports"`)

### 3.2. State & In-Memory Capture (Capturing Single-Agent Rounds)
In [orchestrator.go](file:///Users/alex/repos/personal/trading-agents-go/internal/orchestrator/orchestrator.go), some analysts (e.g., Bull/Bear and Risk Analysts) execute in sequential loops and append their outputs directly to a single shared history string (`state.InvestmentDebate.History` and `state.RiskDebate.History`). 
To save these as individual files (e.g., `bull.md`, `bear.md`, `aggressive.md`), choose one of the following approaches:
* **Option A (In-Memory Capture - Recommended)**: Maintain local `strings.Builder` or slice structures inside the `RunResearchDebate` and `RunRiskAndSizing` methods to accumulate the individual outputs of each respective agent across all rounds. Pass these accumulated strings directly to the report generation engine.
* **Option B (Struct Extension)**: Update `TradingState` in `internal/checkpoint/checkpointer.go` to store specific string fields for `BullOutputs []string`, `BearOutputs []string`, `AggressiveRiskOutputs []string`, `ConservativeRiskOutputs []string`, and `NeutralRiskOutputs []string` to ensure full state-checkpoint capability for report compilation even on resumed runs.

### 3.3. Modular Report Generator Component
Create a new file `internal/report/generator.go` (or `internal/orchestrator/report.go`) containing the report generation engine.

```go
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ReportData groups all markdown payloads together for clean assembly
type ReportData struct {
	Ticker                 string
	TradeDate              string
	Timestamp              time.Time
	MarketReport           string
	SentimentReport        string
	NewsReport             string
	FundamentalsReport     string
	BullDebate             string
	BearDebate             string
	ResearchPlan           string
	TraderProposal         string
	AggressiveRisk         string
	ConservativeRisk       string
	NeutralRisk            string
	FinalDecision          string
}
```

Implement a `GenerateLocalReports(data *ReportData, baseDir string) error` function that performs:
1. **Directory Initializations**: Creates the target folder `reports/<TICKER>_<YYYYMMDD_HHMMSS>/` and subdirectories `1_analysts`, `2_research`, `3_trading`, `4_risk`, and `5_portfolio` using `os.MkdirAll`.
2. **Individual MD Writes**: Writes `os.WriteFile` for each single Markdown file listed in the Target Directory Structure.
3. **Consolidated Report Template (`complete_report.md`)**: Assemble all sections into a single Markdown file. Ensure beautiful formatting using headings (`#`, `##`, `###`), horizontal dividers (`---`), and metadata headers:
   ```markdown
   # Complete Trading Agent Report: AAPL
   
   - **Date of Trade Analysis**: YYYY-MM-DD
   - **Execution Time**: YYYY-MM-DD HH:MM:SS
   
   ---
   
   ## Executive Summary & Portfolio Decision
   [Insert Final Decision Markdown]
   
   ## 1. Concurrent Market Analysis
   ...
   ```

### 3.4. Integration into Orchestrator Execution Pipeline
* **File to modify**: [orchestrator.go](file:///Users/alex/repos/personal/trading-agents-go/internal/orchestrator/orchestrator.go)
* Locate `Phase E: Finalization & Cleanup` inside `Execute()`.
* Right **before** clearing the SQLite checkpoint (`o.checkpointer.Clear(...)`), synthesize the `ReportData` struct with all collected strings and call:
  ```go
  if o.cfg.CreateLocalReports {
      reportData := &report.ReportData{
          Ticker:             ticker,
          TradeDate:          tradeDate,
          Timestamp:          time.Now(),
          MarketReport:       state.AnalystReports["Market"],
          SentimentReport:    state.AnalystReports["Sentiment"],
          NewsReport:         state.AnalystReports["News"],
          FundamentalsReport: state.AnalystReports["Fundamentals"],
          BullDebate:         accumulatedBullText,
          BearDebate:         accumulatedBearText,
          ResearchPlan:       state.InvestmentPlan,
          TraderProposal:     state.TraderInvestmentPlan,
          AggressiveRisk:     accumulatedAggRiskText,
          ConservativeRisk:   accumulatedConRiskText,
          NeutralRisk:        accumulatedNeuRiskText,
          FinalDecision:      state.FinalTradeDecision,
      }
      _ = report.GenerateLocalReports(reportData, o.cfg.LocalReportsDir)
  }
  ```

### 3.5. Verification & Tests
* Create `internal/report/generator_test.go`.
* Write a unit test simulating a dry-run execution. Provide mock strings for all inputs and verify that the exact folder structure is generated and populated correctly under a temporary test directory. Use `os.RemoveAll` to clean up the test directory post-assertion.
