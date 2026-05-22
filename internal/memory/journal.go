package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Separator is the delimiter between distinct journal entries.
const Separator = "\n\n<!-- ENTRY_END -->\n\n"

// JournalEntry represents a parsed entry from the decision log.
type JournalEntry struct {
	Date        string
	Ticker      string
	Rating      string
	Pending     bool
	RawReturn   string
	AlphaReturn string
	HoldingDays string
	Decision    string
	Reflection  string
}

// TradingMemoryLog coordinates append-only log storage and parser retrieval.
type TradingMemoryLog struct {
	logPath    string
	maxEntries int
	mu         sync.RWMutex
}

// NewTradingMemoryLog initializes the memory log manager.
func NewTradingMemoryLog(logPath string, maxEntries int) *TradingMemoryLog {
	return &TradingMemoryLog{
		logPath:    logPath,
		maxEntries: maxEntries,
	}
}

// StoreDecision appends a pending decision block to the end of the journal file.
func (l *TradingMemoryLog) StoreDecision(ticker, tradeDate, finalTradeDecision string) error {
	if l.logPath == "" {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 1. Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(l.logPath), 0755); err != nil {
		return err
	}

	// 2. Idempotency guard: scan for existing pending tag
	data, err := os.ReadFile(l.logPath)
	if err == nil {
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		lines := strings.Split(text, "\n")
		pendingPrefix := fmt.Sprintf("[%s | %s |", tradeDate, ticker)
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, pendingPrefix) && strings.HasSuffix(trimmed, "| pending]") {
				return nil // Already stored and pending
			}
		}
	}

	// 3. Append new pending entry
	rating := ParseRating(finalTradeDecision)
	tag := fmt.Sprintf("[%s | %s | %s | pending]", tradeDate, ticker, rating)
	entry := fmt.Sprintf("%s\n\nDECISION:\n%s%s", tag, finalTradeDecision, Separator)

	f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(entry)
	return err
}

