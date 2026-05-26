package interactive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"trading-agents-go/internal/app"
	"trading-agents-go/internal/config"
)

var (
	// ErrNonInteractive is returned when a guided prompt is requested where it
	// cannot safely read and render terminal UI.
	ErrNonInteractive = errors.New("interactive mode requires a TTY stdin and stdout")

	tickerPattern = regexp.MustCompile(`^[A-Za-z0-9._\-\^]{1,32}$`)
)

const providerMock = "mock"

// PromptForRunOptions collects manual run settings and applies config overlays.
func PromptForRunOptions(cfg *config.Config, defaults app.RunOptions) (app.RunOptions, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return app.RunOptions{}, ErrNonInteractive
	}

	formValues := newFormValues(cfg, defaults)
	if err := runForm(formValues); err != nil {
		return app.RunOptions{}, err
	}

	opts, err := formValues.applyTo(cfg)
	if err != nil {
		return app.RunOptions{}, err
	}
	return opts, nil
}

type formValues struct {
	ticker             string
	tradeDate          string
	outputLanguage     string
	provider           string
	quickThinkLLM      string
	deepThinkLLM       string
	researchDepth      int
	checkpointEnabled  bool
	createLocalReports bool
	localReportsDir    string
	resultsDir         string
	cacheDir           string
	memoryPath         string
	dbPath             string
	timeoutSeconds     string
}

func newFormValues(cfg *config.Config, defaults app.RunOptions) *formValues {
	return &formValues{
		ticker:             strings.ToUpper(defaultString(defaults.Ticker, "AAPL")),
		tradeDate:          defaultString(defaults.TradeDate, time.Now().Format("2006-01-02")),
		outputLanguage:     defaultString(cfg.OutputLanguage, "English"),
		provider:           strings.ToLower(defaultString(cfg.LLMProvider, "openai")),
		quickThinkLLM:      cfg.QuickThinkLLM,
		deepThinkLLM:       cfg.DeepThinkLLM,
		researchDepth:      max(cfg.MaxDebateRounds, cfg.MaxRiskDiscussRounds),
		checkpointEnabled:  cfg.CheckpointEnabled,
		createLocalReports: cfg.CreateLocalReports,
		localReportsDir:    cfg.LocalReportsDir,
		resultsDir:         cfg.ResultsDir,
		cacheDir:           cfg.DataCacheDir,
		memoryPath:         cfg.MemoryLogPath,
		dbPath:             defaultString(defaults.DBPath, filepath.Join(config.GetDefaultHome(), "checkpoints.db")),
		timeoutSeconds:     fmt.Sprintf("%d", cfg.ExecutionTimeout),
	}
}

func runForm(values *formValues) error {
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Ticker symbol").
				Description("Examples: AAPL, CNC.TO, 7203.T, 0700.HK, BTC-USD").
				Value(&values.ticker).
				Validate(validateTicker),
			huh.NewInput().
				Title("Analysis date").
				Description("Use YYYY-MM-DD. Defaults to today. Future dates are rejected.").
				Value(&values.tradeDate).
				Validate(validateTradeDate),
			huh.NewInput().
				Title("Output language").
				Value(&values.outputLanguage).
				Validate(validateRequired("output language")),
			huh.NewSelect[string]().
				Title("LLM provider").
				Value(&values.provider).
				Options(providerOptions()...),
		).Title("TradingAgents Interactive Setup"),
	).WithTheme(huh.ThemeCatppuccin()).Run(); err != nil {
		return err
	}

	values.applyModelDefaultsForProvider()

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Quick-thinking model").
				Value(&values.quickThinkLLM).
				Validate(validateRequired("quick-thinking model")),
			huh.NewInput().
				Title("Deep-thinking model").
				Value(&values.deepThinkLLM).
				Validate(validateRequired("deep-thinking model")),
			huh.NewSelect[int]().
				Title("Research depth").
				Description("Applies to both debate rounds and risk discussion rounds.").
				Value(&values.researchDepth).
				Options(
					huh.NewOption("Shallow - 1 round", 1),
					huh.NewOption("Medium - 3 rounds", 3),
					huh.NewOption("Deep - 5 rounds", 5),
				),
			huh.NewConfirm().
				Title("Enable checkpoint resume?").
				Value(&values.checkpointEnabled),
			huh.NewConfirm().
				Title("Generate local markdown reports?").
				Value(&values.createLocalReports),
			huh.NewInput().
				Title("Local reports directory").
				Value(&values.localReportsDir).
				Validate(validateRequired("local reports directory")),
		).Title("Models And Output"),
		huh.NewGroup(
			huh.NewInput().
				Title("Results log directory").
				Value(&values.resultsDir).
				Validate(validateRequired("results log directory")),
			huh.NewInput().
				Title("Data cache directory").
				Value(&values.cacheDir).
				Validate(validateRequired("data cache directory")),
			huh.NewInput().
				Title("Memory log path").
				Value(&values.memoryPath).
				Validate(validateRequired("memory log path")),
			huh.NewInput().
				Title("Checkpoint database path").
				Value(&values.dbPath).
				Validate(validateRequired("checkpoint database path")),
			huh.NewInput().
				Title("Execution timeout in seconds").
				Value(&values.timeoutSeconds).
				Validate(validatePositiveIntString("execution timeout")),
		).Title("Advanced Paths"),
	).WithTheme(huh.ThemeCatppuccin()).Run()
}

