package dataflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// YahooSessionManager manages thread-safe caching and retrieval of Yahoo Finance session cookies and crumb tokens.
type YahooSessionManager struct {
	mu        sync.RWMutex
	refreshMu sync.Mutex
	cookie    string
	crumb     string
	expiresAt time.Time

	// URLs for dependency injection (allowing unit tests to use httptest.Server)
	fcURL    string
	crumbURL string
}

// NewYahooSessionManager constructs a new session manager with standard Yahoo Finance endpoints.
func NewYahooSessionManager() *YahooSessionManager {
	return &YahooSessionManager{
		fcURL:    "https://fc.yahoo.com/",
		crumbURL: "https://query2.finance.yahoo.com/v1/test/getcrumb",
	}
}

// GetCredentials retrieves a valid cookie and crumb token. If credentials are not set or are expired,
// it triggers a new authentication sweep.
func (sm *YahooSessionManager) GetCredentials(ctx context.Context) (string, string, error) {
	// 1. Read Lock fast-path check
	sm.mu.RLock()
	cookie := sm.cookie
	crumb := sm.crumb
	isValid := cookie != "" && crumb != "" && time.Now().Before(sm.expiresAt)
	sm.mu.RUnlock()

	if isValid {
		return cookie, crumb, nil
	}

	// 2. Acquire refresh lock to serialize refresh attempts without blocking readers
	sm.refreshMu.Lock()
	defer sm.refreshMu.Unlock()

	// Double-check to prevent dog-piling / redundant sweeps
	sm.mu.RLock()
	cookie = sm.cookie
	crumb = sm.crumb
	isValid = cookie != "" && crumb != "" && time.Now().Before(sm.expiresAt)
	sm.mu.RUnlock()

	if isValid {
		return cookie, crumb, nil
	}

	cookieVal, crumbVal, err := sm.refresh(ctx)
	if err != nil {
		return "", "", fmt.Errorf("yahoo finance session sweep failed: %w", err)
	}

	sm.mu.Lock()
	sm.cookie = cookieVal
	sm.crumb = crumbVal
	sm.expiresAt = time.Now().Add(24 * time.Hour) // Cache session credentials for 24 hours
	sm.mu.Unlock()

	return cookieVal, crumbVal, nil
}

// Invalidate clears the cached cookie and crumb, forcing a refresh on the next request.
func (sm *YahooSessionManager) Invalidate() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cookie = ""
	sm.crumb = ""
	sm.expiresAt = time.Time{}
}

// refresh performs the actual two-step authentication sweep.
func (sm *YahooSessionManager) refresh(ctx context.Context) (string, string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
	}

	userAgent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// Step 1: GET fc.yahoo.com (capture session cookies)
	req1, err := http.NewRequestWithContext(ctx, "GET", sm.fcURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create fc request: %w", err)
	}
	req1.Header.Set("User-Agent", userAgent)
	req1.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")

	resp1, err := client.Do(req1)
	if err != nil {
		return "", "", fmt.Errorf("fc request failed: %w", err)
	}
	defer resp1.Body.Close()

	// Locate A3 or B cookie, or fall back to first cookie present
	var targetCookie *http.Cookie
	for _, c := range resp1.Cookies() {
		if c.Name == "A3" || c.Name == "B" {
			targetCookie = c
			break
		}
	}

	// If not found in response cookies directly, check the jar
	if targetCookie == nil {
		u, err := url.Parse(sm.fcURL)
		if err == nil {
			for _, c := range jar.Cookies(u) {
				if c.Name == "A3" || c.Name == "B" {
					targetCookie = c
					break
				}
			}
		}
	}

	// Fallback to first available cookie
	if targetCookie == nil && len(resp1.Cookies()) > 0 {
		targetCookie = resp1.Cookies()[0]
	}
	if targetCookie == nil {
		u, err := url.Parse(sm.fcURL)
		if err == nil {
			cookies := jar.Cookies(u)
			if len(cookies) > 0 {
				targetCookie = cookies[0]
			}
		}
	}

	if targetCookie == nil {
		return "", "", fmt.Errorf("no cookie returned during session sweep from %s", sm.fcURL)
	}

	cookieVal := fmt.Sprintf("%s=%s", targetCookie.Name, targetCookie.Value)

	// Step 2: GET query2.finance.yahoo.com/v1/test/getcrumb with Step 1 cookie in header
	req2, err := http.NewRequestWithContext(ctx, "GET", sm.crumbURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create crumb request: %w", err)
	}
	req2.Header.Set("User-Agent", userAgent)
	req2.Header.Set("Cookie", cookieVal)

	resp2, err := client.Do(req2)
	if err != nil {
		return "", "", fmt.Errorf("crumb request failed: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("crumb request returned non-200 status code: %d", resp2.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read crumb body: %w", err)
	}

	crumbVal := strings.TrimSpace(string(bodyBytes))
	if crumbVal == "" {
		return "", "", fmt.Errorf("crumb value returned is empty")
	}

	return cookieVal, crumbVal, nil
}
