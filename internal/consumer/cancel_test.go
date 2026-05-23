package consumer_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-hclog"

	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/arr"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/consumer"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/event"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/store"
)

// ---------------------------------------------------------------------------
// Cancel-specific helpers
// ---------------------------------------------------------------------------

// newCancelHandler builds a CancelHandler wired to the given store.
func newCancelHandler(st *store.Store) *consumer.CancelHandler {
	return &consumer.CancelHandler{
		Store:     st,
		Radarr:    arr.NewRadarr,
		Sonarr:    arr.NewSonarr,
		Events:    event.New(nil, hclog.NewNullLogger()),
		SecretKey: testSecretKey,
		Log:       hclog.NewNullLogger(),
	}
}

// insertSubmittedRadarr inserts a registered_arr + a request row with
// status='submitted', routed_arr_id and external_id set. Returns the
// registered_arr id and request id.
func insertSubmittedRadarr(
	t *testing.T,
	ctx context.Context,
	st *store.Store,
	requestID string,
	srvURL string,
	externalID int,
) int64 {
	t.Helper()
	arrID := insertRadarr(t, ctx, st, fmt.Sprintf("arr-%s", requestID), srvURL)

	r := &store.Request{
		ID:        requestID,
		TMDBID:    1001,
		MediaType: "movie",
		Title:     "Test Movie",
		Year:      2024,
		Status:    "queued",
	}
	if err := st.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.SetRoutedArr(ctx, requestID, arrID, []byte(`{}`)); err != nil {
		t.Fatalf("SetRoutedArr: %v", err)
	}
	if err := st.MarkSubmitted(ctx, requestID, externalID); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}
	return arrID
}

// cancelDeleteServer builds an httptest.Server that responds to DELETE
// /api/v3/movie/{id} and /api/v3/series/{id} with the given status code,
// incrementing deleteCount on each DELETE hit.
func cancelDeleteServer(t *testing.T, deleteStatus int, deleteCount *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			if deleteCount != nil {
				deleteCount.Add(1)
			}
			w.WriteHeader(deleteStatus)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}

	mux.HandleFunc("/api/v3/movie/", handler)
	mux.HandleFunc("/api/v3/series/", handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCancelUnknownPayloadIsNoOp — payload missing requestId → no-op, no error.
func TestCancelUnknownPayloadIsNoOp(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	h := newCancelHandler(st)

	// Payload with no requestId at all.
	if err := h.HandleCancelled(ctx, map[string]any{"foo": "bar"}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Payload with empty string requestId.
	if err := h.HandleCancelled(ctx, map[string]any{"requestId": ""}); err != nil {
		t.Fatalf("expected nil error for empty requestId, got %v", err)
	}

	// Confirm nothing was written.
	rows, _, err := st.ListRequestsForAdmin(ctx, "", 100, 0)
	if err != nil {
		t.Fatalf("ListRequestsForAdmin: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

// TestCancelUnknownIDIsNoOp — requestId not in DB → no-op.
func TestCancelUnknownIDIsNoOp(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	h := newCancelHandler(st)

	if err := h.HandleCancelled(ctx, map[string]any{"requestId": "does-not-exist"}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestCancelTerminalIsNoOp — row already imported → no DELETE call, status unchanged.
func TestCancelTerminalIsNoOp(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var deleteCalls atomic.Int32
	srv := cancelDeleteServer(t, http.StatusOK, &deleteCalls)
	h := newCancelHandler(st)

	insertSubmittedRadarr(t, ctx, st, "req-terminal", srv.URL, 99)

	// Manually transition to imported (terminal).
	if err := st.MarkImported(ctx, "req-terminal"); err != nil {
		t.Fatalf("MarkImported: %v", err)
	}

	if err := h.HandleCancelled(ctx, map[string]any{"requestId": "req-terminal"}); err != nil {
		t.Fatalf("HandleCancelled: %v", err)
	}

	if deleteCalls.Load() != 0 {
		t.Errorf("expected 0 DELETE calls, got %d", deleteCalls.Load())
	}

	row, err := st.GetRequest(ctx, "req-terminal")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "imported" {
		t.Errorf("status = %q, want imported (terminal should not change)", row.Status)
	}
}

// TestCancelSubmittedDeletesAtArrAndMarks — submitted row with routed_arr_id and
// external_id → DELETE called at arr, status transitions to cancelled.
func TestCancelSubmittedDeletesAtArrAndMarks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var deleteCalls atomic.Int32
	srv := cancelDeleteServer(t, http.StatusOK, &deleteCalls)
	h := newCancelHandler(st)

	insertSubmittedRadarr(t, ctx, st, "req-cancel-submitted", srv.URL, 42)

	if err := h.HandleCancelled(ctx, map[string]any{"requestId": "req-cancel-submitted"}); err != nil {
		t.Fatalf("HandleCancelled: %v", err)
	}

	if deleteCalls.Load() != 1 {
		t.Errorf("expected 1 DELETE call, got %d", deleteCalls.Load())
	}

	row, err := st.GetRequest(ctx, "req-cancel-submitted")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", row.Status)
	}
}

// TestCancelDownloadingDeletesAtArrAndMarks — same as TestCancelSubmittedDeletesAtArrAndMarks
// but starts from downloading.
func TestCancelDownloadingDeletesAtArrAndMarks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var deleteCalls atomic.Int32
	srv := cancelDeleteServer(t, http.StatusOK, &deleteCalls)
	h := newCancelHandler(st)

	insertSubmittedRadarr(t, ctx, st, "req-cancel-downloading", srv.URL, 77)

	// Transition to downloading.
	if _, err := st.MarkDownloading(ctx, "req-cancel-downloading"); err != nil {
		t.Fatalf("MarkDownloading: %v", err)
	}

	if err := h.HandleCancelled(ctx, map[string]any{"requestId": "req-cancel-downloading"}); err != nil {
		t.Fatalf("HandleCancelled: %v", err)
	}

	if deleteCalls.Load() != 1 {
		t.Errorf("expected 1 DELETE call, got %d", deleteCalls.Load())
	}

	row, err := st.GetRequest(ctx, "req-cancel-downloading")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", row.Status)
	}
}

// TestCancelArrUnreachableStillMarksLocal — arr returns 500 → MarkCancelled
// still runs; status becomes cancelled (best-effort).
func TestCancelArrUnreachableStillMarksLocal(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var deleteCalls atomic.Int32
	srv := cancelDeleteServer(t, http.StatusInternalServerError, &deleteCalls)
	h := newCancelHandler(st)

	insertSubmittedRadarr(t, ctx, st, "req-cancel-500", srv.URL, 55)

	if err := h.HandleCancelled(ctx, map[string]any{"requestId": "req-cancel-500"}); err != nil {
		t.Fatalf("HandleCancelled should not return error on arr failure, got %v", err)
	}

	// DELETE was attempted (the server was hit, it just returned 500 — but
	// Radarr.DeleteMovie silently ignores 404; for 500 it returns an error that
	// tryArrDelete swallows).
	if deleteCalls.Load() != 1 {
		t.Errorf("expected 1 DELETE attempt, got %d", deleteCalls.Load())
	}

	row, err := st.GetRequest(ctx, "req-cancel-500")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled (arr error must not block local cancel)", row.Status)
	}
}

// TestCancelOrphanedRoutedArrSkipsDelete — row's routed_arr_id is nil (arr was
// deleted, propagated via ON DELETE SET NULL) → no DELETE attempted, local cancel runs.
func TestCancelOrphanedRoutedArrSkipsDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var deleteCalls atomic.Int32
	srv := cancelDeleteServer(t, http.StatusOK, &deleteCalls)
	h := newCancelHandler(st)

	arrID := insertSubmittedRadarr(t, ctx, st, "req-orphan", srv.URL, 33)

	// Delete the arr — ON DELETE SET NULL propagates to request.routed_arr_id.
	if err := st.DeleteArr(ctx, arrID); err != nil {
		t.Fatalf("DeleteArr: %v", err)
	}

	if err := h.HandleCancelled(ctx, map[string]any{"requestId": "req-orphan"}); err != nil {
		t.Fatalf("HandleCancelled: %v", err)
	}

	if deleteCalls.Load() != 0 {
		t.Errorf("expected 0 DELETE calls (arr gone), got %d", deleteCalls.Load())
	}

	row, err := st.GetRequest(ctx, "req-orphan")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", row.Status)
	}
}