func (v *formValues) applyModelDefaultsForProvider() {
	modelDefaults := defaultModelsForProvider(v.provider)
	if modelDefaults.quick != "" {
		v.quickThinkLLM = modelDefaults.quick
	}
	if modelDefaults.deep != "" {
		v.deepThinkLLM = modelDefaults.deep
	}
}

func (v *formValues) applyTo(cfg *config.Config) (app.RunOptions, error) {
	if err := validateTicker(v.ticker); err != nil {
		return app.RunOptions{}, err
	}
	if err := validateTradeDate(v.tradeDate); err != nil {
		return app.RunOptions{}, err
	}
	timeoutSeconds, err := parsePositiveInt(v.timeoutSeconds, "execution timeout")
	if err != nil {
		return app.RunOptions{}, err
	}

	cfg.OutputLanguage = strings.TrimSpace(v.outputLanguage)
	cfg.LLMProvider = strings.ToLower(strings.TrimSpace(v.provider))
	cfg.QuickThinkLLM = strings.TrimSpace(v.quickThinkLLM)
	cfg.DeepThinkLLM = strings.TrimSpace(v.deepThinkLLM)
	cfg.MaxDebateRounds = v.researchDepth
	cfg.MaxRiskDiscussRounds = v.researchDepth
	cfg.CheckpointEnabled = v.checkpointEnabled
	cfg.CreateLocalReports = v.createLocalReports
	cfg.LocalReportsDir = strings.TrimSpace(v.localReportsDir)
	cfg.ResultsDir = strings.TrimSpace(v.resultsDir)
	cfg.DataCacheDir = strings.TrimSpace(v.cacheDir)
	cfg.MemoryLogPath = strings.TrimSpace(v.memoryPath)
	cfg.ExecutionTimeout = timeoutSeconds

	return app.RunOptions{
		Ticker:    strings.ToUpper(strings.TrimSpace(v.ticker)),
		TradeDate: strings.TrimSpace(v.tradeDate),
		DBPath:    strings.TrimSpace(v.dbPath),
	}, nil
}

func providerOptions() []huh.Option[string] {
	providers := []string{
		"openai",
		"gemini",
		"google",
		"anthropic",
		"azure",
		"xai",
		"deepseek",
		"qwen",
		"qwen-cn",
		"glm",
		"glm-cn",
		"minimax",
		"minimax-cn",
		"openrouter",
		"ollama",
		providerMock,
	}

	options := make([]huh.Option[string], 0, len(providers))
	for _, p := range providers {
		options = append(options, huh.NewOption(p, p))
	}
	return options
}

type modelDefaults struct {
	quick string
	deep  string
}

func defaultModelsForProvider(provider string) modelDefaults {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return modelDefaults{quick: "gpt-4o-mini", deep: "gpt-4o"}
	case "gemini", "google":
		return modelDefaults{quick: "gemini-3.5-flash", deep: "gemini-3.5-flash"}
	case "anthropic":
		return modelDefaults{quick: "claude-3-7-sonnet", deep: "claude-3-7-sonnet"}
	case "azure":
		return modelDefaults{quick: "gpt-4", deep: "gpt-4"}
	case "xai":
		return modelDefaults{quick: "grok-4.20-reasoner", deep: "grok-4.20-reasoner"}
	case "deepseek":
		return modelDefaults{quick: "deepseek-reasoner", deep: "deepseek-reasoner"}
	case "qwen", "qwen-cn":
		return modelDefaults{quick: "qwen3.6-plus", deep: "qwen3.6-plus"}
	case "glm", "glm-cn":
		return modelDefaults{quick: "glm-5", deep: "glm-5"}
	case "minimax", "minimax-cn":
		return modelDefaults{quick: "MiniMax-M2.7", deep: "MiniMax-M2.7"}
	case "openrouter":
		return modelDefaults{quick: "meta-llama/llama-3", deep: "meta-llama/llama-3"}
	case "ollama":
		return modelDefaults{quick: "qwen3:latest", deep: "qwen3:latest"}
	case providerMock:
		return modelDefaults{quick: providerMock, deep: providerMock}
	default:
		return modelDefaults{}
	}
}

func validateTicker(value string) error {
	ticker := strings.TrimSpace(value)
	if ticker == "" {
		return fmt.Errorf("ticker is required")
	}
	if !tickerPattern.MatchString(ticker) {
		return fmt.Errorf("ticker may only contain letters, numbers, dots, underscores, dashes, and carets")
	}
	return nil
}

func validateTradeDate(value string) error {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("date must use YYYY-MM-DD")
	}
	if date.After(time.Now()) {
		return fmt.Errorf("analysis date cannot be in the future")
	}
	return nil
}

func validateRequired(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

func validatePositiveIntString(label string) func(string) error {
	return func(value string) error {
		_, err := parsePositiveInt(value, label)
		return err
	}
}

func parsePositiveInt(value, label string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", label)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", label)
	}
	return parsed, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
