package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/routing"
)

// Compile-time assertion in test package too.
var _ routing.Enricher = (*Cache)(nil)

// ---------------------------------------------------------------------------
// Helpers shared across cache tests
// ---------------------------------------------------------------------------

// movieJSON returns a minimal valid /movie/{id} JSON payload.
func movieJSON() map[string]any {
	return map[string]any{
		"original_language":    "en",
		"original_title":       "Test Movie",
		"genres":               []map[string]any{{"name": "Action"}},
		"runtime":              120,
		"vote_average":         7.5,
		"vote_count":           1000,
		"popularity":           50.0,
		"adult":                false,
		"status":               "Released",
		"production_companies": []map[string]any{},
		"production_countries": []map[string]any{},
		"spoken_languages":     []map[string]any{},
	}
}

// tvJSON returns a minimal valid /tv/{id} JSON payload.
func tvJSON() map[string]any {
	return map[string]any{
		"original_language":    "en",
		"original_name":        "Test Show",
		"genres":               []map[string]any{{"name": "Drama"}},
		"episode_run_time":     []int{45},
		"vote_average":         8.0,
		"vote_count":           5000,
		"popularity":           100.0,
		"adult":                false,
		"status":               "Ended",
		"production_companies": []map[string]any{},
		"production_countries": []map[string]any{},
		"spoken_languages":     []map[string]any{},
		"networks":             []map[string]any{},
		"origin_country":       []string{"US"},
		"created_by":           []map[string]any{},
	}
}

// keywordsMovieJSON returns a minimal valid /movie/{id}/keywords JSON payload.
func keywordsMovieJSON() map[string]any {
	return map[string]any{
		"keywords": []map[string]any{
			{"id": 1, "name": "action"},
			{"id": 2, "name": "hero"},
		},
	}
}

// contentRatingMovieJSON returns a minimal valid /movie/{id}/release_dates JSON
// payload with a US rating of "PG-13".
func contentRatingMovieJSON() map[string]any {
	return map[string]any{
		"results": []map[string]any{
			{
				"iso_3166_1": "US",
				"release_dates": []map[string]any{
					{"certification": "PG-13"},
				},
			},
		},
	}
}

