package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// CLIController manages our CLI state, adapting outputs for TTY or piped logs.
type CLIController struct {
	Theme      ObsidianTheme
	IsTTY      bool
	ColorStyle lipgloss.Renderer
}

// NewCLIController detects the terminal environment and sets up the rendering engine.
func NewCLIController() *CLIController {
	theme := NewObsidianTheme()

	// Detect if output is standard terminal or a redirected file/pipe
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	// Check for dumb terminal environments (e.g. basic emulators, custom pipelines)
	if os.Getenv("TERM") == "dumb" {
		isTTY = false
	}

	controller := &CLIController{
		Theme: theme,
		IsTTY: isTTY,
	}

	if !isTTY {
		// Set Lipgloss rendering to no-color ASCII profile for clean log files
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	return controller
}

// RunStep adapts loading spinners or static sequential logs dynamically.
func (c *CLIController) RunStep(ctx context.Context, title string, stepAction func() (string, CLIState, error)) (string, error) {
	if !c.IsTTY {
		// PIPED OR REDIRECTED MODE: Do not use spinners, ANSI escapes, or carriage returns.
		fmt.Printf("[INFO] Starting step: %s\n", title)

		result, state, err := stepAction()
		if err != nil {
			fmt.Printf("[ERROR] Step %s failed: %v\n", title, err)
			return "", err
		}

		// Print clean static card output. Lipgloss automatically strips colors because profile is Ascii
		fmt.Printf("[SUCCESS] Step %s completed.\n", title)
		card := RenderObsidianCard(title, result, state == StateBullish, c.Theme)
		fmt.Println(card)

		return result, nil
	}

	// TTY MODE: Use rich HSL colored borders, ANSI \r\033[K clear lines, and live progress spinners.
	doneChan := make(chan bool)
	var result string
	var state CLIState
	var stepErr error

	go func() {
		result, state, stepErr = stepAction()
		doneChan <- true
	}()

	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 0
	for {
		select {
		case <-doneChan:
			// 1. Wipe the dynamic active spinner loading line using carriage return and Erase-in-Line
			fmt.Print("\r\033[K")

			if stepErr == nil {
				// 2. Construct dynamic border styling depending on consensus state
				style := GetDynamicBorderStyle(state, c.Theme)
				cardHeader := lipgloss.NewStyle().Foreground(c.Theme.TextBright).Bold(true).Render(fmt.Sprintf("─── [ %s ] ───", title))
				cardBody := lipgloss.NewStyle().Foreground(c.Theme.TextBright).Render(result)

				// 3. Print static Obsidian Card cleanly
				fmt.Println(style.Render(fmt.Sprintf("%s\n\n%s", cardHeader, cardBody)))
			}
			return result, stepErr

		case <-ticker.C:
			// Active Ticking: Print animated frame with carriage return and clear-line sequence
			colorFrame := lipgloss.NewStyle().Foreground(c.Theme.IndigoAccent).Render(spinnerFrames[frameIdx%len(spinnerFrames)])
			fmt.Printf("\r\033[K%s Executing: %s...", colorFrame, title)
			frameIdx++

		case <-ctx.Done():
			// Clean terminal line on cancellation
			fmt.Print("\r\033[K")
			return "", ctx.Err()
		}
	}
}

// RenderObsidianCard wraps content strings inside a beautifully bordered card container.
func RenderObsidianCard(title, content string, isBullish bool, theme ObsidianTheme) string {
	accent := theme.BullAccent
	if !isBullish {
		accent = theme.BearAccent
	}

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderMuted).
		Padding(1, 2).
		MarginBottom(1).
		Width(80)

	titleStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)

	bodyStyle := lipgloss.NewStyle().
		Foreground(theme.TextBright)

	header := titleStyle.Render(fmt.Sprintf("─── [ %s ] ───", title))
	body := bodyStyle.Render(content)

	return cardStyle.Render(fmt.Sprintf("%s\n\n%s", header, body))
}