// LoadEntries parses all entries from the log.
func (l *TradingMemoryLog) LoadEntries() ([]JournalEntry, error) {
	if l.logPath == "" {
		return nil, nil
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	data, err := os.ReadFile(l.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	text := string(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	blocks := strings.Split(text, Separator)

	var entries []JournalEntry
	for _, block := range blocks {
		parsed := parseEntry(block)
		if parsed != nil {
			entries = append(entries, *parsed)
		}
	}

	return entries, nil
}

// GetPendingEntries returns entries that are still pending.
func (l *TradingMemoryLog) GetPendingEntries() ([]JournalEntry, error) {
	entries, err := l.LoadEntries()
	if err != nil {
		return nil, err
	}

	var pending []JournalEntry
	for _, e := range entries {
		if e.Pending {
			pending = append(pending, e)
		}
	}
	return pending, nil
}

// GetPastContext retrieves formatting past context for prompt injection.
func (l *TradingMemoryLog) GetPastContext(ticker string, nSame, nCross int) (string, error) {
	entries, err := l.LoadEntries()
	if err != nil {
		return "", err
	}

	var resolved []JournalEntry
	for _, e := range entries {
		if !e.Pending {
			resolved = append(resolved, e)
		}
	}

	if len(resolved) == 0 {
		return "", nil
	}

	var same []JournalEntry
	var cross []JournalEntry

	// Traverse from newest to oldest (reversed)
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
			parts = append(parts, formatFull(e))
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

// OutcomeUpdate specifies the target parameters for updating an entry outcome.
type OutcomeUpdate struct {
	Ticker      string
	TradeDate   string
	RawReturn   float64
	AlphaReturn float64
	HoldingDays int
	Reflection  string
}

// BatchUpdateWithOutcomes updates pending tags and appends reflections.
func (l *TradingMemoryLog) BatchUpdateWithOutcomes(updates []OutcomeUpdate) error {
	if l.logPath == "" || len(updates) == 0 {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	text := string(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	blocks := strings.Split(text, Separator)

	type key struct {
		tradeDate string
		ticker    string
	}
	updateMap := make(map[key]OutcomeUpdate)
	for _, u := range updates {
		updateMap[key{tradeDate: u.TradeDate, ticker: u.Ticker}] = u
	}

	newBlocks := make([]string, 0, len(blocks))
	for _, block := range blocks {
		stripped := strings.TrimSpace(block)
		if stripped == "" {
			newBlocks = append(newBlocks, block)
			continue
		}

		lines := strings.Split(stripped, "\n")
		tagLine := strings.TrimSpace(lines[0])

		matched := false
		for k, upd := range updateMap {
			pendingPrefix := fmt.Sprintf("[%s | %s |", k.tradeDate, k.ticker)
			if strings.HasPrefix(tagLine, pendingPrefix) && strings.HasSuffix(tagLine, "| pending]") {
				fields := strings.Split(tagLine[1:len(tagLine)-1], "|")
				for i := range fields {
					fields[i] = strings.TrimSpace(fields[i])
				}
				rating := fields[2]
				rawPct := formatPercent(upd.RawReturn)
				alphaPct := formatPercent(upd.AlphaReturn)
				newTag := fmt.Sprintf("[%s | %s | %s | %s | %s | %dd]", k.tradeDate, k.ticker, rating, rawPct, alphaPct, upd.HoldingDays)

				rest := strings.Join(lines[1:], "\n")
				newBlocks = append(newBlocks, fmt.Sprintf("%s\n\n%s\n\nREFLECTION:\n%s", newTag, strings.TrimLeft(rest, "\n\t "), upd.Reflection))
				delete(updateMap, k)
				matched = true
				break
			}
		}

		if !matched {
			newBlocks = append(newBlocks, block)
		}
	}

	newBlocks = l.applyRotation(newBlocks)
	newText := strings.Join(newBlocks, Separator)

	// Atomic write using a temp file
	tmpPath := l.logPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(newText), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, l.logPath)
}

// applyRotation drops older resolved entries to enforce the maxResolved cap.
func (l *TradingMemoryLog) applyRotation(blocks []string) []string {
	if l.maxEntries <= 0 {
		return blocks
	}

	type blockTag struct {
		content    string
		isResolved bool
	}

	var decisions []blockTag
	resolvedCount := 0

	for _, block := range blocks {
		stripped := strings.TrimSpace(block)
		if stripped == "" {
			decisions = append(decisions, blockTag{content: block, isResolved: false})
			continue
		}
		lines := strings.Split(stripped, "\n")
		tagLine := strings.TrimSpace(lines[0])
		isResolved := strings.HasPrefix(tagLine, "[") &&
			strings.HasSuffix(tagLine, "]") &&
			!strings.HasSuffix(tagLine, "| pending]")

		if isResolved {
			resolvedCount++
		}
		decisions = append(decisions, blockTag{content: block, isResolved: isResolved})
	}

	if resolvedCount <= l.maxEntries {
		return blocks
	}

	toDrop := resolvedCount - l.maxEntries
	var kept []string
	for _, bt := range decisions {
		if bt.isResolved && toDrop > 0 {
			toDrop--
			continue
		}
		kept = append(kept, bt.content)
	}

	return kept
}

// --- Helper Functions ---

func parseEntryBody(body string) (decision string, reflection string) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	decIdx := strings.Index(body, "DECISION:\n")
	refIdx := strings.Index(body, "REFLECTION:\n")

	if decIdx != -1 {
		decStart := decIdx + len("DECISION:\n")
		if refIdx != -1 && refIdx > decStart {
			decision = strings.TrimSpace(body[decStart:refIdx])
		} else {
			decision = strings.TrimSpace(body[decStart:])
		}
	}

	if refIdx != -1 {
		reflection = strings.TrimSpace(body[refIdx+len("REFLECTION:\n"):])
	}

	return decision, reflection
}

func parseEntry(raw string) *JournalEntry {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) == 0 {
		return nil
	}
	tagLine := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(tagLine, "[") || !strings.HasSuffix(tagLine, "]") {
		return nil
	}
	fields := strings.Split(tagLine[1:len(tagLine)-1], "|")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	if len(fields) < 4 {
		return nil
	}

	entry := &JournalEntry{
		Date:    fields[0],
		Ticker:  fields[1],
		Rating:  fields[2],
		Pending: fields[3] == "pending",
	}

	if !entry.Pending {
		entry.RawReturn = fields[3]
		if len(fields) > 4 {
			entry.AlphaReturn = fields[4]
		}
		if len(fields) > 5 {
			entry.HoldingDays = fields[5]
		}
	}

	body := strings.Join(lines[1:], "\n")
	dec, ref := parseEntryBody(body)
	entry.Decision = dec
	entry.Reflection = ref

	return entry
}

func formatFull(e JournalEntry) string {
	raw := e.RawReturn
	if raw == "" {
		raw = "n/a"
	}
	alpha := e.AlphaReturn
	if alpha == "" {
		alpha = "n/a"
	}
	holding := e.HoldingDays
	if holding == "" {
		holding = "n/a"
	}
	tag := fmt.Sprintf("[%s | %s | %s | %s | %s | %s]", e.Date, e.Ticker, e.Rating, raw, alpha, holding)
	parts := []string{tag, "DECISION:\n" + e.Decision}
	if e.Reflection != "" {
		parts = append(parts, "REFLECTION:\n"+e.Reflection)
	}
	return strings.Join(parts, "\n\n")
}

func formatReflectionOnly(e JournalEntry) string {
	raw := e.RawReturn
	if raw == "" {
		raw = "n/a"
	}
	tag := fmt.Sprintf("[%s | %s | %s | %s]", e.Date, e.Ticker, e.Rating, raw)
	if e.Reflection != "" {
		return fmt.Sprintf("%s\n%s", tag, e.Reflection)
	}
	decText := e.Decision
	if len(decText) > 300 {
		decText = decText[:300] + "..."
	}
	return fmt.Sprintf("%s\n%s", tag, decText)
}

func formatPercent(val float64) string {
	sign := ""
	if val >= 0 {
		sign = "+"
	}
	return fmt.Sprintf("%s%.1f%%", sign, val*100)
}
