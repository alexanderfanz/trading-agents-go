package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"trading-agents-go/internal/dataflow"
	"trading-agents-go/pkg/provider"
)

func TestResolveBenchmark(t *testing.T) {
	tests := []struct {
		name            string
		ticker          string
		customBenchmark string
		want            string
	}{
		{name: "Tokyo suffix", ticker: "7203.T", customBenchmark: "", want: "^N225"},
		{name: "US ticker default", ticker: "AAPL", customBenchmark: "", want: "SPY"},
		{name: "custom override", ticker: "7203.T", customBenchmark: "QQQ", want: "QQQ"},
		{name: "unknown suffix", ticker: "FOO.BAR", customBenchmark: "", want: "SPY"},
		{name: "NSE suffix", ticker: "RELIANCE.NS", customBenchmark: "", want: "^NSEI"},
		{name: "Hong Kong suffix", ticker: "0005.HK", customBenchmark: "", want: "^HSI"},
		{name: "lowercase ticker normalized", ticker: "7203.t", customBenchmark: "", want: "^N225"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBenchmark(tt.ticker, tt.customBenchmark)
			if got != tt.want {
				t.Errorf("ResolveBenchmark(%q, %q) = %q, want %q", tt.ticker, tt.customBenchmark, got, tt.want)
			}
		})
	}
}

type capturingLLMProvider struct {
	lastReq provider.LLMRequest
	reply   string
	err     error
}

func (m *capturingLLMProvider) Generate(ctx context.Context, req provider.LLMRequest) (string, error) {
	m.lastReq = req
	return m.reply, m.err
}

func (m *capturingLLMProvider) GenerateStructured(ctx context.Context, req provider.LLMRequest, target interface{}) error {
	return nil
}

func TestReflectOnFinalDecision(t *testing.T) {
	t.Run("formats prompts and returns LLM reply", func(t *testing.T) {
		mock := &capturingLLMProvider{reply: "Reflection prose."}
		reflector := NewReflector(mock)

		decision := "Rating: Buy\nConviction on growth."
		got, err := reflector.ReflectOnFinalDecision(context.Background(), decision, 0.10, 0.05, "SPY")
		if err != nil {
			t.Fatalf("ReflectOnFinalDecision: %v", err)
		}
		if got != "Reflection prose." {
			t.Errorf("reply = %q, want %q", got, "Reflection prose.")
		}
		if mock.lastReq.SystemPrompt != ReflectionPrompt {
			t.Errorf("SystemPrompt mismatch:\n%s", mock.lastReq.SystemPrompt)
		}
		if mock.lastReq.Temperature != 0.2 {
			t.Errorf("Temperature = %v, want 0.2", mock.lastReq.Temperature)
		}

		user := mock.lastReq.UserPrompt
		for _, want := range []string{"+10.0%", "+5.0%", "SPY", decision} {
			if !strings.Contains(user, want) {
				t.Errorf("UserPrompt missing %q:\n%s", want, user)
			}
		}
		if !strings.Contains(user, "Raw return:") || !strings.Contains(user, "Alpha vs") || !strings.Contains(user, "Final Decision:") {
			t.Errorf("UserPrompt missing expected sections:\n%s", user)
		}
	})

	t.Run("negative returns formatted", func(t *testing.T) {
		mock := &capturingLLMProvider{reply: "ok"}
		reflector := NewReflector(mock)

		_, err := reflector.ReflectOnFinalDecision(context.Background(), "Rating: Sell", -0.03, -0.07, "^N225")
		if err != nil {
			t.Fatalf("ReflectOnFinalDecision: %v", err)
		}
		user := mock.lastReq.UserPrompt
		if !strings.Contains(user, "-3.0%") || !strings.Contains(user, "-7.0%") {
			t.Errorf("expected negative percent formatting in prompt:\n%s", user)
		}
	})

	t.Run("propagates Generate error", func(t *testing.T) {
		genErr := errors.New("llm unavailable")
		mock := &capturingLLMProvider{err: genErr}
		reflector := NewReflector(mock)

		_, err := reflector.ReflectOnFinalDecision(context.Background(), "Rating: Hold", 0.0, 0.0, "SPY")
		if !errors.Is(err, genErr) {
			t.Fatalf("expected %v, got %v", genErr, err)
		}
	})
}

