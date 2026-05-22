# TradingAgents: Direct Translation Migration Plan (Python to Go)

This document provides a highly detailed, file-by-file design and implementation plan to migrate the existing Python-based `TradingAgents` framework directly to Go. The goal of this direct migration is to preserve the exact logical architecture, state-graph flow, API interactions, and user interface features of the original project, while mapping them to equivalent, stable Go libraries and idiomatic structures.

This plan is written to target the `/Users/alex/repos/personal/trading-agents-go` directory.

---

## 1. Project Layout & Directory Structure Mapping

To maintain alignment with the original Python project, we will map the package directories directly. Since Go does not use `__init__.py` files, the initialization and exported package components will be managed using Go's package boundary system.

| Original Python Path | Target Go Package / File | Purpose |
|:---|:---|:---|
| `/pyproject.toml` | `/go.mod` | Go module declarations and dependencies |
| `/main.py` | `/main.go` | Entrypoint for direct graph propagation |
| `/tradingagents/default_config.py` | `/config/config.go` | Singleton configuration, environment loading |
| `/tradingagents/__init__.py` | `/tradingagents.go` | Package declarations / exported interfaces |
| `/tradingagents/agents/schemas.py` | `/agents/schemas.go` | Pydantic equivalent Go structures and JSON tags |
| `/tradingagents/agents/utils/structured.py` | `/agents/structured.go` | Structured output parsing and fallback logic |
| `/tradingagents/agents/utils/agent_states.go` | `/agents/states.go` | Go struct definitions for `AgentState` & debates |
| `/tradingagents/agents/utils/agent_utils.py` | `/agents/utils.go` | String utilities, prompt builders, message cleaners |
| `/tradingagents/agents/utils/memory.py` | `/agents/memory.go` | Append-only markdown log manager |
| `/tradingagents/agents/analysts/*` | `/agents/analysts/` | Analyst nodes (market, sentiment, news, fundamentals) |
| `/tradingagents/agents/managers/*` | `/agents/managers/` | Manager nodes (research, portfolio) |
| `/tradingagents/agents/researchers/*` | `/agents/researchers/` | Bull and Bear debate researcher nodes |
| `/tradingagents/agents/risk_mgmt/*` | `/agents/risk/` | Aggressive, conservative, and neutral risk debaters |
| `/tradingagents/dataflows/interface.py` | `/dataflows/interface.go` | Unified data flow router with fallback chains |
| `/tradingagents/dataflows/y_finance.py` | `/dataflows/yfinance.go` | Yahoo Finance endpoints (OHLCV, fundamentals) |
| `/tradingagents/dataflows/yfinance_news.py` | `/dataflows/yfinance_news.go`| News & macro news scraping via Yahoo Finance |
| `/tradingagents/dataflows/reddit.py` | `/dataflows/reddit.go` | Reddit JSON search endpoint parser |
| `/tradingagents/dataflows/stocktwits.py` | `/dataflows/stocktwits.go` | StockTwits stream fetcher and sentiment calculator |
| `/tradingagents/dataflows/stockstats_utils.py`| `/dataflows/indicators.go` | Technical indicator mathematical implementations |
| `/tradingagents/graph/trading_graph.py` | `/graph/graph.go` | Main state orchestration coordinator |
| `/tradingagents/graph/setup.py` | `/graph/setup.go` | Node registration and sequential wiring |
| `/tradingagents/graph/checkpointer.py` | `/graph/checkpointer.go` | SQLite-based step persistence & recovery |
| `/tradingagents/graph/conditional_logic.py` | `/graph/conditions.go` | Execution branching decision makers |
| `/tradingagents/llm_clients/*` | `/llm/` | Model clients, capabilities, API key management |
| `/cli/main.py` | `/cli/cli.go` | Typer equivalent CLI via spf13/cobra |

---

## 2. Core Dependencies & Libraries Mapping

