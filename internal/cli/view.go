package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TechnicalIndicator holds metrics for a single market indicator.
type TechnicalIndicator struct {
	Name      string
	Value     string
	Signal    string
	Sentiment CLIState // Bullish, Bearish, Neutral
}

// RenderMetricsGrid creates a side-by-side grid comparing two technical indicators.
func RenderMetricsGrid(col1, col2 TechnicalIndicator, theme ObsidianTheme) string {
	// 1. Column Width Definitions
	colWidth := 37 // Fits perfectly inside an 80-width container with border pads

	// 2. Base cell styles using our HSL Theme Tokens
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.TextBright).
		Background(theme.BorderElevated).
		Padding(0, 1).
		Width(colWidth - 2). // Accounting for borders
		Align(lipgloss.Center)

	metricLabelStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted)

	// 3. Dynamic helper for signal badges
	getSignalBadge := func(ind TechnicalIndicator) string {
		style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
		switch ind.Sentiment {
		case StateBullish:
			return style.Foreground(theme.Background).Background(theme.BullAccent).Render(" BUY ")
		case StateBearish:
			return style.Foreground(theme.Background).Background(theme.BearAccent).Render(" SELL ")
		default:
			return style.Foreground(theme.TextBright).Background(theme.BorderMuted).Render(" HOLD ")
		}
	}

	// 4. Construct Column 1
	col1Title := titleStyle.Render(strings.ToUpper(col1.Name))
	col1Body := fmt.Sprintf("\n%s %s\n\n%s %s",
		metricLabelStyle.Render("Current Value:"), lipgloss.NewStyle().Foreground(theme.TextBright).Render(col1.Value),
		metricLabelStyle.Render("Market Signal:"), getSignalBadge(col1),
	)

	col1Container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderMuted).
		Padding(1, 1).
		Width(colWidth).
		Height(7).
		Render(lipgloss.JoinVertical(lipgloss.Left, col1Title, col1Body))

	// 5. Construct Column 2
	col2Title := titleStyle.Render(strings.ToUpper(col2.Name))
	col2Body := fmt.Sprintf("\n%s %s\n\n%s %s",
		metricLabelStyle.Render("Current Value:"), lipgloss.NewStyle().Foreground(theme.TextBright).Render(col2.Value),
		metricLabelStyle.Render("Market Signal:"), getSignalBadge(col2),
	)

	col2Container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderMuted).
		Padding(1, 1).
		Width(colWidth).
		Height(7).
		Render(lipgloss.JoinVertical(lipgloss.Left, col2Title, col2Body))

	// 6. Join columns side-by-side using JoinHorizontal
	// We use lipgloss.Top alignment to align column tops perfectly
	joinedGrid := lipgloss.JoinHorizontal(lipgloss.Top, col1Container, col2Container)

	// 7. Outer container styling
	outerContainer := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.IndigoAccent).
		Padding(1, 2).
		Width(80).
		Render(fmt.Sprintf("%s\n%s",
			lipgloss.NewStyle().Foreground(theme.IndigoAccent).Bold(true).Render("📊 TECHNICAL INDICATOR ANALYSIS GRID"),
			joinedGrid,
		))

	return outerContainer
}
