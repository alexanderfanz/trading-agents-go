package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"trading-agents-go/internal/checkpoint"
	"trading-agents-go/internal/cli"
	"trading-agents-go/internal/config"
	"trading-agents-go/internal/dataflow"
	"trading-agents-go/internal/indicators"
	"trading-agents-go/internal/orchestrator"
	"trading-agents-go/pkg/provider"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// 1. Load base configurations from environment
	cfg := config.LoadConfig()

	flags := flag.NewFlagSet("tradingagents", flag.ContinueOnError)

	// 2. Define Command-Line Flags
	ticker := flags.String("ticker", "AAPL", "Ticker symbol to analyze")
	tradeDate := flags.String("trade-date", time.Now().Format("2006-01-02"), "Date of trade analysis (YYYY-MM-DD)")
	providerFlag := flags.String("provider", cfg.LLMProvider, "LLM provider (openai, gemini, anthropic, mock)")
	deepThinkLLM := flags.String("deep-think-llm", cfg.DeepThinkLLM, "LLM model for deep/logical reasoning")
	quickThinkLLM := flags.String("quick-think-llm", cfg.QuickThinkLLM, "LLM model for quick sentiment/news analysis")
	maxDebateRounds := flags.Int("max-debate-rounds", cfg.MaxDebateRounds, "Max turns for bull/bear research debate")
	maxRiskRounds := flags.Int("max-risk-rounds", cfg.MaxRiskDiscussRounds, "Max turns for risk appetite debate")
	checkpointEnabled := flags.Bool("enable-checkpoint", cfg.CheckpointEnabled, "Enable checkpointing of intermediate steps")
	resultsDir := flags.String("results-dir", cfg.ResultsDir, "Directory to write results logs")
	cacheDir := flags.String("cache-dir", cfg.DataCacheDir, "Directory to cache downloaded finance data")
	memoryPath := flags.String("memory-path", cfg.MemoryLogPath, "Path to write cumulative decision memory md")
	dbPath := flags.String("db-path", filepath.Join(config.GetDefaultHome(), "checkpoints.db"), "Path to sqlite checkpoints database")
	timeoutFlag := flags.Int("timeout", cfg.ExecutionTimeout, "Master execution timeout boundary in seconds")
	createLocalReports := flags.Bool("enable-local-reports", cfg.CreateLocalReports, "Enable report generation in local directory")
	localReportsDir := flags.String("local-reports-dir", cfg.LocalReportsDir, "Directory to compile structured markdown reports")
	help := flags.Bool("h", false, "Show help usage description")
	flags.BoolVar(help, "help", false, "Show help usage description")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *help {
		fmt.Println("TradingAgents Go Orchestrator - Usage Guide")
		flags.PrintDefaults()
		return 0
	}

	// 3. Override configs with explicit Flags
	cfg.LLMProvider = strings.ToLower(*providerFlag)
	cfg.DeepThinkLLM = *deepThinkLLM
	cfg.QuickThinkLLM = *quickThinkLLM
	cfg.MaxDebateRounds = *maxDebateRounds
	cfg.MaxRiskDiscussRounds = *maxRiskRounds
	cfg.CheckpointEnabled = *checkpointEnabled
	cfg.ResultsDir = *resultsDir
	cfg.DataCacheDir = *cacheDir
	cfg.MemoryLogPath = *memoryPath
	cfg.ExecutionTimeout = *timeoutFlag
	cfg.CreateLocalReports = *createLocalReports
	cfg.LocalReportsDir = *localReportsDir

	// 4. Initialize Terminal Controller for Theme Styling & Piping checks
	cliController := cli.NewCLIController()

	if cliController.IsTTY {
		fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
			fmt.Sprintf("🚀 STARTING AI TRADING AGENT WORKFLOW FOR %s ON %s", strings.ToUpper(*ticker), *tradeDate),
		))
	} else {
		fmt.Printf("[INFO] Starting AI Trading Agent workflow for %s on %s\n", strings.ToUpper(*ticker), *tradeDate)
	}

	// 5. Initialize the LLM Client Provider with Auto-Downgrade Fallback
	var llmProvider provider.LLMProvider
	var initErr error

	llmProvider, initErr = provider.NewLLMProvider(cfg.LLMProvider, cfg.DeepThinkLLM, cfg.BackendURL, cfg.ResultsDir)
	if initErr != nil {
		// If explicit initialization fails, try intelligent key auto-detection
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			cfg.LLMProvider = "openai"
			llmProvider = provider.NewOpenAIAdapter(key, cfg.DeepThinkLLM, cfg.ResultsDir)
			if cliController.IsTTY {
				fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
					"💡 Auto-detected OPENAI_API_KEY in environment. Using OpenAI Provider.",
				))
			}
		} else if key := os.Getenv("GEMINI_API_KEY"); key != "" {
			cfg.LLMProvider = "gemini"
			llmProvider, initErr = provider.NewGeminiAdapter(key, cfg.DeepThinkLLM, cfg.ResultsDir)
			if initErr != nil {
				log.Fatalf("Error auto-configuring Gemini adapter: %v", initErr)
			}
			if cliController.IsTTY {
				fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
					"💡 Auto-detected GEMINI_API_KEY in environment. Using Gemini Provider.",
				))
			}
		} else if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			cfg.LLMProvider = "anthropic"
			llmProvider = provider.NewAnthropicAdapter(key, cfg.DeepThinkLLM, cfg.ResultsDir)
			if cliController.IsTTY {
				fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
					"💡 Auto-detected ANTHROPIC_API_KEY in environment. Using Anthropic Provider.",
				))
			}
		} else {
			// Seamless dry-run downgrade
			cfg.LLMProvider = "mock"
			llmProvider = provider.NewMockProvider(*ticker)
			if cliController.IsTTY {
				fmt.Println(cli.GetDynamicBorderStyle(cli.StateRiskEscalation, cliController.Theme).Render(
					"⚠️  No API keys found in environment.\n" +
						"Auto-defaulting to dynamic dry-run MOCK LLM simulation mode.",
				))
			} else {
				fmt.Println("[WARN] No API keys found in environment. Defaulting to dry-run MOCK LLM simulation mode.")
			}
		}
	}

	// 6. Ensure Parent Database Folders Exist
	_ = os.MkdirAll(filepath.Dir(*dbPath), 0750)

	// 7. Initialize Concurrency-Safe SQLite WAL Connection Managers
	dbMgr, err := checkpoint.NewSQLConnectionManager(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite checkpoint connection pool: %v", err)
	}
	defer func() {
		_ = dbMgr.Close()
	}()

	checkpointer := checkpoint.NewStateCheckpointer(dbMgr)

	// 8. Launch background CleanupWorker to prune database storage
	cleanupWorker := checkpoint.NewCleanupWorker(dbMgr, 1*time.Hour, 10*1024*1024, 7) // 10MB limit, 7-day retention
	cleanupWorker.Start(context.Background())
	defer cleanupWorker.Stop()

	// 9. Configure Core Indicator Caches & Resolvers
	indicatorCache := indicators.NewIndicatorCache()
	indicatorResolver := indicators.NewDynamicIndicatorResolver(indicatorCache)

	// 10. Establish Resilient Rate-Limited Yahoo Finance downloader
	tokenBucket := dataflow.NewTokenBucket(5.0, 2.0) // 5 concurrent requests capacity, refilling at 2 requests per second
	httpClient := dataflow.NewResilientHTTPClient(http.DefaultClient, tokenBucket, 3, 200*time.Millisecond, 2*time.Second)
	dataReader := dataflow.NewYahooFinanceCSVReader(httpClient, cfg.DataCacheDir)
	newsSocialProvider := dataflow.NewHTTPNewsSocialProvider(httpClient)

	// 11. Instantiate the Master Agent Orchestration Engine
	orch := orchestrator.NewTradingOrchestrator(cfg, checkpointer, dataReader, llmProvider, indicatorResolver, newsSocialProvider)

	// 12. Setup Signal Routing contexts for Graceful Interruption cancellations
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	// Setup master execution timeout boundary of dynamic duration
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ExecutionTimeout)*time.Second)
	defer cancel()

	// 13. Execute the workflow execution pipeline
	finalDecision, err := orch.Execute(timeoutCtx, *ticker, *tradeDate, cliController)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			if cliController.IsTTY {
				fmt.Println("\n" + cli.GetDynamicBorderStyle(cli.StateRiskEscalation, cliController.Theme).Render(
					"🛑 Workflow execution was gracefully interrupted or cancelled by the user.",
				))
			} else {
				fmt.Println("[ERROR] Workflow execution was cancelled.")
			}
			return 130
		}

		if cliController.IsTTY {
			fmt.Println("\n" + cli.GetDynamicBorderStyle(cli.StateBearish, cliController.Theme).Render(
				fmt.Sprintf("❌ Critical execution error: %v", err),
			))
		} else {
			fmt.Printf("[FATAL] Critical execution error: %v\n", err)
		}
		return 1
	}

	// 14. Print final Obsidian Card summary
	if cliController.IsTTY {
		fmt.Println(cli.GetDynamicBorderStyle(cli.StateBullish, cliController.Theme).Render(
			fmt.Sprintf("✨ WORKFLOW COMPLETE FOR %s ON %s\n\nFinal Portfolio Sizing Decision:\n%s", strings.ToUpper(*ticker), *tradeDate, finalDecision),
		))
	} else {
		fmt.Printf("[SUCCESS] Workflow complete for %s on %s. Final Decision:\n%s\n", strings.ToUpper(*ticker), *tradeDate, finalDecision)
	}
	return 0
}
