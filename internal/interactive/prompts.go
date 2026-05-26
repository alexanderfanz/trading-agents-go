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

const (
	providerGemini  = "gemini"
	providerQwen    = "qwen"
	providerGLM     = "glm"
	providerMinimax = "minimax"
	providerMock    = "mock"

	modelGPT55  = "gpt-5.5"
	modelO4Mini = "o4-mini"
)

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
			huh.NewSelect[string]().
				Title("Quick-thinking model").
				Description(modelSelectDescription(values.provider, "quick")).
				Value(&values.quickThinkLLM).
				Options(modelOptionsForProvider(values.provider, modelCategoryQuick)...).
				Validate(validateRequired("quick-thinking model")),
			huh.NewSelect[string]().
				Title("Deep-thinking model").
				Description(modelSelectDescription(values.provider, "deep")).
				Value(&values.deepThinkLLM).
				Options(modelOptionsForProvider(values.provider, modelCategoryDeep)...).
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
		providerGemini,
		"openai",
		"google",
		"anthropic",
		"azure",
		"xai",
		"deepseek",
		providerQwen,
		"qwen-cn",
		providerGLM,
		"glm-cn",
		providerMinimax,
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

type modelCategory string

const (
	modelCategoryQuick modelCategory = "quick"
	modelCategoryDeep  modelCategory = "deep"
)

type providerModelCatalog struct {
	quick []string
	deep  []string
}

var modelCatalogByProvider = map[string]providerModelCatalog{
	"openai": {
		quick: []string{
			"gpt-5.4-nano",
			"gpt-5.4-mini",
			modelO4Mini,
			"gpt-5",
			modelGPT55,
		},
		deep: []string{
			modelGPT55,
			"gpt-5.5-pro",
			"gpt-5.4",
			"gpt-5.4-pro",
			"o3",
			modelO4Mini,
		},
	},
	providerGemini: {
		quick: []string{
			"gemini-3.5-flash",
			"gemini-3-flash-preview",
			"gemini-3.1-flash-lite",
			"gemini-2.5-flash",
		},
		deep: []string{
			"gemini-3.1-pro-preview",
			"gemini-3.1-pro-preview-customtools",
			"gemini-3.5-flash",
			"gemini-2.5-pro",
		},
	},
	"anthropic": {
		quick: []string{
			"claude-sonnet-4-6",
			"claude-haiku-4-5",
			"claude-sonnet-4-5",
			"claude-opus-4-6",
		},
		deep: []string{
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
			"claude-sonnet-4-5",
		},
	},
	"azure": {
		quick: []string{
			"gpt-5.4-nano",
			"gpt-5.4-mini",
			modelO4Mini,
			"gpt-5",
			modelGPT55,
		},
		deep: []string{
			modelGPT55,
			"gpt-5.5-pro",
			"gpt-5.4",
			"gpt-5.4-pro",
			"o3",
			modelO4Mini,
		},
	},
	"xai": {
		quick: []string{
			"grok-4.1-fast",
			"grok-4-fast",
			"grok-4-mini",
		},
		deep: []string{
			"grok-4.20",
			"grok-4",
			"grok-4-heavy",
		},
	},
	"deepseek": {
		quick: []string{
			"deepseek-v4-flash",
			"deepseek-v4",
			"deepseek-chat",
		},
		deep: []string{
			"deepseek-v4-pro",
			"deepseek-reasoner",
			"deepseek-r1",
		},
	},
	providerQwen: {
		quick: []string{
			"qwen3.6-flash",
			"qwen3.6-30b-a3b",
			"qwen3.5-turbo",
		},
		deep: []string{
			"qwen3.7-max",
			"qwen3.6-max-preview",
			"qwen3-max-thinking",
			"qwen3.6-235b-a22b",
		},
	},
	providerGLM: {
		quick: []string{
			"glm-5.1-highspeed",
			"glm-5.1-air",
			"glm-5.1-flash",
		},
		deep: []string{
			"glm-5.1",
			"glm-5.1-plus",
			"glm-5",
		},
	},
	providerMinimax: {
		quick: []string{
			"MiniMax-M2.7-highspeed",
			"MiniMax-M2.5",
			"MiniMax-Text-01",
		},
		deep: []string{
			"MiniMax-M2.7",
			"MiniMax-M2.5-Pro",
			"MiniMax-01",
		},
	},
	"openrouter": {
		quick: []string{
			"google/gemini-3.5-flash",
			"openai/gpt-5.4-nano",
			"anthropic/claude-sonnet-4.6",
			"deepseek/deepseek-v4-flash",
			"qwen/qwen3.6-flash",
			"x-ai/grok-4.1-fast",
		},
		deep: []string{
			"anthropic/claude-opus-4.7",
			"openai/gpt-5.5",
			"google/gemini-3.1-pro",
			"deepseek/deepseek-v4-pro",
			"qwen/qwen3.7-max",
			"z-ai/glm-5.1",
		},
	},
	"ollama": {
		quick: []string{
			"qwen3:4b",
			"gemma3:4b",
			"llama3.2:3b",
			"phi4-mini",
		},
		deep: []string{
			"qwen3:32b",
			"deepseek-r1:32b",
			"gemma3:27b",
			"llama4:scout",
		},
	},
	providerMock: {
		quick: []string{providerMock},
		deep:  []string{providerMock},
	},
}

func defaultModelsForProvider(provider string) modelDefaults {
	catalog, ok := modelCatalogForProvider(provider)
	if !ok {
		return modelDefaults{}
	}
	return modelDefaults{quick: catalog.quick[0], deep: catalog.deep[0]}
}

func modelOptionsForProvider(provider string, category modelCategory) []huh.Option[string] {
	catalog, ok := modelCatalogForProvider(provider)
	if !ok {
		return nil
	}

	var models []string
	switch category {
	case modelCategoryQuick:
		models = catalog.quick
	case modelCategoryDeep:
		models = catalog.deep
	default:
		return nil
	}

	options := make([]huh.Option[string], 0, len(models))
	for _, model := range models {
		options = append(options, huh.NewOption(model, model))
	}
	return options
}

func modelCatalogForProvider(provider string) (providerModelCatalog, bool) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "google":
		normalized = providerGemini
	case "qwen-cn":
		normalized = providerQwen
	case "glm-cn":
		normalized = providerGLM
	case "minimax-cn":
		normalized = providerMinimax
	}
	catalog, ok := modelCatalogByProvider[normalized]
	return catalog, ok
}

func modelSelectDescription(provider, category string) string {
	return fmt.Sprintf("Curated %s choices for %s as of May 2026. Use CLI/env vars for custom model IDs.", category, strings.ToLower(strings.TrimSpace(provider)))
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
