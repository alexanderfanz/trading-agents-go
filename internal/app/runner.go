package app

import (
	"context"
	"errors"
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

// RunOptions contains per-run values that are independent from environment
// defaults. Both flag parsing and interactive prompts should converge here.
type RunOptions struct {
	Ticker    string
	TradeDate string
	DBPath    string
}

// Run executes the full trading workflow with an already-populated config.
func Run(cfg *config.Config, opts RunOptions, cliController *cli.CLIController) int {
	ticker := strings.ToUpper(opts.Ticker)
	if ticker == "" {
		ticker = "AAPL"
	}

	if cliController.IsTTY {
		fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
			fmt.Sprintf("🚀 STARTING AI TRADING AGENT WORKFLOW FOR %s ON %s", ticker, opts.TradeDate),
		))
	} else {
		fmt.Printf("[INFO] Starting AI Trading Agent workflow for %s on %s\n", ticker, opts.TradeDate)
	}

	llmProvider := initializeLLMProvider(cfg, ticker, cliController)

	_ = os.MkdirAll(filepath.Dir(opts.DBPath), 0750)

	dbMgr, err := checkpoint.NewSQLConnectionManager(opts.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite checkpoint connection pool: %v", err)
	}
	defer func() {
		_ = dbMgr.Close()
	}()

	checkpointer := checkpoint.NewStateCheckpointer(dbMgr)

	cleanupWorker := checkpoint.NewCleanupWorker(dbMgr, 1*time.Hour, 10*1024*1024, 7)
	cleanupWorker.Start(context.Background())
	defer cleanupWorker.Stop()

	indicatorCache := indicators.NewIndicatorCache()
	indicatorResolver := indicators.NewDynamicIndicatorResolver(indicatorCache)

	tokenBucket := dataflow.NewTokenBucket(5.0, 2.0)
	httpClient := dataflow.NewResilientHTTPClient(http.DefaultClient, tokenBucket, 3, 200*time.Millisecond, 2*time.Second)
	dataReader := dataflow.NewYahooFinanceCSVReader(httpClient, cfg.DataCacheDir)
	newsSocialProvider := dataflow.NewHTTPNewsSocialProvider(httpClient)

	orch := orchestrator.NewTradingOrchestrator(cfg, checkpointer, dataReader, llmProvider, indicatorResolver, newsSocialProvider)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ExecutionTimeout)*time.Second)
	defer cancel()

	finalDecision, err := orch.Execute(timeoutCtx, ticker, opts.TradeDate, cliController)
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

	if cliController.IsTTY {
		fmt.Println(cli.GetDynamicBorderStyle(cli.StateBullish, cliController.Theme).Render(
			fmt.Sprintf("✨ WORKFLOW COMPLETE FOR %s ON %s\n\nFinal Portfolio Sizing Decision:\n%s", ticker, opts.TradeDate, finalDecision),
		))
	} else {
		fmt.Printf("[SUCCESS] Workflow complete for %s on %s. Final Decision:\n%s\n", ticker, opts.TradeDate, finalDecision)
	}

	return 0
}

func initializeLLMProvider(cfg *config.Config, ticker string, cliController *cli.CLIController) provider.LLMProvider {
	llmProvider, initErr := provider.NewLLMProvider(cfg.LLMProvider, cfg.DeepThinkLLM, cfg.BackendURL, cfg.ResultsDir)
	if initErr == nil {
		return llmProvider
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.LLMProvider = "openai"
		llmProvider = provider.NewOpenAIAdapter(key, cfg.DeepThinkLLM, cfg.ResultsDir)
		if cliController.IsTTY {
			fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
				"💡 Auto-detected OPENAI_API_KEY in environment. Using OpenAI Provider.",
			))
		}
		return llmProvider
	}

	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
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
		return llmProvider
	}

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.LLMProvider = "anthropic"
		llmProvider = provider.NewAnthropicAdapter(key, cfg.DeepThinkLLM, cfg.ResultsDir)
		if cliController.IsTTY {
			fmt.Println(cli.GetDynamicBorderStyle(cli.StateSystemAction, cliController.Theme).Render(
				"💡 Auto-detected ANTHROPIC_API_KEY in environment. Using Anthropic Provider.",
			))
		}
		return llmProvider
	}

	cfg.LLMProvider = "mock"
	llmProvider = provider.NewMockProvider(ticker)
	if cliController.IsTTY {
		fmt.Println(cli.GetDynamicBorderStyle(cli.StateRiskEscalation, cliController.Theme).Render(
			"⚠️  No API keys found in environment.\n" +
				"Auto-defaulting to dynamic dry-run MOCK LLM simulation mode.",
		))
	} else {
		fmt.Println("[WARN] No API keys found in environment. Defaulting to dry-run MOCK LLM simulation mode.")
	}

	return llmProvider
}