// newCountingServer builds an httptest.Server that counts how many times
// each path prefix has been called. The handler dispatches to the provided
// map of path → response factory. If a path is not found it returns 404.
//
// callCount is incremented atomically for every request that hits the server,
// regardless of path.
func newCountingServer(t *testing.T, callCount *int64, routes map[string]func() (int, any)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(callCount, 1)
		for path, factory := range routes {
			if r.URL.Path == path {
				status, body := factory()
				writeJSON(w, status, body)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// TestCacheHitAvoidsSecondCall
// ---------------------------------------------------------------------------

func TestCacheHitAvoidsSecondCall(t *testing.T) {
	var calls int64
	srv := newCountingServer(t, &calls, map[string]func() (int, any){
		"/movie/550": func() (int, any) { return 200, movieJSON() },
	})

	cache := NewCache(newTestClient(srv), 24*time.Hour)

	if _, err := cache.Primary(context.Background(), "movie", 550); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := cache.Primary(context.Background(), "movie", 550); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("upstream calls: want 1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TestCacheTTLExpires
// ---------------------------------------------------------------------------

func TestCacheTTLExpires(t *testing.T) {
	var calls int64
	srv := newCountingServer(t, &calls, map[string]func() (int, any){
		"/movie/550": func() (int, any) { return 200, movieJSON() },
	})

	cache := NewCache(newTestClient(srv), time.Millisecond)

	// First call populates the cache at t=0.
	base := time.Now()
	cache.SetNow(func() time.Time { return base })

	if _, err := cache.Primary(context.Background(), "movie", 550); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Advance clock past TTL.
	cache.SetNow(func() time.Time { return base.Add(2 * time.Millisecond) })

	if _, err := cache.Primary(context.Background(), "movie", 550); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Errorf("upstream calls: want 2, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TestCacheUpstreamErrorIsNotCached
// ---------------------------------------------------------------------------

func TestCacheUpstreamErrorIsNotCached(t *testing.T) {
	var calls int64
	// First call returns 500; second call returns 200.
	var callNum int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&callNum, 1)
		atomic.AddInt64(&calls, 1)
		if n == 1 {
			writeJSON(w, 500, map[string]any{"status_message": "server error"})
		} else {
			writeJSON(w, 200, movieJSON())
		}
	}))
	t.Cleanup(srv.Close)

	cache := NewCache(newTestClient(srv), 24*time.Hour)

	// First call must error (not cached).
	_, err := cache.Primary(context.Background(), "movie", 1)
	if err == nil {
		t.Fatal("expected error on first call (500), got nil")
	}

	// Second call should hit upstream again (error was not cached).
	_, err = cache.Primary(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Errorf("upstream calls: want 2, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TestCacheGroupIsolation
// ---------------------------------------------------------------------------

func TestCacheGroupIsolation(t *testing.T) {
	var calls int64
	srv := newCountingServer(t, &calls, map[string]func() (int, any){
		"/movie/550":          func() (int, any) { return 200, movieJSON() },
		"/movie/550/keywords": func() (int, any) { return 200, keywordsMovieJSON() },
	})

	cache := NewCache(newTestClient(srv), 24*time.Hour)

	if _, err := cache.Primary(context.Background(), "movie", 550); err != nil {
		t.Fatalf("Primary call: %v", err)
	}
	if _, err := cache.Keywords(context.Background(), "movie", 550); err != nil {
		t.Fatalf("Keywords call: %v", err)
	}

	// Both Primary and Keywords should have made individual upstream calls.
	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Errorf("upstream calls: want 2, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TestCacheTTLZeroDisables
// ---------------------------------------------------------------------------

func TestCacheTTLZeroDisables(t *testing.T) {
	var calls int64
	srv := newCountingServer(t, &calls, map[string]func() (int, any){
		"/movie/1": func() (int, any) { return 200, movieJSON() },
	})

	cache := NewCache(newTestClient(srv), 0) // ttl=0 → caching disabled

	for i := 0; i < 3; i++ {
		if _, err := cache.Primary(context.Background(), "movie", 1); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	if got := atomic.LoadInt64(&calls); got != 3 {
		t.Errorf("upstream calls: want 3, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TestCacheKeysSeparateMovieAndTV
// ---------------------------------------------------------------------------

func TestCacheKeysSeparateMovieAndTV(t *testing.T) {
	var calls int64
	srv := newCountingServer(t, &calls, map[string]func() (int, any){
		"/movie/100": func() (int, any) { return 200, movieJSON() },
		"/tv/100":    func() (int, any) { return 200, tvJSON() },
	})

	cache := NewCache(newTestClient(srv), 24*time.Hour)

	if _, err := cache.Primary(context.Background(), "movie", 100); err != nil {
		t.Fatalf("movie call: %v", err)
	}
	if _, err := cache.Primary(context.Background(), "tv", 100); err != nil {
		t.Fatalf("tv call: %v", err)
	}

	// Different media types → different cache keys → 2 upstream calls.
	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Errorf("upstream calls: want 2, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TestCacheImplementsRoutingEnricher
// ---------------------------------------------------------------------------

// Covered by the package-level var _ at the top of this file and in cache.go.
// This explicit test makes the assertion visible in the test output.
func TestCacheImplementsRoutingEnricher(t *testing.T) {
	var _ routing.Enricher = (*Cache)(nil)
}

// ---------------------------------------------------------------------------
// TestCacheKeywordsCached
// ---------------------------------------------------------------------------

func TestCacheKeywordsCached(t *testing.T) {
	var calls int64
	srv := newCountingServer(t, &calls, map[string]func() (int, any){
		"/movie/550/keywords": func() (int, any) { return 200, keywordsMovieJSON() },
	})

	cache := NewCache(newTestClient(srv), 24*time.Hour)

	for i := 0; i < 2; i++ {
		if _, err := cache.Keywords(context.Background(), "movie", 550); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("upstream calls: want 1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TestCacheContentRatingCached
// ---------------------------------------------------------------------------

func TestCacheContentRatingCached(t *testing.T) {
	var calls int64
	srv := newCountingServer(t, &calls, map[string]func() (int, any){
		"/movie/550/release_dates": func() (int, any) { return 200, contentRatingMovieJSON() },
	})

	cache := NewCache(newTestClient(srv), 24*time.Hour)

	for i := 0; i < 2; i++ {
		if _, err := cache.ContentRating(context.Background(), "movie", 550); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("upstream calls: want 1, got %d", got)
	}
}
