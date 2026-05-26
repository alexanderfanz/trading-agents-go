package dataflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	mockCookieName  = "A3"
	mockCookieValue = "mock_cookie_val"
	mockCookieFull  = "A3=mock_cookie_val"
	mockCrumbValue  = "mock_crumb_val"
)

func TestYahooSessionManager_GetCredentials(t *testing.T) {
	var fcHits int32
	var crumbHits int32

	// Setup a mock HTTP server that simulates fc.yahoo.com and the crumb endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fc":
			atomic.AddInt32(&fcHits, 1)
			// Return a cookie and a 404 status (just like Yahoo)
			http.SetCookie(w, &http.Cookie{
				Name:     mockCookieName,
				Value:    mockCookieValue,
				Secure:   true,
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			w.WriteHeader(http.StatusNotFound)

		case "/getcrumb":
			atomic.AddInt32(&crumbHits, 1)
			// Check for correct cookie
			cookie, err := r.Cookie(mockCookieName)
			if err != nil || cookie.Value != mockCookieValue {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("Unauthorized - missing or invalid cookie"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockCrumbValue))

		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	sm := &YahooSessionManager{
		fcURL:    server.URL + "/fc",
		crumbURL: server.URL + "/getcrumb",
	}

	ctx := context.Background()

	// 1. Initial Retrieve
	cookie, crumb, err := sm.GetCredentials(ctx)
	if err != nil {
		t.Fatalf("Unexpected error fetching credentials: %v", err)
	}

	if cookie != mockCookieFull {
		t.Errorf("Expected cookie '%s', got '%s'", mockCookieFull, cookie)
	}

	if crumb != mockCrumbValue {
		t.Errorf("Expected crumb '%s', got '%s'", mockCrumbValue, crumb)
	}

	if atomic.LoadInt32(&fcHits) != 1 {
		t.Errorf("Expected exactly 1 fc hit, got %d", fcHits)
	}

	if atomic.LoadInt32(&crumbHits) != 1 {
		t.Errorf("Expected exactly 1 crumb hit, got %d", crumbHits)
	}

	// 2. Secondary Retrieve (Should use cached value, no server hits)
	cookie2, crumb2, err := sm.GetCredentials(ctx)
	if err != nil {
		t.Fatalf("Unexpected error on second credentials call: %v", err)
	}

	if cookie2 != mockCookieFull || crumb2 != mockCrumbValue {
		t.Errorf("Cached credentials changed: got (%s, %s)", cookie2, crumb2)
	}

	if atomic.LoadInt32(&fcHits) != 1 {
		t.Errorf("Expected cached credentials to not increment fc hits, but hits is %d", fcHits)
	}

	if atomic.LoadInt32(&crumbHits) != 1 {
		t.Errorf("Expected cached credentials to not increment crumb hits, but hits is %d", crumbHits)
	}

	// 3. Test concurrent safety and double-checked locking (dog-piling prevention)
	// Clear the credentials to trigger a fresh sweep
	sm.Invalidate()

	var wg sync.WaitGroup
	var concurrentErr error
	var errOnce sync.Once

	numGoroutines := 10
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, cr, gErr := sm.GetCredentials(ctx)
			if gErr != nil {
				errOnce.Do(func() {
					concurrentErr = gErr
				})
				return
			}
			if c != mockCookieFull || cr != mockCrumbValue {
				errOnce.Do(func() {
					concurrentErr = fmt.Errorf("incorrect credentials returned: got (%s, %s)", c, cr)
				})
			}
		}()
	}
	wg.Wait()

	if concurrentErr != nil {
		t.Fatalf("Error during concurrent execution: %v", concurrentErr)
	}

	// Because of double-checked locking, only 1 new sweep should have been performed
	if atomic.LoadInt32(&fcHits) != 2 {
		t.Errorf("Expected exactly 2 cumulative fc hits after concurrent sweep, got %d", fcHits)
	}

	if atomic.LoadInt32(&crumbHits) != 2 {
		t.Errorf("Expected exactly 2 cumulative crumb hits after concurrent sweep, got %d", crumbHits)
	}

	// 4. Invalidation Test
	sm.Invalidate()
	cookie3, crumb3, err := sm.GetCredentials(ctx)
	if err != nil {
		t.Fatalf("Unexpected error fetching credentials after invalidation: %v", err)
	}

	if cookie3 != mockCookieFull || crumb3 != mockCrumbValue {
		t.Errorf("Credentials fetched after invalidation are incorrect: (%s, %s)", cookie3, crumb3)
	}

	if atomic.LoadInt32(&fcHits) != 3 {
		t.Errorf("Expected 3 cumulative fc hits after invalidation and fetch, got %d", fcHits)
	}

	if atomic.LoadInt32(&crumbHits) != 3 {
		t.Errorf("Expected 3 cumulative crumb hits after invalidation and fetch, got %d", crumbHits)
	}
}