We will replace the heavy Python dependencies with stable, well-maintained Go equivalents:

| Python Library | Go Library Equivalent | Rationale |
|:---|:---|:---|
| `langchain-core`, `langgraph` | Custom `graph` engine | Go does not have a mature LangGraph port. A native channel/goroutine/struct-based graph engine is simple and far more performant. |
| `langgraph-checkpoint-sqlite`| `database/sql` + `modernc.org/sqlite` | Pure Go, CGO-free SQLite driver. Allows seamless cross-compilation. |
| `pandas`, `stockstats` | Native calculations or `cinar/indicator` | Replaces heavy dataframes with simple, highly efficient, zero-allocation Go loops over slices of floats. |
| `yfinance` | Custom HTTP client calling Yahoo endpoints | Emulates `yfinance` by directly querying the `query1.finance.yahoo.com/v7/finance/download` and `query2.finance.yahoo.com/v1/finance/search` APIs. |
| `requests` | `net/http` | Standard library HTTP client with custom headers, timeouts, and transport configurations. |
| `pydantic` | Standard `struct` + `encoding/json` | Go's compiler and strict typing provide validation out of the box, supplemented by struct tags. |
| `typer`, `questionary` | `spf13/cobra` + `manifoldco/promptui` | Cobra is the industry standard for Go CLIs. Promptui delivers beautiful, interactive multi-select menus. |
| `rich` | `github.com/charmbracelet/lipgloss` | Lipgloss provides highly aesthetic, color-harmonized terminal styling, layout blocks, and text wrapping. |

---

## 3. Detailed Component Migration Subplans

### 3.1. Package `config` (Configuration Management)
**Python file**: `/tradingagents/default_config.py`

#### Architecture
The Go configuration will use a `Config` struct initialized with default values mirroring `DEFAULT_CONFIG`. It will utilize standard environment variables via `os.Getenv` to override settings at launch, performing coercion to match types.

