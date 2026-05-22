package dataflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RedditResponse struct {
	Data RedditData `json:"data"`
}

type RedditData struct {
	Children []RedditChild `json:"children"`
}

type RedditChild struct {
	Data RedditPost `json:"data"`
}

type RedditPost struct {
	Title       string  `json:"title"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
	Selftext    string  `json:"selftext"`
}

// FetchReddit fetches recent Reddit posts mentioning ticker across subreddits.
func (p *HTTPNewsSocialProvider) FetchReddit(ctx context.Context, ticker string, subreddits []string, limitPerSub int) (string, error) {
	if len(subreddits) == 0 {
		subreddits = []string{"wallstreetbets", "stocks", "investing"}
	}

	var blocks []string
	var totalPosts int

	for i, sub := range subreddits {
		if i > 0 {
			// Enforce delay to protect IP from strict rate limiting
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.interRequestDelay):
			}
		}

		posts, err := p.fetchSubreddit(ctx, ticker, sub, limitPerSub)
		if err != nil {
			log.Printf("[WARN] Reddit fetch failed for r/%s · %s: %v", sub, ticker, err)
			blocks = append(blocks, fmt.Sprintf("r/%s: <reddit unavailable: %v>", sub, err))
			continue
		}

		totalPosts += len(posts)
		if len(posts) == 0 {
			blocks = append(blocks, fmt.Sprintf("r/%s: <no posts found mentioning %s in the past 7 days>", sub, strings.ToUpper(ticker)))
			continue
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("r/%s — %d recent posts mentioning %s:", sub, len(posts), strings.ToUpper(ticker)))

		for _, post := range posts {
			title := strings.ReplaceAll(post.Title, "\n", " ")
			title = strings.TrimSpace(title)

			selftext := strings.ReplaceAll(post.Selftext, "\n", " ")
			selftext = strings.TrimSpace(selftext)

			createdStr := "?"
			if post.CreatedUTC > 0 {
				createdStr = time.Unix(int64(post.CreatedUTC), 0).UTC().Format("2006-01-02")
			}

			if len(selftext) > 240 {
				runes := []rune(selftext)
				selftext = string(runes[:240]) + "…"
			}

			postLine := fmt.Sprintf("  [%s · %4d↑ · %3dc] %s", createdStr, post.Score, post.NumComments, title)
			if selftext != "" {
				postLine += "\n    body excerpt: " + selftext
			}
			lines = append(lines, postLine)
		}

		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	if totalPosts == 0 {
		return fmt.Sprintf("<no Reddit posts found mentioning %s across %s in the past 7 days>",
			strings.ToUpper(ticker), strings.Join(subreddits, ", ")), nil
	}

	return strings.Join(blocks, "\n\n"), nil
}

func (p *HTTPNewsSocialProvider) fetchSubreddit(ctx context.Context, ticker string, sub string, limit int) ([]RedditPost, error) {
	qs := url.Values{}
	qs.Set("q", ticker)
	qs.Set("restrict_sr", "on")
	qs.Set("sort", "new")
	qs.Set("t", "week")
	qs.Set("limit", fmt.Sprintf("%d", limit))

	apiURL := fmt.Sprintf("https://www.reddit.com/r/%s/search.json?%s", sub, qs.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "tradingagents/0.2 (+https://github.com/TauricResearch/TradingAgents)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status %d", resp.StatusCode)
	}

	var payload RedditResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var posts []RedditPost
	for _, child := range payload.Data.Children {
		posts = append(posts, child.Data)
	}

	return posts, nil
}
