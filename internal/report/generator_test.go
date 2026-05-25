package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateLocalReports(t *testing.T) {
	// 1. Create a temporary test directory
	tempDir := t.TempDir()

	// 2. Synthesize mock ReportData
	data := &ReportData{
		Ticker:             "MSFT",
		TradeDate:          "2026-05-25",
		Timestamp:          time.Date(2026, 5, 25, 15, 41, 0, 0, time.UTC),
		MarketReport:       "Mock Market Report Content",
		SentimentReport:    "Mock Sentiment Report Content",
		NewsReport:         "Mock News Report Content",
		FundamentalsReport: "Mock Fundamentals Report Content",
		BullDebate:         "Mock Bull Round 1\n\n---\n\nMock Bull Round 2",
		BearDebate:         "Mock Bear Round 1\n\n---\n\nMock Bear Round 2",
		ResearchPlan:       "Mock Research Plan Content",
		TraderProposal:     "Mock Trader Proposal Content",
		AggressiveRisk:     "Mock Aggressive Risk Critique",
		ConservativeRisk:   "Mock Conservative Risk Critique",
		NeutralRisk:        "Mock Neutral Risk Critique",
		FinalDecision:      "Mock Final Decision Content",
	}

	// 3. Invoke report generation
	err := GenerateLocalReports(data, tempDir)
	if err != nil {
		t.Fatalf("GenerateLocalReports returned an error: %v", err)
	}

	// 4. Assert directories and files exist with correct content
	targetFolder := filepath.Join(tempDir, "MSFT_20260525_154100")

	expectedFiles := []struct {
		subDir   string
		fileName string
		content  string
	}{
		{"1_analysts", "market.md", "Mock Market Report Content"},
		{"1_analysts", "sentiment.md", "Mock Sentiment Report Content"},
		{"1_analysts", "news.md", "Mock News Report Content"},
		{"1_analysts", "fundamentals.md", "Mock Fundamentals Report Content"},
		{"2_research", "bull.md", "Mock Bull Round 1\n\n---\n\nMock Bull Round 2"},
		{"2_research", "bear.md", "Mock Bear Round 1\n\n---\n\nMock Bear Round 2"},
		{"2_research", "manager.md", "Mock Research Plan Content"},
		{"3_trading", "trader.md", "Mock Trader Proposal Content"},
		{"4_risk", "aggressive.md", "Mock Aggressive Risk Critique"},
		{"4_risk", "conservative.md", "Mock Conservative Risk Critique"},
		{"4_risk", "neutral.md", "Mock Neutral Risk Critique"},
		{"5_portfolio", "decision.md", "Mock Final Decision Content"},
	}

	for _, ef := range expectedFiles {
		filePath := filepath.Join(targetFolder, ef.subDir, ef.fileName)
		info, err := os.Stat(filePath)
		if err != nil {
			t.Errorf("expected file to exist: %s, err: %v", filePath, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("expected path to be a file, not a directory: %s", filePath)
		}

		// Verify content
		bytes, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("failed to read file %s: %v", filePath, err)
			continue
		}
		gotContent := string(bytes)
		if gotContent != ef.content {
			t.Errorf("file %s has incorrect content; expected %q, got %q", filePath, ef.content, gotContent)
		}
	}

	// 5. Assert complete_report.md exists and has the unified content structure
	completeReportPath := filepath.Join(targetFolder, "complete_report.md")
	bytes, err := os.ReadFile(completeReportPath)
	if err != nil {
		t.Fatalf("failed to read complete report: %v", err)
	}

	completeReportContent := string(bytes)
	requiredSubstrings := []string{
		"# Complete Trading Agent Report: MSFT",
		"- **Date of Trade Analysis**: 2026-05-25",
		"- **Execution Time**: 2026-05-25 15:41:00",
		"## Executive Summary & Portfolio Decision",
		"Mock Final Decision Content",
		"## 1. Concurrent Market Analysis",
		"Mock Market Report Content",
		"Mock Sentiment Report Content",
		"Mock News Report Content",
		"Mock Fundamentals Report Content",
		"## 2. Research Debate & Consensus",
		"Mock Bull Round 1",
		"Mock Bear Round 1",
		"Mock Research Plan Content",
		"## 3. Trading Proposal",
		"Mock Trader Proposal Content",
		"## 4. Risk Assessment & Sizing Debate",
		"Mock Aggressive Risk Critique",
		"Mock Conservative Risk Critique",
		"Mock Neutral Risk Critique",
	}

	for _, s := range requiredSubstrings {
		if !strings.Contains(completeReportContent, s) {
			t.Errorf("complete_report.md is missing expected substring %q", s)
		}
	}
}

func TestGenerateLocalReportsEmpty(t *testing.T) {
	tempDir := t.TempDir()

	data := &ReportData{
		Ticker:    "AAPL",
		TradeDate: "2026-05-25",
		Timestamp: time.Date(2026, 5, 25, 15, 41, 0, 0, time.UTC),
	}

	err := GenerateLocalReports(data, tempDir)
	if err != nil {
		t.Fatalf("GenerateLocalReports returned an error: %v", err)
	}

	// Verify fallback handling for empty fields
	marketPath := filepath.Join(tempDir, "AAPL_20260525_154100", "1_analysts", "market.md")
	bytes, err := os.ReadFile(marketPath)
	if err != nil {
		t.Fatalf("failed to read market.md: %v", err)
	}
	content := string(bytes)
	if !strings.Contains(content, "No content was recorded") {
		t.Errorf("expected fallback content for empty field, got %q", content)
	}
}