#### Struct Definition
```go
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type DataVendors struct {
	CoreStockAPIs       string `json:"core_stock_apis"`
	TechnicalIndicators string `json:"technical_indicators"`
	FundamentalData     string `json:"fundamental_data"`
	NewsData            string `json:"news_data"`
}

type Config struct {
	ProjectDir              string            `json:"project_dir"`
	ResultsDir              string            `json:"results_dir"`
	DataCacheDir            string            `json:"data_cache_dir"`
	MemoryLogPath           string            `json:"memory_log_path"`
	MemoryLogMaxEntries     *int              `json:"memory_log_max_entries"`
	LLMProvider             string            `json:"llm_provider"`
	DeepThinkLLM            string            `json:"deep_think_llm"`
	QuickThinkLLM           string            `json:"quick_think_llm"`
	BackendURL              string            `json:"backend_url"`
	GoogleThinkingLevel     string            `json:"google_thinking_level"`
	OpenAIReasoningEffort   string            `json:"openai_reasoning_effort"`
	AnthropicEffort         string            `json:"anthropic_effort"`
	CheckpointEnabled       bool              `json:"checkpoint_enabled"`
	OutputLanguage          string            `json:"output_language"`
	MaxDebateRounds         int               `json:"max_debate_rounds"`
	MaxRiskDiscussRounds    int               `json:"max_risk_discuss_rounds"`
	MaxRecurLimit           int               `json:"max_recur_limit"`
	AnalystConcurrencyLimit int               `json:"analyst_concurrency_limit"`
	NewsArticleLimit        int               `json:"news_article_limit"`
	GlobalNewsArticleLimit  int               `json:"global_news_article_limit"`
	GlobalNewsLookbackDays  int               `json:"global_news_lookback_days"`
	GlobalNewsQueries       []string          `json:"global_news_queries"`
	DataVendors             DataVendors       `json:"data_vendors"`
	ToolVendors             map[string]string `json:"tool_vendors"`
	BenchmarkTicker         string            `json:"benchmark_ticker"`
	BenchmarkMap            map[string]string `json:"benchmark_map"`
}

func LoadDefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	taHome := filepath.Join(home, ".tradingagents")

	c := &Config{
		ProjectDir:              ".",
		ResultsDir:              getEnvOr("TRADINGAGENTS_RESULTS_DIR", filepath.Join(taHome, "logs")),
		DataCacheDir:            getEnvOr("TRADINGAGENTS_CACHE_DIR", filepath.Join(taHome, "cache")),
		MemoryLogPath:           getEnvOr("TRADINGAGENTS_MEMORY_LOG_PATH", filepath.Join(taHome, "memory", "trading_memory.md")),
		MemoryLogMaxEntries:     nil,
		LLMProvider:             "openai",
		DeepThinkLLM:            "gpt-4o", // Upgraded to stable models
		QuickThinkLLM:           "gpt-4o-mini",
		CheckpointEnabled:       false,
		OutputLanguage:          "English",
		MaxDebateRounds:         1,
		MaxRiskDiscussRounds:    1,
		MaxRecurLimit:           100,
		AnalystConcurrencyLimit: 1,
		NewsArticleLimit:        20,
		GlobalNewsArticleLimit:  10,
		GlobalNewsLookbackDays:  7,
		GlobalNewsQueries: []string{
			"Federal Reserve interest rates inflation",
			"S&P 500 earnings GDP economic outlook",
			"geopolitical risk trade war sanctions",
			"ECB Bank of England BOJ central bank policy",
			"oil commodities supply chain energy",
		},
		DataVendors: DataVendors{
			CoreStockAPIs:       "yfinance",
			TechnicalIndicators: "yfinance",
			FundamentalData:     "yfinance",
			NewsData:            "yfinance",
		},
		ToolVendors: make(map[string]string),
		BenchmarkMap: map[string]string{
			".NS": "^NSEI",
			".BO": "^BSESN",
			".T":  "^N225",
			".HK": "^HSI",
			".L":  "^FTSE",
			".TO": "^GSPTSE",
			".AX": "^AXJO",
			"":    "SPY",
		},
	}

	applyEnvOverrides(c)
	return c
}
```

---

### 3.2. Package `llm` (LLM Provider Integrations)
**Python folder**: `/tradingagents/llm_clients/`

#### Architecture
The LLM layer defines a standard `Client` interface. We will implement providers (OpenAI, Gemini, Anthropic) using Go SDKs. 

#### Structure and Key Methods
```go
package llm

import "context"

type Message struct {
	Role    string // "system", "user", "assistant"
	Content string
}

type Client interface {
	Invoke(ctx context.Context, messages []Message) (string, error)
	InvokeStructured(ctx context.Context, messages []Message, responseSchema interface{}) error
}
```

- **OpenAI Client**: Wrap `github.com/sashabaranov/go-openai`. It natively supports JSON schema validation via structured outputs using `ResponseFormatTypeJSONObject`.
- **Gemini Client**: Wrap `github.com/google/generative-ai-go`. It maps structures to `ResponseSchema` configurations.
- **Anthropic Client**: Wrap `github.com/liushuangls/go-anthropic`. It uses tool-use configurations to enforce JSON structures when required.

---

### 3.3. Package `dataflows` (External APIs and Technical Indicators)
**Python folder**: `/tradingagents/dataflows/`

#### Yahoo Finance & Alpha Vantage Connectors
We will construct standard HTTP queries targeting the public REST endpoints:
- **Historical Prices**: `https://query1.finance.yahoo.com/v7/finance/download/{ticker}?period1={start_epoch}&period2={end_epoch}&interval=1d&events=history`
- **Fundamentals & Metrics**: Target `https://query2.finance.yahoo.com/v10/finance/quoteSummary/{ticker}?modules=assetProfile,financialData,defaultKeyStatistics,balanceSheetHistory,cashflowStatementHistory,incomeStatementHistory`
- **Reddit JSON Client**: Perform direct parsing of `https://www.reddit.com/r/{sub}/search.json` with a dedicated user agent header.
- **StockTwits Stream**: Call `https://api.stocktwits.com/api/2/streams/symbol/{ticker}.json`.