// TestCancelExternalIDMissingSkipsDelete — row has routed_arr_id but no external_id
// (e.g. 409 path that didn't capture external_id) → no DELETE, local cancel runs.
func TestCancelExternalIDMissingSkipsDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var deleteCalls atomic.Int32
	srv := cancelDeleteServer(t, http.StatusOK, &deleteCalls)
	h := newCancelHandler(st)

	// Insert arr and a queued request, route it, but mark submitted with externalID=0
	// and then NULL it out by using the 409 path (externalID=0 → MarkSubmitted stores 0).
	// We need a row that has routed_arr_id but external_id IS NULL in the DB.
	// The easiest way: insert queued, SetRoutedArr, then leave it as queued
	// (external_id starts NULL). We need status != terminal though; queued is fine.
	arrID := insertRadarr(t, ctx, st, "arr-no-extid", srv.URL)

	r := &store.Request{
		ID:        "req-no-extid",
		TMDBID:    1001,
		MediaType: "movie",
		Title:     "Test Movie",
		Year:      2024,
		Status:    "queued",
	}
	if err := st.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.SetRoutedArr(ctx, "req-no-extid", arrID, []byte(`{}`)); err != nil {
		t.Fatalf("SetRoutedArr: %v", err)
	}
	// Do NOT call MarkSubmitted — external_id remains NULL.

	if err := h.HandleCancelled(ctx, map[string]any{"requestId": "req-no-extid"}); err != nil {
		t.Fatalf("HandleCancelled: %v", err)
	}

	if deleteCalls.Load() != 0 {
		t.Errorf("expected 0 DELETE calls (external_id nil), got %d", deleteCalls.Load())
	}

	row, err := st.GetRequest(ctx, "req-no-extid")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", row.Status)
	}
}

// TestCancelDispatcherIntegration — routes via Dispatcher.Handle for
// "plugin.silo.requests.cancelled". Confirms the wiring end-to-end.
func TestCancelDispatcherIntegration(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	var deleteCalls atomic.Int32
	srv := cancelDeleteServer(t, http.StatusOK, &deleteCalls)

	cancelH := newCancelHandler(st)
	submitH := newHandler(st, &noopEnricher{})

	d := consumer.New(submitH, cancelH, hclog.NewNullLogger())

	insertSubmittedRadarr(t, ctx, st, "req-disp-cancel", srv.URL, 11)

	if err := d.Handle(ctx, "plugin.silo.requests.cancelled", map[string]any{
		"requestId": "req-disp-cancel",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if deleteCalls.Load() != 1 {
		t.Errorf("expected 1 DELETE via dispatcher, got %d", deleteCalls.Load())
	}

	row, err := st.GetRequest(ctx, "req-disp-cancel")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", row.Status)
	}
}
