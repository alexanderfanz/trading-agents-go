package dataflow

import (
	"context"
	"time"
)

// NewsSocialProvider abstracts ticker-specific and global macro news/social ingestion flows.
type NewsSocialProvider interface {
	// FetchNews retrieves company-specific news articles between start and end dates.
	FetchNews(ctx context.Context, ticker string, start, end time.Time) (string, error)

	// FetchGlobalNews retrieves global macro economic news up to currDate.
	FetchGlobalNews(ctx context.Context, currDate time.Time, lookBackDays int, limit int) (string, error)

	// FetchStockTwits retrieves stocktwits message streams for the ticker.
	FetchStockTwits(ctx context.Context, ticker string, limit int) (string, error)

	// FetchReddit retrieves reddit posts mentioning ticker across subreddits.
	FetchReddit(ctx context.Context, ticker string, subreddits []string, limitPerSub int) (string, error)
}

// HTTPNewsSocialProvider implements NewsSocialProvider using a resilient HTTP client.
type HTTPNewsSocialProvider struct {
	client              *ResilientHTTPClient
	interRequestDelay   time.Duration
}

// NewHTTPNewsSocialProvider instantiates a new HTTPNewsSocialProvider.
func NewHTTPNewsSocialProvider(client *ResilientHTTPClient) *HTTPNewsSocialProvider {
	return &HTTPNewsSocialProvider{
		client:            client,
		interRequestDelay: 400 * time.Millisecond,
	}
}
