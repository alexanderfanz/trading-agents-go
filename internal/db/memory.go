package db

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"tradingagents/internal/config"
)

const entrySeparator = "\n\n<!-- ENTRY_END -->\n\n"

type MemoryEntry struct {
	Date       string `json:"date"`
	Ticker     string `json:"ticker"`
	Rating     string `json:"rating"`
	Pending    bool   `json:"pending"`
	RawReturn  string `json:"raw_return,omitempty"`
	Alpha      string `json:"alpha_return,omitempty"`
	Holding    string `json:"holding,omitempty"`
	Decision   string `json:"decision"`
	Reflection string `json:"reflection,omitempty"`
}

type TradingMemoryLog struct {
	mu      sync.RWMutex
	logPath string
	maxCap  *int
}

func NewTradingMemoryLog(cfg *config.Config) *TradingMemoryLog {
	if cfg.MemoryLogPath != "" {
		dir := filepath.Dir(cfg.MemoryLogPath)
		_ = os.MkdirAll(dir, 0755)
	}

	return &TradingMemoryLog{
		logPath: cfg.MemoryLogPath,
		maxCap:  cfg.MemoryLogMaxEntries,
	}
}

// StoreDecision appends a pending entry to the memory log.
func (l *TradingMemoryLog) StoreDecision(ticker, tradeDate, finalTradeDecision string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logPath == "" {
		return nil
	}

	// Idempotency check
	if _, err := os.Stat(l.logPath); err == nil {
		data, err := os.ReadFile(l.logPath)
		if err == nil {
			content := string(data)
			pendingTag := fmt.Sprintf("[%s | %s |", tradeDate, ticker)
			for _, line := range strings.Split(content, "\n") {
				if strings.HasPrefix(line, pendingTag) && strings.HasSuffix(line, "| pending]") {
					return nil // Already stored
				}
			}
		}
	}

	rating := parseRating(finalTradeDecision)
	tag := fmt.Sprintf("[%s | %s | %s | pending]", tradeDate, ticker, rating)
	entry := fmt.Sprintf("%s\n\nDECISION:\n%s%s", tag, finalTradeDecision, entrySeparator)

	f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open memory log: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to memory log: %w", err)
	}

	return nil
}

// LoadEntries parses all entries from the markdown log.
func (l *TradingMemoryLog) LoadEntries() ([]MemoryEntry, error) {
	if l.logPath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(l.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	rawBlocks := strings.Split(string(data), entrySeparator)
	var entries []MemoryEntry

	decisionRe := regexp.MustCompile(`(?s)DECISION:\n(.*?)(?:\nREFLECTION:|\z)`)
	reflectionRe := regexp.MustCompile(`(?s)REFLECTION:\n(.*)$`)

	for _, block := range rawBlocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.SplitN(block, "\n", 2)
		if len(lines) < 2 {
			continue
		}

		tagLine := strings.TrimSpace(lines[0])
		if !strings.HasPrefix(tagLine, "[") || !strings.HasSuffix(tagLine, "]") {
			continue
		}

		fields := strings.Split(tagLine[1:len(tagLine)-1], "|")
		if len(fields) < 4 {
			continue
		}

		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		entry := MemoryEntry{
			Date:    fields[0],
			Ticker:  fields[1],
			Rating:  fields[2],
			Pending: fields[3] == "pending",
		}

		if !entry.Pending {
			entry.RawReturn = fields[3]
			if len(fields) > 4 {
				entry.Alpha = fields[4]
			}
			if len(fields) > 5 {
				entry.Holding = fields[5]
			}
		}

		body := lines[1]
		if dMatch := decisionRe.FindStringSubmatch(body); len(dMatch) > 1 {
			entry.Decision = strings.TrimSpace(dMatch[1])
		}
		if rMatch := reflectionRe.FindStringSubmatch(body); len(rMatch) > 1 {
			entry.Reflection = strings.TrimSpace(rMatch[1])
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// GetPendingEntries returns entries with outcome: pending.
func (l *TradingMemoryLog) GetPendingEntries() ([]MemoryEntry, error) {
	entries, err := l.LoadEntries()
	if err != nil {
		return nil, err
	}

	var pending []MemoryEntry
	for _, e := range entries {
		if e.Pending {
			pending = append(pending, e)
		}
	}
	return pending, nil
}

// GetPastContext returns formatted context for prompts (last nSame same-ticker resolved, nCross cross-ticker resolved).
func (l *TradingMemoryLog) GetPastContext(ticker string, nSame, nCross int) (string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entries, err := l.LoadEntries()
	if err != nil {
		return "", err
	}

	var resolved []MemoryEntry
	for _, e := range entries {
		if !e.Pending {
			resolved = append(resolved, e)
		}
	}

	if len(resolved) == 0 {
		return "", nil
	}

	var same []MemoryEntry
	var cross []MemoryEntry

	// Go backwards (most recent first)
	for i := len(resolved) - 1; i >= 0; i-- {
		e := resolved[i]
		if len(same) >= nSame && len(cross) >= nCross {
			break
		}
		if e.Ticker == ticker && len(same) < nSame {
			same = append(same, e)
		} else if e.Ticker != ticker && len(cross) < nCross {
			cross = append(cross, e)
		}
	}

	if len(same) == 0 && len(cross) == 0 {
		return "", nil
	}

	var parts []string
	if len(same) > 0 {
		parts = append(parts, fmt.Sprintf("Past analyses of %s (most recent first):", ticker))
		for _, e := range same {
			parts = append(parts, formatFullEntry(e))
		}
	}

	if len(cross) > 0 {
		parts = append(parts, "Recent cross-ticker lessons:")
		for _, e := range cross {
			parts = append(parts, formatReflectionOnly(e))
		}
	}

	return strings.Join(parts, "\n\n"), nil
}

type BatchUpdate struct {
	Ticker       string
	TradeDate    string
	RawReturn    float64
	AlphaReturn  float64
	HoldingDays  int
	Reflection   string
}

// BatchUpdateWithOutcomes atomic updates for multiple entries.
func (l *TradingMemoryLog) BatchUpdateWithOutcomes(updates []BatchUpdate) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logPath == "" || len(updates) == 0 {
		return nil
	}

	data, err := os.ReadFile(l.logPath)
	if err != nil {
		return err
	}

	blocks := strings.Split(string(data), entrySeparator)
	updateMap := make(map[string]BatchUpdate)
	for _, u := range updates {
		updateMap[u.TradeDate+"|"+u.Ticker] = u
	}

	updated := false
	var newBlocks []string

	for _, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			newBlocks = append(newBlocks, block)
			continue
		}

		lines := strings.Split(trimmed, "\n")
		tagLine := strings.TrimSpace(lines[0])

		if strings.HasPrefix(tagLine, "[") && strings.HasSuffix(tagLine, "| pending]") {
			fields := strings.Split(tagLine[1:len(tagLine)-1], "|")
			if len(fields) >= 4 {
				tDate := strings.TrimSpace(fields[0])
				ticker := strings.TrimSpace(fields[1])
				rating := strings.TrimSpace(fields[2])

				key := tDate + "|" + ticker
				if upd, ok := updateMap[key]; ok {
					rawPct := fmt.Sprintf("%+.1f%%", upd.RawReturn*100)
					alphaPct := fmt.Sprintf("%+.1f%%", upd.AlphaReturn*100)
					newTag := fmt.Sprintf("[%s | %s | %s | %s | %s | %dd]", tDate, ticker, rating, rawPct, alphaPct, upd.HoldingDays)

					rest := strings.Join(lines[1:], "\n")
					newBlock := fmt.Sprintf("%s\n\n%s\n\nREFLECTION:\n%s", newTag, strings.TrimLeft(rest, " \n\r"), upd.Reflection)
					newBlocks = append(newBlocks, newBlock)
					delete(updateMap, key)
					updated = true
					continue
				}
			}
		}

		newBlocks = append(newBlocks, block)
	}

	if !updated {
		return nil
	}

	// Apply rotation if needed
	newBlocks = l.applyRotation(newBlocks)

	newText := strings.Join(newBlocks, entrySeparator)
	tmpPath := l.logPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(newText), 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, l.logPath)
}

