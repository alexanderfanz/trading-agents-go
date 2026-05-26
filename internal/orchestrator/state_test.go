package orchestrator

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"trading-agents-go/internal/checkpoint"
)

func TestSafeReportMap_ConcurrentStoreAndRead(t *testing.T) {
	m := NewSafeReportMap()
	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("analyst-%d", i)
			m.Store(name, "report-"+name, time.Duration(i)*time.Millisecond)
		}()
	}
	wg.Wait()

	reports := m.GetReports()
	latencies := m.GetLatencies()
	if len(reports) != goroutines {
		t.Fatalf("expected %d reports, got %d", goroutines, len(reports))
	}
	if len(latencies) != goroutines {
		t.Fatalf("expected %d latencies, got %d", goroutines, len(latencies))
	}

	// Copies must be independent of internal map.
	reports["mutated"] = "should not leak"
	if _, ok := m.GetReports()["mutated"]; ok {
		t.Fatal("GetReports should return a defensive copy")
	}
}

func TestAddDebateMessage(t *testing.T) {
	state := &checkpoint.TradingState{}
	AddDebateMessage(state, "Bull Analyst", "first point")
	AddDebateMessage(state, "Bear Analyst", "counter point")

	state.RLock()
	defer state.RUnlock()

	want := "Bull Analyst: first point\nBear Analyst: counter point"
	if state.InvestmentDebate.History != want {
		t.Fatalf("history = %q, want %q", state.InvestmentDebate.History, want)
	}
	if state.InvestmentDebate.Count != 2 {
		t.Fatalf("count = %d, want 2", state.InvestmentDebate.Count)
	}
}
