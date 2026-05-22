package cli

import (
	"github.com/charmbracelet/lipgloss"
)

// ObsidianTheme defines the visual token palette for our glassy CLI interface.
type ObsidianTheme struct {
	Background     lipgloss.Color
	BorderMuted    lipgloss.Color
	BorderElevated lipgloss.Color
	BullAccent     lipgloss.Color
	BearAccent     lipgloss.Color
	WarningAccent  lipgloss.Color
	IndigoAccent   lipgloss.Color
	TextBright     lipgloss.Color
	TextMuted      lipgloss.Color
}

// NewObsidianTheme initializes the standard HSL-harmonized hex values.
func NewObsidianTheme() ObsidianTheme {
	return ObsidianTheme{
		Background:     lipgloss.Color("#0B0B0C"), // HSL(240, 7, 5)
		BorderMuted:    lipgloss.Color("#282830"), // HSL(240, 9, 17)
		BorderElevated: lipgloss.Color("#40404C"), // HSL(240, 9, 27)
		BullAccent:     lipgloss.Color("#10B981"), // HSL(161, 84, 39)
		BearAccent:     lipgloss.Color("#EF4444"), // HSL(0, 84, 60)
		WarningAccent:  lipgloss.Color("#F59E0B"), // HSL(38, 92, 50)
		IndigoAccent:   lipgloss.Color("#6366F1"), // HSL(239, 84, 66)
		TextBright:     lipgloss.Color("#F3F4F6"), // HSL(220, 14, 96)
		TextMuted:      lipgloss.Color("#8E8E9F"), // HSL(240, 9, 59)
	}
}

// CLIState represents the current execution context of a trading agent.
type CLIState int

const (
	StateNeutral CLIState = iota
	StateBullish
	StateBearish
	StateRiskEscalation
	StateSystemAction
)

// GetDynamicBorderStyle returns a tailored Lipgloss Style with customized
// borders, colors, and margins matching the active execution state.
func GetDynamicBorderStyle(state CLIState, theme ObsidianTheme) lipgloss.Style {
	baseStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		MarginBottom(1).
		Width(80).
		Background(theme.Background)

	switch state {
	case StateBullish:
		// Glowing Mint Green borders representing positive sentiment
		return baseStyle.BorderForeground(theme.BullAccent)
	case StateBearish:
		// Radiant Crimson borders representing negative sentiment
		return baseStyle.BorderForeground(theme.BearAccent)
	case StateRiskEscalation:
		// Amber Warning borders indicating risk management actions
		return baseStyle.BorderForeground(theme.WarningAccent)
	case StateSystemAction:
		// Indigo borders representing background orchestrator steps
		return baseStyle.BorderForeground(theme.IndigoAccent)
	case StateNeutral:
		fallthrough
	default:
		// Standard Obsidian glass borders
		return baseStyle.BorderForeground(theme.BorderMuted)
	}
}