#### Mathematical Technical Indicators (Replacing `stockstats`)
We will write lightweight functions in Go to compute indicators over a slice of OHLCV rows (without loading entire Python Pandas runtimes):
- **SMA**: Simple rolling average of size $N$.
- **EMA**: Cumulative exponential weighting:
  $$\text{EMA}_t = \text{Price}_t \times \alpha + \text{EMA}_{t-1} \times (1 - \alpha), \quad \alpha = \frac{2}{N+1}$$
- **RSI**: Compute relative strength index by tracking rolling gains/losses.
- **MACD**: Difference between EMA(12) and EMA(26), with a Signal Line of EMA(9).
- **Bollinger Bands**: SMA(20) plus/minus 2 standard deviations.

---

### 3.4. Package `agents` (Structured Schemas and Nodes)
**Python folder**: `/tradingagents/agents/`

#### Go Equivalents of Pydantic Schemas (`schemas.go`)
Pydantic structures are converted to standard Go structs with JSON tags matching the key schemas:

```go
package agents

type PortfolioRating string

const (
	Buy         PortfolioRating = "Buy"
	Overweight  PortfolioRating = "Overweight"
	Hold        PortfolioRating = "Hold"
	Underweight PortfolioRating = "Underweight"
	Sell        PortfolioRating = "Sell"
)

type ResearchPlan struct {
	Recommendation   PortfolioRating `json:"recommendation"`
	Rationale        string          `json:"rationale"`
	StrategicActions string          `json:"strategic_actions"`
}

type TraderProposal struct {
	Action         string   `json:"action"` // "Buy", "Hold", "Sell"
	Reasoning      string   `json:"reasoning"`
	EntryPrice     *float64 `json:"entry_price,omitempty"`
	StopLoss       *float64 `json:"stop_loss,omitempty"`
	PositionSizing string   `json:"position_sizing,omitempty"`
}

type PortfolioDecision struct {
	Rating           PortfolioRating `json:"rating"`
	ExecutiveSummary string          `json:"executive_summary"`
	InvestmentThesis string          `json:"investment_thesis"`
	PriceTarget      *float64        `json:"price_target,omitempty"`
	TimeHorizon      string          `json:"time_horizon,omitempty"`
}
```

#### Structured Fallbacks (`structured.go`)
The `InvokeStructuredOrFreetext` pattern uses reflection or type-assertion in Go. If structured invocation fails, it logs a warning and falls back to a plain string invocation, executing regular expression extractions to attempt recovery.

#### Memory Log Manager (`memory.go`)
`TradingMemoryLog` parses and writes standard markdown files. We will translate this using Go's `os` package for atomic writes (writing a `.tmp` file and renaming it using `os.Rename`) and `regexp` for entry matching.

```go
package agents

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type MemoryEntry struct {
	Date       string
	Ticker     string
	Rating     string
	Pending    bool
	RawReturn  string
	Alpha      string
	Holding    string
	Decision   string
	Reflection string
}

type TradingMemoryLog struct {
	LogPath    string
	MaxEntries int
}

func (l *TradingMemoryLog) StoreDecision(ticker, date, decision string) error {
	// Implements append-only store with file write locks
	return nil
}
```

---

### 3.5. Package `graph` (State Orchestration Engine)
**Python folder**: `/tradingagents/graph/`

#### State Machine & Context (`states.go`)
`AgentState` is defined as a concrete, mutable Go struct passed by reference through the graph nodes:

