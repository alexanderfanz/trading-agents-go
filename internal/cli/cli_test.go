package cli

import (
	"context"
	"testing"
	"time"
)

func TestCLIThemeAndRendering(t *testing.T) {
	theme := NewObsidianTheme()

	// Assert colors
	if theme.BullAccent == "" || theme.BearAccent == "" {
		t.Errorf("expected theme accents to be populated")
	}

	// Verify dynamic border style can be created
	style := GetDynamicBorderStyle(StateBullish, theme)
	_ = style

	// Verify TechnicalIndicator grid rendering
	ind1 := TechnicalIndicator{
		Name:      "RSI",
		Value:     "30.2",
		Signal:    "BUY",
		Sentiment: StateBullish,
	}
	ind2 := TechnicalIndicator{
		Name:      "SMA",
		Value:     "152.5",
		Signal:    "SELL",
		Sentiment: StateBearish,
	}

	gridStr := RenderMetricsGrid(ind1, ind2, theme)
	if gridStr == "" {
		t.Errorf("expected grid string to be non-empty")
	}

	// Create CLIController (should automatically detect if run in TTY or redirected file mode)
	controller := NewCLIController()
	if controller == nil {
		t.Fatalf("failed to create CLIController")
	}

	// Test RunStep with a fast action
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	title := "Test Action"
	action := func() (string, CLIState, error) {
		return "Analysis Output", StateBullish, nil
	}

	result, err := controller.RunStep(ctx, title, action)
	if err != nil {
		t.Fatalf("RunStep failed: %v", err)
	}
	if result != "Analysis Output" {
		t.Errorf("expected 'Analysis Output', got %s", result)
	}
}
