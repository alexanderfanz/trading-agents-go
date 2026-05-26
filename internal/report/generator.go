package report

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ReportData groups all markdown payloads together for clean assembly
type ReportData struct {
	Ticker                 string
	TradeDate              string
	Timestamp              time.Time
	MarketReport           string
	SentimentReport        string
	NewsReport             string
	FundamentalsReport     string
	BullDebate             string
	BearDebate             string
	ResearchPlan           string
	TraderProposal         string
	AggressiveRisk         string
	ConservativeRisk       string
	NeutralRisk            string
	FinalDecision          string
}

// GenerateLocalReports executes folder creation and writes individual & consolidated markdown reports.
func GenerateLocalReports(data *ReportData, baseDir string) error {
	if data == nil {
		return fmt.Errorf("report data cannot be nil")
	}
	// Construct the root reports folder name: reports/<TICKER>_<YYYYMMDD_HHMMSS>
	timestampStr := data.Timestamp.Format("20060102_150405")
	folderName := fmt.Sprintf("%s_%s", data.Ticker, timestampStr)
	targetDir := filepath.Join(baseDir, folderName)

	// Define subdirectories to create
	dirs := []string{
		filepath.Join(targetDir, "1_analysts"),
		filepath.Join(targetDir, "2_research"),
		filepath.Join(targetDir, "3_trading"),
		filepath.Join(targetDir, "4_risk"),
		filepath.Join(targetDir, "5_portfolio"),
	}

	// Create directories
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Helper function to write file with fallback value for empty data
	writeFile := func(subDir, fileName, content string) error {
		filePath := filepath.Join(targetDir, subDir, fileName)
		if content == "" {
			content = "_No content was recorded for this section._"
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
		return nil
	}

	// Write individual Markdown files
	if err := writeFile("1_analysts", "market.md", data.MarketReport); err != nil {
		return err
	}
	if err := writeFile("1_analysts", "sentiment.md", data.SentimentReport); err != nil {
		return err
	}
	if err := writeFile("1_analysts", "news.md", data.NewsReport); err != nil {
		return err
	}
	if err := writeFile("1_analysts", "fundamentals.md", data.FundamentalsReport); err != nil {
		return err
	}

	if err := writeFile("2_research", "bull.md", data.BullDebate); err != nil {
		return err
	}
	if err := writeFile("2_research", "bear.md", data.BearDebate); err != nil {
		return err
	}
	if err := writeFile("2_research", "manager.md", data.ResearchPlan); err != nil {
		return err
	}

	if err := writeFile("3_trading", "trader.md", data.TraderProposal); err != nil {
		return err
	}

	if err := writeFile("4_risk", "aggressive.md", data.AggressiveRisk); err != nil {
		return err
	}
	if err := writeFile("4_risk", "conservative.md", data.ConservativeRisk); err != nil {
		return err
	}
	if err := writeFile("4_risk", "neutral.md", data.NeutralRisk); err != nil {
		return err
	}

	if err := writeFile("5_portfolio", "decision.md", data.FinalDecision); err != nil {
		return err
	}

	// Assemble consolidated report
	completeReportContent := assembleCompleteReport(data)
	completeReportPath := filepath.Join(targetDir, "complete_report.md")
	if err := os.WriteFile(completeReportPath, []byte(completeReportContent), 0644); err != nil {
		return fmt.Errorf("failed to write complete report: %w", err)
	}

	return nil
}

func assembleCompleteReport(data *ReportData) string {
	fallback := func(val string) string {
		if val == "" {
			return "_No content was recorded for this section._"
		}
		return val
	}

	return fmt.Sprintf(`# Complete Trading Agent Report: %s

- **Date of Trade Analysis**: %s
- **Execution Time**: %s

---

## Executive Summary & Portfolio Decision
%s

---

## 1. Concurrent Market Analysis

### Market Analyst Report
%s

### Sentiment Analyst Report
%s

### News Analyst Report
%s

### Fundamentals Analyst Report
%s

---

## 2. Research Debate & Consensus

### Bull Analyst Arguments (All Rounds)
%s

### Bear Analyst Arguments (All Rounds)
%s

### Research Manager Investment Plan
%s

---

## 3. Trading Proposal
%s

---

## 4. Risk Assessment & Sizing Debate

### Aggressive Risk Critique
%s

### Conservative Risk Critique
%s

### Neutral Risk Critique
%s

---

## 5. Final Portfolio Decision
%s
`,
		data.Ticker,
		data.TradeDate,
		data.Timestamp.Format("2006-01-02 15:04:05"),
		fallback(data.FinalDecision),
		fallback(data.MarketReport),
		fallback(data.SentimentReport),
		fallback(data.NewsReport),
		fallback(data.FundamentalsReport),
		fallback(data.BullDebate),
		fallback(data.BearDebate),
		fallback(data.ResearchPlan),
		fallback(data.TraderProposal),
		fallback(data.AggressiveRisk),
		fallback(data.ConservativeRisk),
		fallback(data.NeutralRisk),
		fallback(data.FinalDecision),
	)
}
