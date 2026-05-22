package orchestrator

import (
	"fmt"
	"sync"
	"time"
	"trading-agents-go/internal/checkpoint"
)

// Alias important types from checkpoint package for local orchestration convenience
type TradingState = checkpoint.TradingState
type PortfolioState = checkpoint.PortfolioState
type SignalEntry = checkpoint.SignalEntry
type InvestDebateState = checkpoint.InvestDebateState
type RiskDebateState = checkpoint.RiskDebateState

// SafeReportMap holds the result and profiling metrics of fanned-out concurrent analyst runners.
type SafeReportMap struct {
	mu        sync.RWMutex
	reports   map[string]string
	latencies map[string]time.Duration
}

// NewSafeReportMap initializes a thread-safe report container.
func NewSafeReportMap() *SafeReportMap {
	return &SafeReportMap{
		reports:   make(map[string]string),
		latencies: make(map[string]time.Duration),
	}
}

// Store records an analyst's report and latency profile in a thread-safe manner.
func (m *SafeReportMap) Store(analyst string, report string, dur time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports[analyst] = report
	m.latencies[analyst] = dur
}

// GetReports returns a copy of gathered analyst reports.
func (m *SafeReportMap) GetReports() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := make(map[string]string, len(m.reports))
	for k, v := range m.reports {
		copied[k] = v
	}
	return copied
}

// GetLatencies returns a copy of gathered analyst latencies.
func (m *SafeReportMap) GetLatencies() map[string]time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := make(map[string]time.Duration, len(m.latencies))
	for k, v := range m.latencies {
		copied[k] = v
	}
	return copied
}

// AddDebateMessage appends a speaker's message to the active investment debate logs.
func AddDebateMessage(state *TradingState, speaker, message string) {
	state.Lock()
	defer state.Unlock()
	
	if state.InvestmentDebate.History != "" {
		state.InvestmentDebate.History += "\n"
	}
	state.InvestmentDebate.History += fmt.Sprintf("%s: %s", speaker, message)
	state.InvestmentDebate.Count++
}