```go
package graph

import "sync"

type InvestDebateState struct {
	BullHistory     string
	BearHistory     string
	History         string
	CurrentResponse string
	JudgeDecision   string
	Count           int
}

type RiskDebateState struct {
	AggressiveHistory           string
	ConservativeHistory         string
	NeutralHistory              string
	History                     string
	LatestSpeaker               string
	CurrentAggressiveResponse   string
	CurrentConservativeResponse string
	CurrentNeutralResponse      string
	JudgeDecision               string
	Count                       int
}

type AgentState struct {
	Mu                    sync.RWMutex
	CompanyOfInterest     string
	AssetType             string
	TradeDate             string
	Sender                string
	MarketReport          string
	SentimentReport       string
	NewsReport            string
	FundamentalsReport    string
	InvestmentDebateState InvestDebateState
	InvestmentPlan        string
	TraderInvestmentPlan  string
	RiskDebateState       RiskDebateState
	FinalTradeDecision    string
	PastContext           string
	Messages              []string // Emulates LangGraph message stack
}
```

#### Graph Sequential Wiring (`setup.go` & `propagation.go`)
Since LangGraph is bypassed, we will build a custom router that executes nodes in sequence, evaluating branching conditions via `conditions.go` after each step.

```go
package graph

type NodeFunc func(state *AgentState) error

type Graph struct {
	nodes      map[string]NodeFunc
	checkpoint *Checkpointer
}

func (g *Graph) Setup(selectedAnalysts []string) {
	// Register Market, Sentiment, News, Fundamentals analysts sequentially.
	// Map Bull/Bear Debaters, Research Manager, Trader, and Risk Analysts.
}

func (g *Graph) Run(state *AgentState) error {
	// Sequential execution loops matching setup.py.
	// Recovers from last saved step if checkpointing is active.
	return nil
}
```

#### SQLite Checkpointer (`checkpointer.go`)
Uses `modernc.org/sqlite` (no CGO required) to maintain an execution state database:
```sql
CREATE TABLE IF NOT EXISTS checkpoints (
    thread_id TEXT PRIMARY KEY,
    step_index INTEGER,
    state_json TEXT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```
Before a node executes, the graph serializes `AgentState` to JSON and writes it to SQLite. If a step fails, the next execution with the same ticker and date detects the row, deserializes the state, and resumes execution from that `step_index`.

---

### 3.6. Package `cli` and Entrypoints
**Python folder**: `/cli/` & `main.py`

- `/main.go` will initialize a simple, direct run of `TradingAgentsGraph` for an asset and date, outputting logs directly.
- `/cli/cli.go` will be compiled into the `tradingagents` binary using `github.com/spf13/cobra`.
- Terminal output formatting will use `github.com/charmbracelet/lipgloss` for elegant, structured headers and table layout rendering, matching the Python `rich` display.
- Dynamic menus will be implemented using `github.com/manifoldco/promptui` to offer multi-select options for active analysts and settings overrides.

---

## 4. Verification & Testing Plan

### Automated Integration Tests
1. **Mock HTTP Server Test**: Create Go unit tests that spin up `httptest.NewServer` to serve mock stock history, Reddit feeds, and StockTwits JSON payloads, asserting that parser components map data correctly.
2. **LLM Driver Test**: Build regression tests using environment variables for active models to verify that JSON schema boundaries are parsed correctly across Gemini, OpenAI, and Anthropic.
3. **Checkpointer Recovery Test**: Intentionally insert a panic into the 3rd node of execution, verify that state is successfully written to SQLite, resolve the panic, and verify that the subsequent run starts precisely at the 3rd node instead of rebuilding historical inputs.

### Manual Verification
1. Compile the tool to a single binary:
   ```bash
   go build -o tradingagents main.go
   ```
2. Execute the CLI with a target test run and evaluate stdout styling:
   ```bash
   ./tradingagents run --ticker TSLA --date 2026-05-15 --analysts market,sentiment
   ```
3. Inspect and verify the markdown formatting inside the created `trading_memory.md` log file.