func (l *TradingMemoryLog) applyRotation(blocks []string) []string {
	if l.maxCap == nil || *l.maxCap <= 0 {
		return blocks
	}

	type taggedBlock struct {
		content    string
		isResolved bool
	}

	var list []taggedBlock
	resolvedCount := 0

	for _, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			list = append(list, taggedBlock{content: block, isResolved: false})
			continue
		}
		tagLine := strings.Split(trimmed, "\n")[0]
		isResolved := strings.HasPrefix(tagLine, "[") && strings.HasSuffix(tagLine, "]") && !strings.HasSuffix(tagLine, "| pending]")
		if isResolved {
			resolvedCount++
		}
		list = append(list, taggedBlock{content: block, isResolved: isResolved})
	}

	if resolvedCount <= *l.maxCap {
		return blocks
	}

	toDrop := resolvedCount - *l.maxCap
	var kept []string

	for _, tb := range list {
		if tb.isResolved && toDrop > 0 {
			toDrop--
			continue
		}
		kept = append(kept, tb.content)
	}

	return kept
}

func parseRating(decision string) string {
	lower := strings.ToLower(decision)
	if strings.Contains(lower, "rating**: buy") || strings.Contains(lower, "rating: buy") || strings.Contains(lower, "**buy**") {
		return "Buy"
	}
	if strings.Contains(lower, "rating**: overweight") || strings.Contains(lower, "rating: overweight") || strings.Contains(lower, "**overweight**") {
		return "Overweight"
	}
	if strings.Contains(lower, "rating**: sell") || strings.Contains(lower, "rating: sell") || strings.Contains(lower, "**sell**") {
		return "Sell"
	}
	if strings.Contains(lower, "rating**: underweight") || strings.Contains(lower, "rating: underweight") || strings.Contains(lower, "**underweight**") {
		return "Underweight"
	}
	return "Hold"
}

func formatFullEntry(e MemoryEntry) string {
	raw := e.RawReturn
	if raw == "" {
		raw = "n/a"
	}
	alpha := e.Alpha
	if alpha == "" {
		alpha = "n/a"
	}
	holding := e.Holding
	if holding == "" {
		holding = "n/a"
	}
	tag := fmt.Sprintf("[%s | %s | %s | %s | %s | %s]", e.Date, e.Ticker, e.Rating, raw, alpha, holding)
	parts := []string{tag, fmt.Sprintf("DECISION:\n%s", e.Decision)}
	if e.Reflection != "" {
		parts = append(parts, fmt.Sprintf("REFLECTION:\n%s", e.Reflection))
	}
	return strings.Join(parts, "\n\n")
}

func formatReflectionOnly(e MemoryEntry) string {
	raw := e.RawReturn
	if raw == "" {
		raw = "n/a"
	}
	tag := fmt.Sprintf("[%s | %s | %s | %s]", e.Date, e.Ticker, e.Rating, raw)
	if e.Reflection != "" {
		return fmt.Sprintf("%s\n%s", tag, e.Reflection)
	}
	text := e.Decision
	if len(text) > 300 {
		text = text[:300] + "..."
	}
	return fmt.Sprintf("%s\n%s", tag, text)
}

// CopyFile helper for backups
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
