# Component 5: Premium Styled Linear CLI (Lipgloss Design)

## 1. Technical Architecture & Data Flows

The original Python implementation relies on full-screen layout engines (like `Rich`'s fullscreen live view or `Textual`) that hijack the terminal buffer. While visually interactive, this approach makes running tests in headless pipelines difficult, prevents normal terminal output scrollback auditing, and blocks standard text search operations (`grep`) on execution logs.

The Go architecture implements a premium, HSL-harmonized **Linear Text CLI** utilizing Charmbracelet’s **`lipgloss`** styling and layout engine. It renders elegant, glassy obsidian cards and status metrics tables directly to standard out sequentially. This layout keeps your terminal scroll buffer active, allows piping metrics via standard shell commands, and operates natively under headless CI environments.

```
       [Orchestrator Executing Component Step]
                         │
                         ▼
        [Non-Blocking Live Terminal Spinner]
        (Displays active step e.g. "Running Market Analyst...")
                         │
                         ▼ (On Step Complete)
        [Wipe Active Loading Line using ANSI Escapes]
                         │
                         ▼
        [Construct & Render Premium Obsidian Card]
        (Padded container with rounded Slate borders and HSL highlights)
                         │
                         ▼ (Print to stdout)
        [Chronological Scrollback History Preserved]
```

### Visual Styling Design Decisions
1. **HSL-Harmonized Color System**: The palette utilizes customized deep tokens representing an obsidian glass theme. Colors are carefully crafted using Hue, Saturation, and Lightness coordinates to ensure visual harmony and optimal contrast.
2. **Linear Grid & Cards Layout**: Each major decision-making block (analyst reports, consensus synthesis, pm recommendations) is presented as a rounded, beautiful lipgloss card. This delivers high visual appeal while preserving normal terminal scrollback capacity.
3. **Non-Blocking Step Progress**: Live status loading spinners display during active LLM network requests. When a step finishes, the loading line is immediately replaced with the finalized obsidian styled card using standard ANSI escape sequences.
4. **Log Searchability**: By dumping raw text with standard ANSI color boundaries to standard out, terminal piping operations (`| grep "FINAL TRANSACTION PROPOSAL"`) and native scroll audits function seamlessly.

---

## 2. Obsidian Theme HSL Color Tokens & Dynamic Styling

To achieve a cohesive, modern visual aesthetic reminiscent of Obsidian glass, we define exact HSL channels and translate them to high-fidelity Hex tokens for Charmbracelet’s `lipgloss` rendering engine. 

### 2.1 Color Palette Specifications

| Token Name | Hex Code | HSL Color Coordinates | Visual Purpose |
| :--- | :--- | :--- | :--- |
| **Obsidian Dark BG** | `#0B0B0C` | `HSL(240°, 7%, 5%)` | Card container background and terminal background fill |
| **Muted Slate Border**| `#282830` | `HSL(240°, 9%, 17%)` | Standard, non-active container boundaries |
| **Elevated Slate Border**| `#40404C`| `HSL(240°, 9%, 27%)` | Hover states, active context boundaries, or neutral status |
| **Radiant Mint Bull** | `#10B981` | `HSL(161°, 84°, 39%)` | Bullish indicators, positive consensus, or active success states |
| **Soft Crimson Bear** | `#EF4444` | `HSL(0°, 84°, 60%)` | Bearish indicators, risk warnings, or failed execution states |
| **Amber Warning** | `#F59E0B` | `HSL(38°, 92°, 50%)` | Neutral/caution metrics, system warnings, or pending transitions |
| **Manager Indigo** | `#6366F1` | `HSL(239°, 84°, 66%)` | Manager checkpoints, system coordination, or primary headers |
| **Text Bright** | `#F3F4F6` | `HSL(220°, 14%, 96%)` | High-contrast primary label and paragraph text |
| **Text Muted** | `#8E8E9F` | `HSL(240°, 9%, 59%)` | Low-contrast secondary metadata and timestamp labels |

---

### 2.2 Dynamic State-Dependent Borders

A premium interface feels responsive and alive. Instead of using static borders, the Go CLI constructs dynamic border styles that shift based on runtime states (e.g., bullish trends, risk escalation, or system actions).

The following Go code block demonstrates how to declare the theme tokens and construct a dynamic style generator that alters the borders of card components in real-time depending on the agent's current state:

```go
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

// TradingState represents the current execution context of a trading agent.
type TradingState int

const (
	StateNeutral TradingState = iota
	StateBullish
	StateBearish
	StateRiskEscalation
	StateSystemAction
)

// GetDynamicBorderStyle returns a tailored Lipgloss Style with customized
// borders, colors, and margins matching the active execution state.
func GetDynamicBorderStyle(state TradingState, theme ObsidianTheme) lipgloss.Style {
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
```

---

## 3. Complex Table Grid Formatting

Displaying comparative technical indicators side-by-side requires combining horizontal and vertical layouts cleanly. In Charmbracelet's `lipgloss` system, this is achieved by constructing standalone vertical stacks for each column and then stitching them together using `lipgloss.JoinHorizontal`.

Below is a complete, production-grade Go function `RenderMetricsGrid` designed to display comparative technical indicators side-by-side in HSL-styled grids:

```go
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
	Sentiment TradingState // Bullish, Bearish, Neutral
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
```

---

## 4. ANSI Carriage Return Spinner Mechanics

To execute background orchestration steps without cluttering the output scroll buffer, we must manipulate terminal printing using low-level ANSI console mechanics.

### 4.1 How carriage returns (`\r`) and `\033[K` clear-line operations work

* **Carriage Return (`\r`)**: This moves the active terminal cursor back to column 0 of the *current line* without moving down to a new row.
* **Erase In Line (`\033[K` or `CSI K`)**: This deletes all text from the current cursor position to the end of the line. Without this, if a new status update is shorter than the previous status update, the terminal will leave trailing "ghost" characters from the old message (e.g., `Executing step: Analyst...ation` instead of `Executing step: Analyst...`).
  * `\033[K` instructs the terminal to erase the tail of the line, guaranteeing a clean slate for the next frame.

### 4.2 System-level Execution Lifecycles

The dynamic step animation lifecycle coordinates background task completion and foreground console painting safely.

```
       Main Thread                     Spinner Goroutine
            │                                  │
            │─── Launch Task Goroutine ───────>│
            │                                  │ [Start Loop]
            │                                  │ ───> Print "\r\033[K" (Wipe Line)
            │                                  │ ───> Print Active Frame & Msg
            │                                  │ ───> Sleep 80ms
            │                                  │ <─── [Repeat Loop]
            │                                  │
            │ <── Task Signals Completion ─────│
            │                                  │ [Stop Loop]
            │─── Wipe Terminal Line ──────────>│ ───> Print "\r\033[K" (Wipe Line)
            │                                  │
            │─── Render Obsidian Card ────────>│ (Prints static Lipgloss card to stdout)
            ▼                                  ▼
```

1. **Active Ticking**: The main thread spawns a step in a background goroutine while executing a terminal ticker. On every tick (e.g., 80ms), the terminal writes `\r\033[K` followed by the new spinner frame and the execution message.
2. **Buffer Safety**: Because no newline (`\n`) is written during active ticking, the scrollback buffer is not populated with garbage intermediate loading frames. The scrollbar remains completely clean and stable.
3. **Pristine Wipe & Replace**: Once the background step signals completion, the loop terminates. The main thread immediately issues a final `\r\033[K` sequence to completely erase the loading line, then prints the fully constructed, static Lipgloss card container to standard output, appended with `\n` to lock it in historical scrollback.

---

## 5. Piping & Redirection Automatic Detection

When running under headless automated test environments, CI/CD systems (like GitHub Actions), or when output is redirected to files (e.g., `tradingagents > trade.log`), displaying terminal escape sequences (`\033[K`), carriage returns (`\r`), and graphical borders creates corrupted files filled with raw ANSI byte streams.

### 5.1 Terminal Detection Mechanism

The Go Linear CLI automatically inspects standard out using `golang.org/x/term` and `github.com/muesli/termenv` to dynamically downscale its visual profile.

* **Checking if stdout is an active terminal**: We verify if the file descriptor is a TTY via `term.IsTerminal(int(os.Stdout.Fd()))`.
* **Checking for "dumb" terminal variables**: If the target system environment sets `TERM=dumb`, we disable graphical features.
* **Auto-Fallback Behavior**:
  1. **ANSI Escape Codes**: Completely disabled.
  2. **Lipgloss Style Colors**: Set to `termenv.Ascii` or `termenv.NoColor` so that `lipgloss` methods automatically output clean raw string characters without ANSI styling tags.
  3. **Carriage Return Spinners**: The spinner ticker is bypassed completely. The CLI writes static sequential log lines (e.g., `[INFO] Running Market Analyst...` followed by `[SUCCESS] Market Analyst finished.`) rather than performing carriage returns.

---

### 5.2 piping.go Implementation

The following complete Go implementation handles automatic detection and fallback styling:

```go
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
func (c *CLIController) RunStep(ctx context.Context, title string, stepAction func() (string, TradingState, error)) (string, error) {
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
	var state TradingState
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
```

---

## 6. Step-by-Step Implementation Sub-plan

- [x] **1. Scaffolding Style Assets**: Configure HSL color palettes and style variables in `internal/cli/theme.go`.
- [x] **2. Card Renderers**:
  - Implement the `RenderObsidianCard` bordered layout function with dynamic state changes.
  - Implement styled grid metrics tables for technical indicators displaying bullish green and bearish red highlights side-by-side using `JoinHorizontal`.
- [x] **3. Terminal Progress Spinner & Redirection**:
  - Implement `CLIController.RunStep` with piping check `term.IsTerminal(int(os.Stdout.Fd()))` and environment check `TERM=dumb`.
  - Use channels, ticker, and carriage returns `\r\033[K` for smooth non-blocking execution in active TTYs.
- [ ] **4. Orchestrator Binding**:
  - Wire step execution hooks in `cmd/tradingagents/main.go` to display live cards as the multi-agent orchestration proceeds.
- [ ] **5. UX Visual Audits**:
  - Conduct visual testing inside standard shell environments to verify that card layouts align beautifully on 80-character constraints, and verify logs are perfectly stripped of escape codes when running redirection: `go run cmd/tradingagents/main.go > test.log`.

---

## 7. Idiomatic Trade-offs

### Styled Linear CLI vs. Full-Screen Interactive hijacking (Bubble Tea)
* **Interactive Hijacking**: Standard bubbletea models hijack the terminal buffer, which works great for full-screen dashboards but breaks shell history, scrollback auditing, and CLI piping operations (`| grep`).
* **Linear Lipgloss Style**: Prints elegant, bounded cards directly to stdout chronologically. This keeps your terminal scroll buffer intact, allows direct redirection to log files, runs beautifully in headless CI environments, and preserves searchability.

### Automated Low-Level Color Downscaling
* **Static Styling**: If standard color codes are forced onto pipes, log files become filled with unreadable, messy ANSI sequences (e.g., `^[40m^[38;5;...`).
* **Dynamic Term Detection**: Programmatic checks via `golang.org/x/term` ensure that when output is redirected, colors are automatically downgraded to ASCII, making logs perfectly clean, structured, and searchable.

