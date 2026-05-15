package consumer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/arr"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/consumer"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/store"
)

// ---------------------------------------------------------------------------
// Enricher fakes
// ---------------------------------------------------------------------------

// noopEnricher satisfies routing.Enricher and never makes real HTTP calls.
// All methods return nil/empty with no error — Group A rules don't need it.
type noopEnricher struct{}

func (n *noopEnricher) Primary(_ context.Context, _ string, _ int) (*routing.TMDBPrimary, error) {
	return nil, nil
}

func (n *noopEnricher) Keywords(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}

func (n *noopEnricher) ContentRating(_ context.Context, _ string, _ int) (string, error) {
	return "", nil
}

// errorPrimaryEnricher returns an error from Primary; other methods succeed.
// Used to verify that Group-A-only routing still works when Primary fails.
type errorPrimaryEnricher struct{}

func (e *errorPrimaryEnricher) Primary(_ context.Context, _ string, _ int) (*routing.TMDBPrimary, error) {
	return nil, fmt.Errorf("tmdb primary: simulated failure")
}

func (e *errorPrimaryEnricher) Keywords(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}

func (e *errorPrimaryEnricher) ContentRating(_ context.Context, _ string, _ int) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------
// arr httptest helpers
// ---------------------------------------------------------------------------

// radarrServer builds an httptest.Server that mocks the minimal Radarr API
// surface needed by SubmitHandler.
//   - movieID: value returned in POST /api/v3/movie response body.
//   - postStatus: HTTP status code for POST /api/v3/movie.
//   - postCount: if non-nil, incremented on each POST /api/v3/movie call.
func radarrServer(t *testing.T, movieID int, postStatus int, postCount *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v3/rootfolder", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "path": "/movies"}})
	})

	mux.HandleFunc("/api/v3/qualityprofile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "HD-1080p"}})
	})

	mux.HandleFunc("/api/v3/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if postCount != nil {
			postCount.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(postStatus)
		json.NewEncoder(w).Encode(map[string]any{
			"id":    movieID,
			"title": "Test Movie",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// Shared constants and helpers
// ---------------------------------------------------------------------------

const testSecretKey = "test-secret-key-arr-request-router"

// sealKey encrypts plaintext with the test secret key.
func sealKey(t *testing.T, plaintext string) string {
	t.Helper()
	sealed, err := crypto.Seal(testSecretKey, plaintext)
	if err != nil {
		t.Fatalf("seal key: %v", err)
	}
	return sealed
}

// insertRadarr inserts a radarr entry into the store and returns its ID.
// RulesJSON defaults to `{}` (empty object = catch-all rules).
func insertRadarr(t *testing.T, ctx context.Context, st *store.Store, name, srvURL string) int64 {
	t.Helper()
	id, err := st.CreateArr(ctx, &store.RegisteredArr{
		Name:      name,
		Kind:      "radarr",
		URL:       srvURL,
		APIKey:    sealKey(t, "test-api-key"),
		Enabled:   true,
		RulesJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}
	return id
}

// newHandler builds a SubmitHandler with the given store and enricher.
func newHandler(st *store.Store, enr routing.Enricher) *consumer.SubmitHandler {
	return &consumer.SubmitHandler{
		Store:     st,
		Enricher:  enr,
		Radarr:    arr.NewRadarr,
		Sonarr:    arr.NewSonarr,
		Events:    event.New(nil, hclog.NewNullLogger()),
		SecretKey: testSecretKey,
		Log:       hclog.NewNullLogger(),
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestSubmitHappyPath: movie payload, one radarr with empty (catch-all) rules.
// Expect: row queued then submitted, routed_arr_id set, external_id=42.
func TestSubmitHappyPath(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var postCount atomic.Int32
	srv := radarrServer(t, 42, http.StatusCreated, &postCount)
	h := newHandler(st, &noopEnricher{})

	arrID := insertRadarr(t, ctx, st, "arr1", srv.URL)

	payload := map[string]any{
		"requestId": "req-happy",
		"mediaType": "movie",
		"tmdbId":    float64(1001),
		"title":     "Test Movie",
		"year":      float64(2024),
	}
	if err := h.HandleSubmitted(ctx, payload); err != nil {
		t.Fatalf("HandleSubmitted: %v", err)
	}

	row, err := st.GetRequest(ctx, "req-happy")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row == nil {
		t.Fatal("expected row, got nil")
	}
	if row.Status != "submitted" {
		t.Errorf("status = %q, want submitted", row.Status)
	}
	if row.RoutedArrID == nil || *row.RoutedArrID != arrID {
		t.Errorf("routed_arr_id = %v, want %d", row.RoutedArrID, arrID)
	}
	if row.ExternalID == nil || *row.ExternalID != 42 {
		t.Errorf("external_id = %v, want 42", row.ExternalID)
	}
	if postCount.Load() != 1 {
		t.Errorf("POST /api/v3/movie called %d times, want 1", postCount.Load())
	}
}

// TestSubmitNoMatchUnrouted: empty registry for movie.
// Expect: row inserted and set to unrouted.
func TestSubmitNoMatchUnrouted(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	h := newHandler(st, &noopEnricher{})

	payload := map[string]any{
		"requestId": "req-unrouted",
		"mediaType": "movie",
		"tmdbId":    float64(2002),
	}
	if err := h.HandleSubmitted(ctx, payload); err != nil {
		t.Fatalf("HandleSubmitted: %v", err)
	}

	row, err := st.GetRequest(ctx, "req-unrouted")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if row.Status != "unrouted" {
		t.Errorf("status = %q, want unrouted", row.Status)
	}
	if row.Error == "" {
		t.Error("expected non-empty error message on unrouted row")
	}
}

// TestSubmitArrReturns409TreatedAsSubmitted: arr POST returns 409.
// Expect: status='submitted', not 'failed'.
func TestSubmitArrReturns409TreatedAsSubmitted(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	srv := radarrServer(t, 0, http.StatusConflict, nil)
	h := newHandler(st, &noopEnricher{})
	insertRadarr(t, ctx, st, "arr-409", srv.URL)

	payload := map[string]any{
		"requestId": "req-409",
		"mediaType": "movie",
		"tmdbId":    float64(3003),
	}
	if err := h.HandleSubmitted(ctx, payload); err != nil {
		t.Fatalf("HandleSubmitted: %v", err)
	}

	row, err := st.GetRequest(ctx, "req-409")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if row.Status != "submitted" {
		t.Errorf("status = %q, want submitted (409 treated as already exists)", row.Status)
	}
}

// TestSubmitArrReturns500MarksFailed: arr POST returns 500.
// Expect: status='failed' with error message.
func TestSubmitArrReturns500MarksFailed(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	srv := radarrServer(t, 0, http.StatusInternalServerError, nil)
	h := newHandler(st, &noopEnricher{})
	insertRadarr(t, ctx, st, "arr-500", srv.URL)

	payload := map[string]any{
		"requestId": "req-500",
		"mediaType": "movie",
		"tmdbId":    float64(4004),
	}
	if err := h.HandleSubmitted(ctx, payload); err != nil {
		t.Fatalf("HandleSubmitted: %v", err)
	}

	row, err := st.GetRequest(ctx, "req-500")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if row.Status != "failed" {
		t.Errorf("status = %q, want failed", row.Status)
	}
	if row.Error == "" {
		t.Error("expected non-empty error on failed row")
	}
}

// TestSubmitDoesNotFallThroughToNextArr: two radarrs, first returns 500.
// Expect: failed immediately, no POST attempted to second arr.
func TestSubmitDoesNotFallThroughToNextArr(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var arr1Posts atomic.Int32
	srv1 := radarrServer(t, 0, http.StatusInternalServerError, &arr1Posts)

	var arr2Posts atomic.Int32
	srv2 := radarrServer(t, 99, http.StatusCreated, &arr2Posts)

	h := newHandler(st, &noopEnricher{})

	// Priority 1 (higher priority) — will 500
	_, err := st.CreateArr(ctx, &store.RegisteredArr{
		Name:      "arr1-primary",
		Kind:      "radarr",
		URL:       srv1.URL,
		APIKey:    sealKey(t, "key1"),
		Enabled:   true,
		Priority:  1,
		RulesJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateArr arr1: %v", err)
	}

	// Priority 2 (lower priority) — should not be contacted
	_, err = st.CreateArr(ctx, &store.RegisteredArr{
		Name:      "arr2-secondary",
		Kind:      "radarr",
		URL:       srv2.URL,
		APIKey:    sealKey(t, "key2"),
		Enabled:   true,
		Priority:  2,
		RulesJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateArr arr2: %v", err)
	}

	payload := map[string]any{
		"requestId": "req-no-fallthrough",
		"mediaType": "movie",
		"tmdbId":    float64(5005),
	}
	if err := h.HandleSubmitted(ctx, payload); err != nil {
		t.Fatalf("HandleSubmitted: %v", err)
	}

	row, err := st.GetRequest(ctx, "req-no-fallthrough")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if row.Status != "failed" {
		t.Errorf("status = %q, want failed", row.Status)
	}
	if arr1Posts.Load() != 1 {
		t.Errorf("arr1 POST count = %d, want 1", arr1Posts.Load())
	}
	if arr2Posts.Load() != 0 {
		t.Errorf("arr2 POST count = %d, want 0 (no fallthrough)", arr2Posts.Load())
	}
}

// TestSubmitTMDBPrimaryFailureStillRoutes: enricher errors on Primary but
// rules are Group A only, so routing must succeed regardless.
func TestSubmitTMDBPrimaryFailureStillRoutes(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	srv := radarrServer(t, 77, http.StatusCreated, nil)
	h := newHandler(st, &errorPrimaryEnricher{})

	insertRadarr(t, ctx, st, "arr-group-a-only", srv.URL)

	payload := map[string]any{
		"requestId": "req-tmdb-fail",
		"mediaType": "movie",
		"tmdbId":    float64(6006),
	}
	if err := h.HandleSubmitted(ctx, payload); err != nil {
		t.Fatalf("HandleSubmitted: %v", err)
	}

	row, err := st.GetRequest(ctx, "req-tmdb-fail")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if row.Status != "submitted" {
		t.Errorf("status = %q, want submitted (Group A rules, Primary failure irrelevant)", row.Status)
	}
}

// TestSubmitParsePayloadInvalidReturnsErrorAndDoesNotInsert: missing requestId.
// Expect: error returned, no row inserted.
func TestSubmitParsePayloadInvalidReturnsErrorAndDoesNotInsert(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	h := newHandler(st, &noopEnricher{})

	payload := map[string]any{
		// requestId intentionally missing
		"mediaType": "movie",
		"tmdbId":    float64(7007),
	}
	err := h.HandleSubmitted(ctx, payload)
	if err == nil {
		t.Fatal("expected error for missing requestId, got nil")
	}

	rows, _, listErr := st.ListRequestsForAdmin(ctx, "", 100, 0)
	if listErr != nil {
		t.Fatalf("ListRequestsForAdmin: %v", listErr)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows after parse failure, got %d", len(rows))
	}
}

// TestSubmitMatchTraceStored: happy path; verify match_trace is stored as
// non-empty valid JSON.
func TestSubmitMatchTraceStored(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	srv := radarrServer(t, 55, http.StatusCreated, nil)
	h := newHandler(st, &noopEnricher{})
	insertRadarr(t, ctx, st, "arr-trace-test", srv.URL)

	payload := map[string]any{
		"requestId": "req-trace",
		"mediaType": "movie",
		"tmdbId":    float64(8008),
		"title":     "Trace Movie",
	}
	if err := h.HandleSubmitted(ctx, payload); err != nil {
		t.Fatalf("HandleSubmitted: %v", err)
	}

	row, err := st.GetRequest(ctx, "req-trace")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if len(row.MatchTrace) == 0 {
		t.Error("expected non-empty match_trace")
	}
	var parsed map[string]any
	if err := json.Unmarshal(row.MatchTrace, &parsed); err != nil {
		t.Errorf("match_trace is not valid JSON: %v\n%s", err, row.MatchTrace)
	}
}
