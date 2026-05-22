package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	socialUA = "tradingagents/0.2 (+https://github.com/TauricResearch/TradingAgents)"
)

var DefaultSubreddits = []string{"wallstreetbets", "stocks", "investing"}

type SocialClient struct {
	client *http.Client
}

func NewSocialClient() *SocialClient {
	return &SocialClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchRedditPosts searches the specified subreddits for a ticker and formats the results.
func (c *SocialClient) FetchRedditPosts(ctx context.Context, ticker string, subreddits []string, limitPerSub int) (string, error) {
	if len(subreddits) == 0 {
		subreddits = DefaultSubreddits
	}

	var blocks []string
	totalPosts := 0

	for i, sub := range subreddits {
		if i > 0 {
			// Enforce inter-request delay of 400ms to stay below Reddit's rate limit
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(400 * time.Millisecond):
			}
		}

		posts, err := c.fetchSubreddit(ctx, ticker, sub, limitPerSub)
		if err != nil {
			// Degrade gracefully, log warning or represent in output blocks
			blocks = append(blocks, fmt.Sprintf("r/%s: <error fetching: %v>", sub, err))
			continue
		}

		totalPosts += len(posts)
		if len(posts) == 0 {
			blocks = append(blocks, fmt.Sprintf("r/%s: <no posts found mentioning %s in the past 7 days>", sub, strings.ToUpper(ticker)))
			continue
		}

		lines := []string{fmt.Sprintf("r/%s — %d recent posts mentioning %s:", sub, len(posts), strings.ToUpper(ticker))}
		for _, p := range posts {
			title := strings.ReplaceAll(p.Title, "\n", " ")
			title = strings.TrimSpace(title)

			createdStr := "?"
			if p.CreatedUTC > 0 {
				createdStr = time.Unix(int64(p.CreatedUTC), 0).UTC().Format("2006-01-02")
			}

			selftext := strings.ReplaceAll(p.Selftext, "\n", " ")
			selftext = strings.TrimSpace(selftext)
			if len(selftext) > 240 {
				selftext = selftext[:240] + "…"
			}

			line := fmt.Sprintf("  [%s · %4d↑ · %3dc] %s", createdStr, p.Score, p.NumComments, title)
			if selftext != "" {
				line += fmt.Sprintf("\n    body excerpt: %s", selftext)
			}
			lines = append(lines, line)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	if totalPosts == 0 {
		var subRefs []string
		for _, s := range subreddits {
			subRefs = append(subRefs, "r/"+s)
		}
		return fmt.Sprintf("<no Reddit posts found mentioning %s across %s in the past 7 days>",
			strings.ToUpper(ticker), strings.Join(subRefs, ", ")), nil
	}

	return strings.Join(blocks, "\n\n"), nil
}

type redditPostData struct {
	Title       string  `json:"title"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
	Selftext    string  `json:"selftext"`
}

type redditChild struct {
	Kind string         `json:"kind"`
	Data redditPostData `json:"data"`
}

type redditResponse struct {
	Data struct {
		Children []redditChild `json:"children"`
	} `json:"data"`
}

func (c *SocialClient) fetchSubreddit(ctx context.Context, ticker, sub string, limit int) ([]redditPostData, error) {
	qs := url.Values{}
	qs.Set("q", ticker)
	qs.Set("restrict_sr", "on")
	qs.Set("sort", "new")
	qs.Set("t", "week")
	qs.Set("limit", fmt.Sprintf("%d", limit))

	u := fmt.Sprintf("https://www.reddit.com/r/%s/search.json?%s", sub, qs.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", socialUA)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reddit API returned status %d: %s", resp.StatusCode, string(body))
	}

	var payload redditResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var posts []redditPostData
	for _, child := range payload.Data.Children {
		posts = append(posts, child.Data)
	}
	return posts, nil
}

// FetchStockTwitsMessages fetches and parses recent StockTwits messages for the ticker.
func (c *SocialClient) FetchStockTwitsMessages(ctx context.Context, ticker string, limit int) (string, error) {
	u := fmt.Sprintf("https://api.stocktwits.com/api/2/streams/symbol/%s.json", url.QueryEscape(strings.ToUpper(ticker)))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", socialUA)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Sprintf("<stocktwits unavailable: %v>", err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("<stocktwits unavailable: status %d - %s>", resp.StatusCode, string(body)), nil
	}

	var payload struct {
		Messages []struct {
			CreatedAt string `json:"created_at"`
			Body      string `json:"body"`
			User      struct {
				Username string `json:"username"`
			} `json:"user"`
			Entities struct {
				Sentiment struct {
					Basic string `json:"basic"`
				} `json:"sentiment"`
			} `json:"entities"`
		} `json:"messages"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Sprintf("<stocktwits unavailable: decode error %v>", err), nil
	}

	if len(payload.Messages) == 0 {
		return fmt.Sprintf("<no StockTwits messages found for $%s>", strings.ToUpper(ticker)), nil
	}

	var lines []string
	var bullish, bearish, unlabeled int

	for i, m := range payload.Messages {
		if i >= limit {
			break
		}

		user := m.User.Username
		if user == "" {
			user = "?"
		}

		body := strings.ReplaceAll(m.Body, "\n", " ")
		body = strings.TrimSpace(body)
		if len(body) > 280 {
			body = body[:280] + "…"
		}

		tag := "no-label"
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

		lines = append(lines, fmt.Sprintf("[%s · @%s · %s] %s", m.CreatedAt, user, tag, body))
	}

	total := bullish + bearish + unlabeled
	bullPct := 0
	bearPct := 0
	if total > 0 {
		bullPct = int(float64(bullish*100)/float64(total) + 0.5)
		bearPct = int(float64(bearish*100)/float64(total) + 0.5)
	}

	summary := fmt.Sprintf("Bullish: %d (%d%%) · Bearish: %d (%d%%) · Unlabeled: %d · Total: %d most-recent messages",
		bullish, bullPct, bearish, bearPct, unlabeled, total)

	return summary + "\n\n" + strings.Join(lines, "\n"), nil
}