func TestResolvePendingEntries_edgeCases(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "reflector_edge_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logFile := filepath.Join(tmpDir, "test_memory.md")
	log := NewTradingMemoryLog(logFile, 0)
	reflector := NewReflector(&mockLLMProvider{reply: "reflection"})
	ctx := context.Background()

	t.Run("no pending for ticker", func(t *testing.T) {
		_ = log.StoreDecision("AAPL", "2026-05-10", "Rating: Buy\nBuy AAPL.")
		if err := ResolvePendingEntries(ctx, "MSFT", log, reflector, &mockDataProvider{}, ""); err != nil {
			t.Fatalf("ResolvePendingEntries: %v", err)
		}
		entries, _ := log.LoadEntries()
		if len(entries) != 1 || !entries[0].Pending {
			t.Fatalf("expected unchanged pending AAPL entry, got %+v", entries)
		}
	})

	t.Run("skips trade still within holding period", func(t *testing.T) {
		recent := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		_ = log.StoreDecision("TSLA", recent, "Rating: Buy\nRecent trade.")
		if err := ResolvePendingEntries(ctx, "TSLA", log, reflector, &mockDataProvider{}, "SPY"); err != nil {
			t.Fatalf("ResolvePendingEntries: %v", err)
		}
		entries, _ := log.LoadEntries()
		for _, e := range entries {
			if e.Ticker == "TSLA" && e.Date == recent && !e.Pending {
				t.Fatalf("expected recent TSLA entry to remain pending, got %+v", e)
			}
		}
	})

	t.Run("skips invalid trade date", func(t *testing.T) {
		_ = log.StoreDecision("NVDA", "not-a-date", "Rating: Buy\nBad date.")
		if err := ResolvePendingEntries(ctx, "NVDA", log, reflector, &mockDataProvider{}, "SPY"); err != nil {
			t.Fatalf("ResolvePendingEntries: %v", err)
		}
	})

	t.Run("skips when data provider errors", func(t *testing.T) {
		oldDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		_ = log.StoreDecision("AMD", oldDate, "Rating: Buy\nOld trade.")
		dp := &mockDataProvider{err: errors.New("market data down")}
		if err := ResolvePendingEntries(ctx, "AMD", log, reflector, dp, "SPY"); err != nil {
			t.Fatalf("ResolvePendingEntries: %v", err)
		}
	})
}

func TestResolvePendingEntries_resolvesJapanTicker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "reflector_jp_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logFile := filepath.Join(tmpDir, "test_memory.md")
	log := NewTradingMemoryLog(logFile, 0)

	tradeDateStr := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	tradeDate, _ := time.Parse("2006-01-02", tradeDateStr)
	_ = log.StoreDecision("7203.T", tradeDateStr, "Rating: Buy\nBuy Toyota.")

	stockCandles := make([]dataflow.Candle, 12)
	benchCandles := make([]dataflow.Candle, 12)
	for i := 0; i < 12; i++ {
		day := tradeDate.AddDate(0, 0, i)
		stockCandles[i] = dataflow.Candle{Time: day, Close: 200.0 + float64(i)}
		benchCandles[i] = dataflow.Candle{Time: day, Close: 30000.0 + float64(i*10)}
	}

	mockDP := &mockDataProvider{stockCandles: stockCandles, benchCandles: benchCandles}
	mockLLM := &mockLLMProvider{reply: "Toyota thesis held."}
	reflector := NewReflector(mockLLM)

	if err := ResolvePendingEntries(context.Background(), "7203.T", log, reflector, mockDP, ""); err != nil {
		t.Fatalf("ResolvePendingEntries: %v", err)
	}

	entries, _ := log.LoadEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Pending {
		t.Fatal("expected 7203.T entry to be resolved")
	}
	if entries[0].Reflection != "Toyota thesis held." {
		t.Errorf("reflection = %q", entries[0].Reflection)
	}
}
