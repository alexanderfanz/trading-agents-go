package dataflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type StockTwitsResponse struct {
	Messages []StockTwitsMessage `json:"messages"`
}

type StockTwitsMessage struct {
	Body      string             `json:"body"`
	CreatedAt string             `json:"created_at"`
	User      StockTwitsUser     `json:"user"`
	Entities  StockTwitsEntities `json:"entities"`
}

type StockTwitsUser struct {
	Username string `json:"username"`
}

type StockTwitsEntities struct {
	Sentiment *StockTwitsSentiment `json:"sentiment"`
}

type StockTwitsSentiment struct {
	Basic string `json:"basic"` // "Bullish", "Bearish", or empty
}

// FetchStockTwits fetches recent StockTwits messages for ticker and returns them as a formatted plaintext block.
func (p *HTTPNewsSocialProvider) FetchStockTwits(ctx context.Context, ticker string, limit int) (string, error) {
	url := fmt.Sprintf("https://api.stocktwits.com/api/2/streams/symbol/%s.json", strings.ToUpper(ticker))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Sprintf("<stocktwits unavailable: %v>", err), err
	}

	req.Header.Set("User-Agent", "tradingagents/0.2 (+https://github.com/TauricResearch/TradingAgents)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Sprintf("<stocktwits unavailable: %v>", err), err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("<stocktwits unavailable: status %d>", resp.StatusCode), fmt.Errorf("stocktwits responded with status %d", resp.StatusCode)
	}

	var payload StockTwitsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Sprintf("<stocktwits unavailable: JSON decode error %v>", err), err
	}

	messages := payload.Messages
	if len(messages) == 0 {
		return fmt.Sprintf("<no StockTwits messages found for $%s>", strings.ToUpper(ticker)), nil
	}

	if len(messages) > limit {
		messages = messages[:limit]
	}

	var bullish, bearish, unlabeled int
	var lines []string

	// Reusable strings builder to keep allocation low
	var sb strings.Builder
	// Est. allocation size to reduce re-allocations
	sb.Grow(len(messages) * 300)

	for _, m := range messages {
		// Clean body of newlines
		body := strings.ReplaceAll(m.Body, "\n", " ")
		body = strings.TrimSpace(body)
		
		// Truncate if too long
		runes := []rune(body)
		if len(runes) > 280 {
			body = string(runes[:280]) + "…"
		}

		tag := "no-label"
		if m.Entities.Sentiment != nil {
			switch m.Entities.Sentiment.Basic {
			case "Bullish":
				bullish++
				tag = "Bullish"
			case "Bearish":
				bearish++
				tag = "Bearish"
			default:
				unlabeled++
			}
		} else {
			unlabeled++
		}

		lines = append(lines, fmt.Sprintf("[%s · @%s · %s] %s", m.CreatedAt, m.User.Username, tag, body))
	}

	total := bullish + bearish + unlabeled
	var bullPct, bearPct int
	if total > 0 {
		bullPct = int(float64(bullish*100)/float64(total) + 0.5)
		bearPct = int(float64(bearish*100)/float64(total) + 0.5)
	}

	fmt.Fprintf(&sb, "Bullish: %d (%d%%) · Bearish: %d (%d%%) · Unlabeled: %d · Total: %d most-recent messages\n\n",
		bullish, bullPct, bearish, bearPct, unlabeled, total)
	sb.WriteString(strings.Join(lines, "\n"))

	return sb.String(), nil
}
