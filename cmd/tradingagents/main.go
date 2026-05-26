package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"trading-agents-go/internal/app"
	"trading-agents-go/internal/cli"
	"trading-agents-go/internal/config"
	"trading-agents-go/internal/interactive"
)

var (
	version   = "unknown"
	buildDate = "unknown"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiCyan  = "\x1b[36m"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func printBanner(subtitle string) {
	border := "+-------------------------------------+"
	title := "TradingAgents Go Orchestrator"

	fmt.Println()
	fmt.Println()

	if stdoutSupportsColor() {
		fmt.Printf("%s%s%s\n", ansiCyan, border, ansiReset)
		fmt.Printf("%s|%s %s%-35s%s %s|%s\n", ansiCyan, ansiReset, ansiBold, title, ansiReset, ansiCyan, ansiReset)
		fmt.Printf("%s|%s %-35s %s|%s\n", ansiCyan, ansiReset, subtitle, ansiCyan, ansiReset)
		fmt.Printf("%s%s%s\n", ansiCyan, border, ansiReset)
	} else {
		fmt.Println(border)
		fmt.Printf("| %-35s |\n", title)
		fmt.Printf("| %-35s |\n", subtitle)
		fmt.Println(border)
	}

	fmt.Println()
	fmt.Printf("Version:    %s\n", version)
	fmt.Printf("Build date: %s\n", buildDate)
}

func printHelpHeader() {
	printBanner("Usage Guide")
	fmt.Println()
	fmt.Println("Options:")
}

func printRunHeader() {
	printBanner("Analysis Run")
	fmt.Println()
}

func printInteractiveHeader() {
	printBanner("Interactive Setup")
	fmt.Println()
}

func stdoutSupportsColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	return term.IsTerminal(int(os.Stdout.Fd()))
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
	interactiveMode := flags.Bool("interactive", false, "Launch guided interactive mode")
	help := flags.Bool("h", false, "Show help usage description")
	flags.BoolVar(help, "help", false, "Show help usage description")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *help {
		printHelpHeader()
		flags.SetOutput(os.Stdout)
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

	cliController := cli.NewCLIController()

	opts := app.RunOptions{
		Ticker:    *ticker,
		TradeDate: *tradeDate,
		DBPath:    *dbPath,
	}

	if *interactiveMode {
		printInteractiveHeader()

		var err error
		opts, err = interactive.PromptForRunOptions(cfg, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "interactive mode cancelled: %v\n", err)
			return 1
		}
	}

	printRunHeader()
	return app.Run(cfg, opts, cliController)
}
