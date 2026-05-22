package dataflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type YahooSearchResponse struct {
	News []YahooNewsItem `json:"news"`
}

type YahooNewsItem struct {
	UUID                string `json:"uuid"`
	Title               string `json:"title"`
	Publisher           string `json:"publisher"`
	Link                string `json:"link"`
	ProviderPublishTime int64  `json:"providerPublishTime"`
	Summary             string `json:"summary"` // Might be present in some response schemas
}

var defaultGlobalNewsQueries = []string{
	"Federal Reserve interest rates inflation",
	"S&P 500 earnings GDP economic outlook",
	"geopolitical risk trade war sanctions",
	"ECB Bank of England BOJ central bank policy",
	"oil commodities supply chain energy",
}

// FetchNews retrieves company-specific news articles between start and end dates.
func (p *HTTPNewsSocialProvider) FetchNews(ctx context.Context, ticker string, start, end time.Time) (string, error) {
	limit := 20 // Default news_article_limit
	
	apiURL := fmt.Sprintf("https://query2.finance.yahoo.com/v1/finance/search?q=%s&newsCount=%d", url.QueryEscape(ticker), limit)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Sprintf("<news unavailable: %v>", err), err
	}

	req.Header.Set("User-Agent", "tradingagents/0.2 (+https://github.com/TauricResearch/TradingAgents)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Sprintf("<news unavailable: %v>", err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("<news unavailable: status %d>", resp.StatusCode), fmt.Errorf("news responded with status %d", resp.StatusCode)
	}

	var payload YahooSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Sprintf("<news unavailable: JSON decode error %v>", err), err
	}

	if len(payload.News) == 0 {
		return fmt.Sprintf("No news found for %s", strings.ToUpper(ticker)), nil
	}

	startUTC := start.UTC()
	endUTC := end.UTC()
	endLimit := endUTC.AddDate(0, 0, 1) // +1 day buffer

	var sb strings.Builder
	sb.Grow(len(payload.News) * 350)

	filteredCount := 0
	for _, item := range payload.News {
		pubTime := time.Unix(item.ProviderPublishTime, 0).UTC()
		
		// Date filter logic matching yfinance_news.py
		if pubTime.Before(startUTC) || pubTime.After(endLimit) {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s (source: %s)\n", item.Title, item.Publisher))
		if item.Summary != "" {
			sb.WriteString(fmt.Sprintf("%s\n", item.Summary))
		}
		if item.Link != "" {
			sb.WriteString(fmt.Sprintf("Link: %s\n", item.Link))
		}
		sb.WriteString("\n")
		filteredCount++
	}

	if filteredCount == 0 {
		return fmt.Sprintf("No news found for %s between %s and %s",
			strings.ToUpper(ticker), start.Format("2006-01-02"), end.Format("2006-01-02")), nil
	}

	header := fmt.Sprintf("## %s News, from %s to %s:\n\n",
		strings.ToUpper(ticker), start.Format("2006-01-02"), end.Format("2006-01-02"))
	
	return header + sb.String(), nil
}

// FetchGlobalNews retrieves global macro economic news up to currDate.
func (p *HTTPNewsSocialProvider) FetchGlobalNews(ctx context.Context, currDate time.Time, lookBackDays int, limit int) (string, error) {
	if lookBackDays <= 0 {
		lookBackDays = 7
	}
	if limit <= 0 {
		limit = 10
	}

	var allNews []YahooNewsItem
	seenTitles := make(map[string]bool)

	for _, query := range defaultGlobalNewsQueries {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		apiURL := fmt.Sprintf("https://query2.finance.yahoo.com/v1/finance/search?q=%s&newsCount=%d", url.QueryEscape(query), limit)
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", "tradingagents/0.2 (+https://github.com/TauricResearch/TradingAgents)")
		req.Header.Set("Accept", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			continue
		}

		var payload YahooSearchResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()

		if decodeErr != nil {
			continue
		}

		for _, item := range payload.News {
			cleanTitle := strings.TrimSpace(item.Title)
			if cleanTitle != "" && !seenTitles[cleanTitle] {
				seenTitles[cleanTitle] = true
				allNews = append(allNews, item)
			}
		}

		if len(allNews) >= limit {
			break
		}
	}

	if len(allNews) == 0 {
		return fmt.Sprintf("No global news found for %s", currDate.Format("2006-01-02")), nil
	}

	if len(allNews) > limit {
		allNews = allNews[:limit]
	}

	// Filter and format
	currUTC := currDate.UTC()
	startUTC := currUTC.AddDate(0, 0, -lookBackDays)
	endLimit := currUTC.AddDate(0, 0, 1)

	var sb strings.Builder
	sb.Grow(len(allNews) * 350)

	for _, item := range allNews {
		pubTime := time.Unix(item.ProviderPublishTime, 0).UTC()
		
		// Prevent look-ahead bias
		if pubTime.After(endLimit) {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s (source: %s)\n", item.Title, item.Publisher))
		if item.Summary != "" {
			sb.WriteString(fmt.Sprintf("%s\n", item.Summary))
		}
		if item.Link != "" {
			sb.WriteString(fmt.Sprintf("Link: %s\n", item.Link))
		}
		sb.WriteString("\n")
	}

	header := fmt.Sprintf("## Global Market News, from %s to %s:\n\n",
		startUTC.Format("2006-01-02"), currDate.Format("2006-01-02"))

	return header + sb.String(), nil
}
