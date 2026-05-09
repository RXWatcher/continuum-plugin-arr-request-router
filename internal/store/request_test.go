package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

// sampleRequest returns a minimal Request suitable for insertion in tests.
func sampleRequest(id string) *store.Request {
	return &store.Request{
		ID:              id,
		TMDBID:          12345,
		MediaType:       "movie",
		Title:           "Test Movie",
		Year:            2024,
		PosterURL:       "http://example.com/poster.jpg",
		RequesterUserID: "user-1",
	}
}

// insertSampleArr inserts a minimal RegisteredArr and returns its ID.
// Used by tests that need a valid FK for routed_arr_id.
func insertSampleArr(t *testing.T, s *store.Store, ctx context.Context) int64 {
	t.Helper()
	a := sampleArr("FK Arr", "radarr")
	id, err := s.CreateArr(ctx, a)
	if err != nil {
		t.Fatalf("insertSampleArr: %v", err)
	}
	return id
}

// TestUpsertRequestQueuedInsertsThenIsNoop verifies that the first call inserts
// with status=queued, and a subsequent call with the same id does NOT overwrite
// a row whose status has since changed.
func TestUpsertRequestQueuedInsertsThenIsNoop(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-upsert-noop")

	// First insert.
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued (first): %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest after first upsert: %v", err)
	}
	if got == nil {
		t.Fatal("GetRequest returned nil after first upsert")
	}
	if got.Status != "queued" {
		t.Errorf("Status after first upsert: got %q, want %q", got.Status, "queued")
	}

	// Advance status to submitted.
	if err := s.MarkSubmitted(ctx, r.ID, 99); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	// Second upsert with same id — must be a no-op.
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued (second): %v", err)
	}

	got2, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest after second upsert: %v", err)
	}
	if got2 == nil {
		t.Fatal("GetRequest returned nil after second upsert")
	}
	if got2.Status != "submitted" {
		t.Errorf("Status after second upsert: got %q, want %q (should be no-op)", got2.Status, "submitted")
	}
}

// TestSetRoutedArrStoresArrIDAndTrace inserts a queued request, calls
// SetRoutedArr, and verifies routed_arr_id and match_trace are stored.
func TestSetRoutedArrStoresArrIDAndTrace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	arrID := insertSampleArr(t, s, ctx)

	r := sampleRequest("req-set-routed")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}

	trace := []byte(`{"matched_arr":1,"score":0.9}`)
	if err := s.SetRoutedArr(ctx, r.ID, arrID, trace); err != nil {
		t.Fatalf("SetRoutedArr: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got == nil {
		t.Fatal("GetRequest returned nil")
	}
	if got.RoutedArrID == nil {
		t.Fatal("RoutedArrID: got nil, want non-nil")
	}
	if *got.RoutedArrID != arrID {
		t.Errorf("RoutedArrID: got %d, want %d", *got.RoutedArrID, arrID)
	}
	if !jsonEqual(t, got.MatchTrace, trace) {
		t.Errorf("MatchTrace: got %s, want %s", got.MatchTrace, trace)
	}
}

// TestMarkSubmittedSetsStatusExternalIDAndTimestamp verifies that MarkSubmitted
// sets status=submitted, external_id, and submitted_at.
func TestMarkSubmittedSetsStatusExternalIDAndTimestamp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-submitted")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}

	if err := s.MarkSubmitted(ctx, r.ID, 42); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got == nil {
		t.Fatal("GetRequest returned nil")
	}
	if got.Status != "submitted" {
		t.Errorf("Status: got %q, want %q", got.Status, "submitted")
	}
	if got.ExternalID == nil {
		t.Fatal("ExternalID: got nil, want non-nil")
	}
	if *got.ExternalID != 42 {
		t.Errorf("ExternalID: got %d, want 42", *got.ExternalID)
	}
	if got.SubmittedAt == nil {
		t.Error("SubmittedAt: got nil, want non-nil")
	}
}

// TestMarkDownloadingTransitionsOnceReturnsTrue verifies that the first
// MarkDownloading call on a submitted row returns (true, nil) and a second
// call returns (false, nil) — status stays downloading.
func TestMarkDownloadingTransitionsOnceReturnsTrue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-downloading-once")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkSubmitted(ctx, r.ID, 7); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	// First transition: submitted → downloading.
	transitioned, err := s.MarkDownloading(ctx, r.ID)
	if err != nil {
		t.Fatalf("MarkDownloading (first): %v", err)
	}
	if !transitioned {
		t.Error("MarkDownloading (first): transitioned=false, want true")
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "downloading" {
		t.Errorf("Status after first MarkDownloading: got %q, want %q", got.Status, "downloading")
	}

	// Second call: already downloading → no-op.
	transitioned2, err := s.MarkDownloading(ctx, r.ID)
	if err != nil {
		t.Fatalf("MarkDownloading (second): %v", err)
	}
	if transitioned2 {
		t.Error("MarkDownloading (second): transitioned=true, want false")
	}

	got2, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest (after second): %v", err)
	}
	if got2.Status != "downloading" {
		t.Errorf("Status after second MarkDownloading: got %q, want %q", got2.Status, "downloading")
	}
}

// TestMarkDownloadingFromQueuedTransitions verifies that MarkDownloading can
// also transition a queued row (not just submitted).
func TestMarkDownloadingFromQueuedTransitions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-downloading-from-queued")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}

	transitioned, err := s.MarkDownloading(ctx, r.ID)
	if err != nil {
		t.Fatalf("MarkDownloading: %v", err)
	}
	if !transitioned {
		t.Error("MarkDownloading: transitioned=false, want true")
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "downloading" {
		t.Errorf("Status: got %q, want %q", got.Status, "downloading")
	}
}

// TestMarkImportedSetsCompletedAt verifies that MarkImported sets
// status=imported and completed_at.
func TestMarkImportedSetsCompletedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-imported")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	// MarkImported is only valid from submitted or downloading.
	if err := s.MarkSubmitted(ctx, r.ID, 10); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	if err := s.MarkImported(ctx, r.ID); err != nil {
		t.Fatalf("MarkImported: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got == nil {
		t.Fatal("GetRequest returned nil")
	}
	if got.Status != "imported" {
		t.Errorf("Status: got %q, want %q", got.Status, "imported")
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt: got nil, want non-nil")
	}
}

// TestMarkFailedSetsErrorAndCompletedAt verifies that MarkFailed sets
// status=failed, error, and completed_at.
func TestMarkFailedSetsErrorAndCompletedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-failed")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}

	if err := s.MarkFailed(ctx, r.ID, "boom"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got == nil {
		t.Fatal("GetRequest returned nil")
	}
	if got.Status != "failed" {
		t.Errorf("Status: got %q, want %q", got.Status, "failed")
	}
	if got.Error != "boom" {
		t.Errorf("Error: got %q, want %q", got.Error, "boom")
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt: got nil, want non-nil")
	}
}

// TestMarkCancelledSetsCompletedAt verifies that MarkCancelled sets
// status=cancelled and completed_at.
func TestMarkCancelledSetsCompletedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-cancelled")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}

	if err := s.MarkCancelled(ctx, r.ID); err != nil {
		t.Fatalf("MarkCancelled: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got == nil {
		t.Fatal("GetRequest returned nil")
	}
	if got.Status != "cancelled" {
		t.Errorf("Status: got %q, want %q", got.Status, "cancelled")
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt: got nil, want non-nil")
	}
}

// TestMarkUnroutedStoresTraceAndCompletedAt verifies that MarkUnrouted sets
// status=unrouted, stores match_trace, sets error to the reason, and sets
// completed_at.
func TestMarkUnroutedStoresTraceAndCompletedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-unrouted")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}

	trace := []byte(`{"evaluated":3,"matched":0}`)
	if err := s.MarkUnrouted(ctx, r.ID, trace, "no match"); err != nil {
		t.Fatalf("MarkUnrouted: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got == nil {
		t.Fatal("GetRequest returned nil")
	}
	if got.Status != "unrouted" {
		t.Errorf("Status: got %q, want %q", got.Status, "unrouted")
	}
	if got.Error != "no match" {
		t.Errorf("Error: got %q, want %q", got.Error, "no match")
	}
	if !jsonEqual(t, got.MatchTrace, trace) {
		t.Errorf("MatchTrace: got %s, want %s", got.MatchTrace, trace)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt: got nil, want non-nil")
	}
}

// TestListPollableReturnsOnlySubmittedAndDownloading inserts rows in every
// status and confirms that ListPollable returns only submitted and downloading
// rows.
func TestListPollableReturnsOnlySubmittedAndDownloading(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	statuses := []string{"queued", "submitted", "downloading", "imported", "failed", "cancelled", "unrouted"}

	for _, status := range statuses {
		id := "req-pollable-" + status
		r := sampleRequest(id)
		if err := s.UpsertRequestQueued(ctx, r); err != nil {
			t.Fatalf("UpsertRequestQueued(%s): %v", status, err)
		}
		// Transition to target status.
		switch status {
		case "queued":
			// Already queued; nothing to do.
		case "submitted":
			if err := s.MarkSubmitted(ctx, id, 1); err != nil {
				t.Fatalf("MarkSubmitted(%s): %v", id, err)
			}
		case "downloading":
			if err := s.MarkSubmitted(ctx, id, 2); err != nil {
				t.Fatalf("MarkSubmitted(%s): %v", id, err)
			}
			if _, err := s.MarkDownloading(ctx, id); err != nil {
				t.Fatalf("MarkDownloading(%s): %v", id, err)
			}
		case "imported":
			if err := s.MarkSubmitted(ctx, id, 3); err != nil {
				t.Fatalf("MarkSubmitted(%s): %v", id, err)
			}
			if err := s.MarkImported(ctx, id); err != nil {
				t.Fatalf("MarkImported(%s): %v", id, err)
			}
		case "failed":
			if err := s.MarkFailed(ctx, id, "err"); err != nil {
				t.Fatalf("MarkFailed(%s): %v", id, err)
			}
		case "cancelled":
			if err := s.MarkCancelled(ctx, id); err != nil {
				t.Fatalf("MarkCancelled(%s): %v", id, err)
			}
		case "unrouted":
			if err := s.MarkUnrouted(ctx, id, nil, "no match"); err != nil {
				t.Fatalf("MarkUnrouted(%s): %v", id, err)
			}
		}
	}

	pollable, err := s.ListPollable(ctx)
	if err != nil {
		t.Fatalf("ListPollable: %v", err)
	}
	if len(pollable) != 2 {
		t.Fatalf("ListPollable: got %d rows, want 2", len(pollable))
	}
	for _, row := range pollable {
		if row.Status != "submitted" && row.Status != "downloading" {
			t.Errorf("ListPollable returned unexpected status %q for id %q", row.Status, row.ID)
		}
	}
}

// TestUpdateLastPolledSetsTimestampWithoutChangingStatus verifies that
// UpdateLastPolled sets last_polled_at and does NOT change status or updated_at.
func TestUpdateLastPolledSetsTimestampWithoutChangingStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-last-polled")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkSubmitted(ctx, r.ID, 55); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	before, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest (before): %v", err)
	}

	pollTime := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.UpdateLastPolled(ctx, r.ID, pollTime); err != nil {
		t.Fatalf("UpdateLastPolled: %v", err)
	}

	after, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest (after): %v", err)
	}
	if after == nil {
		t.Fatal("GetRequest returned nil")
	}
	if after.Status != "submitted" {
		t.Errorf("Status changed: got %q, want %q", after.Status, "submitted")
	}
	if after.LastPolledAt == nil {
		t.Fatal("LastPolledAt: got nil, want non-nil")
	}
	// Verify last_polled_at was actually set (should be at or after pollTime).
	if after.LastPolledAt.Before(pollTime) {
		t.Errorf("LastPolledAt: got %v, want >= %v", after.LastPolledAt, pollTime)
	}
	// updated_at must not change (spec: UpdateLastPolled does NOT touch updated_at).
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("UpdatedAt changed: before=%v after=%v (should not change)", before.UpdatedAt, after.UpdatedAt)
	}
}

// TestListRequestsForAdminPaginatesAndFilters inserts 5 rows with mixed
// statuses and verifies pagination and status filtering.
func TestListRequestsForAdminPaginatesAndFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert 5 rows: 3 queued + 2 failed.
	for i := 0; i < 3; i++ {
		r := sampleRequest("req-admin-queued-" + string(rune('a'+i)))
		if err := s.UpsertRequestQueued(ctx, r); err != nil {
			t.Fatalf("UpsertRequestQueued(%s): %v", r.ID, err)
		}
	}
	for i := 0; i < 2; i++ {
		id := "req-admin-failed-" + string(rune('a'+i))
		r := sampleRequest(id)
		if err := s.UpsertRequestQueued(ctx, r); err != nil {
			t.Fatalf("UpsertRequestQueued(%s): %v", id, err)
		}
		if err := s.MarkFailed(ctx, id, "test fail"); err != nil {
			t.Fatalf("MarkFailed(%s): %v", id, err)
		}
	}

	// All rows, page 1 of 2 (limit=2).
	rows, total, err := s.ListRequestsForAdmin(ctx, "", 2, 0)
	if err != nil {
		t.Fatalf("ListRequestsForAdmin (all, limit=2): %v", err)
	}
	if total != 5 {
		t.Errorf("total: got %d, want 5", total)
	}
	if len(rows) != 2 {
		t.Errorf("rows: got %d, want 2", len(rows))
	}

	// Only failed rows.
	failedRows, failedTotal, err := s.ListRequestsForAdmin(ctx, "failed", 10, 0)
	if err != nil {
		t.Fatalf("ListRequestsForAdmin (failed): %v", err)
	}
	if failedTotal != 2 {
		t.Errorf("failedTotal: got %d, want 2", failedTotal)
	}
	if len(failedRows) != 2 {
		t.Errorf("failedRows count: got %d, want 2", len(failedRows))
	}
	for _, row := range failedRows {
		if row.Status != "failed" {
			t.Errorf("unexpected status %q in failed filter results", row.Status)
		}
	}
}

// TestMarkImportedIsNoOpFromTerminal drives a request to cancelled, then calls
// MarkImported, and asserts the row remains cancelled.
func TestMarkImportedIsNoOpFromTerminal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-imported-noop-terminal")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkCancelled(ctx, r.ID); err != nil {
		t.Fatalf("MarkCancelled: %v", err)
	}

	// Now attempt MarkImported on an already-terminal row.
	if err := s.MarkImported(ctx, r.ID); err != nil {
		t.Fatalf("MarkImported (should be no-op): %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "cancelled" {
		t.Errorf("Status: got %q, want %q (MarkImported must not overwrite terminal)", got.Status, "cancelled")
	}
}

// TestMarkFailedIsNoOpFromTerminal drives a request to imported, then calls
// MarkFailed, and asserts the row remains imported with no error set.
func TestMarkFailedIsNoOpFromTerminal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-failed-noop-terminal")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	// MarkImported is only valid from submitted or downloading.
	if err := s.MarkSubmitted(ctx, r.ID, 11); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}
	if err := s.MarkImported(ctx, r.ID); err != nil {
		t.Fatalf("MarkImported: %v", err)
	}

	// Now attempt MarkFailed on an already-terminal row.
	if err := s.MarkFailed(ctx, r.ID, "late error"); err != nil {
		t.Fatalf("MarkFailed (should be no-op): %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "imported" {
		t.Errorf("Status: got %q, want %q (MarkFailed must not overwrite terminal)", got.Status, "imported")
	}
	if got.Error != "" {
		t.Errorf("Error: got %q, want empty (MarkFailed must not set error on terminal row)", got.Error)
	}
}

// TestMarkCancelledIsNoOpFromTerminal drives a request to failed, then calls
// MarkCancelled, and asserts the row remains failed.
func TestMarkCancelledIsNoOpFromTerminal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-cancelled-noop-terminal")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkFailed(ctx, r.ID, "original error"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	// Now attempt MarkCancelled on an already-terminal row.
	if err := s.MarkCancelled(ctx, r.ID); err != nil {
		t.Fatalf("MarkCancelled (should be no-op): %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status: got %q, want %q (MarkCancelled must not overwrite terminal)", got.Status, "failed")
	}
}

// TestMarkUnroutedIsNoOpFromNonQueued drives a request to submitted, then calls
// MarkUnrouted, and asserts the row remains submitted.
func TestMarkUnroutedIsNoOpFromNonQueued(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-unrouted-noop-nonqueued")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkSubmitted(ctx, r.ID, 77); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	// Now attempt MarkUnrouted on a submitted (non-queued) row.
	trace := []byte(`{"evaluated":1,"matched":0}`)
	if err := s.MarkUnrouted(ctx, r.ID, trace, "too late"); err != nil {
		t.Fatalf("MarkUnrouted (should be no-op): %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "submitted" {
		t.Errorf("Status: got %q, want %q (MarkUnrouted must not overwrite non-queued)", got.Status, "submitted")
	}
}

// TestMarkRetryingFromFailedTransitionsToQueued verifies that MarkRetrying
// transitions a 'failed' row back to 'queued', clearing error, completed_at,
// submitted_at, external_id, and match_trace.
func TestMarkRetryingFromFailedTransitionsToQueued(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-retrying-from-failed")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkSubmitted(ctx, r.ID, 42); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}
	if err := s.MarkFailed(ctx, r.ID, "network error"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	// Confirm we're in failed state before retry.
	before, _ := s.GetRequest(ctx, r.ID)
	if before.Status != "failed" {
		t.Fatalf("pre-condition: status=%q, want failed", before.Status)
	}

	if err := s.MarkRetrying(ctx, r.ID); err != nil {
		t.Fatalf("MarkRetrying: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "queued" {
		t.Errorf("Status: got %q, want queued", got.Status)
	}
	if got.Error != "" {
		t.Errorf("Error: got %q, want empty", got.Error)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt: got %v, want nil", got.CompletedAt)
	}
	if got.SubmittedAt != nil {
		t.Errorf("SubmittedAt: got %v, want nil", got.SubmittedAt)
	}
	if got.ExternalID != nil {
		t.Errorf("ExternalID: got %v, want nil", got.ExternalID)
	}
	if len(got.MatchTrace) != 0 {
		t.Errorf("MatchTrace: got %s, want nil/empty", got.MatchTrace)
	}
}

// TestMarkRetryingFromOtherStateIsNoOp verifies that MarkRetrying on a
// non-failed row is a silent no-op (row status unchanged).
func TestMarkRetryingFromOtherStateIsNoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-retrying-noop")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkSubmitted(ctx, r.ID, 7); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	// Row is now 'submitted'; MarkRetrying should be a no-op.
	if err := s.MarkRetrying(ctx, r.ID); err != nil {
		t.Fatalf("MarkRetrying on submitted: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "submitted" {
		t.Errorf("Status: got %q, want submitted (must be no-op)", got.Status)
	}
}

// TestMarkReRoutingFromUnroutedTransitionsToQueued verifies that MarkReRouting
// transitions an 'unrouted' row back to 'queued', clearing error, completed_at,
// and match_trace.
func TestMarkReRoutingFromUnroutedTransitionsToQueued(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-rerouting-from-unrouted")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}

	trace := []byte(`{"candidates":[]}`)
	if err := s.MarkUnrouted(ctx, r.ID, trace, "no match"); err != nil {
		t.Fatalf("MarkUnrouted: %v", err)
	}

	before, _ := s.GetRequest(ctx, r.ID)
	if before.Status != "unrouted" {
		t.Fatalf("pre-condition: status=%q, want unrouted", before.Status)
	}

	if err := s.MarkReRouting(ctx, r.ID); err != nil {
		t.Fatalf("MarkReRouting: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "queued" {
		t.Errorf("Status: got %q, want queued", got.Status)
	}
	if got.Error != "" {
		t.Errorf("Error: got %q, want empty", got.Error)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt: got %v, want nil", got.CompletedAt)
	}
	if len(got.MatchTrace) != 0 {
		t.Errorf("MatchTrace: got %s, want nil/empty", got.MatchTrace)
	}
}

// TestMarkReRoutingFromOtherStateIsNoOp verifies that MarkReRouting on a
// non-unrouted row is a silent no-op (row status unchanged).
func TestMarkReRoutingFromOtherStateIsNoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := sampleRequest("req-mark-rerouting-noop")
	if err := s.UpsertRequestQueued(ctx, r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := s.MarkFailed(ctx, r.ID, "pre-existing failure"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	// Row is now 'failed'; MarkReRouting should be a no-op.
	if err := s.MarkReRouting(ctx, r.ID); err != nil {
		t.Fatalf("MarkReRouting on failed: %v", err)
	}

	got, err := s.GetRequest(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status: got %q, want failed (must be no-op)", got.Status)
	}
}

// jsonEqual normalises two JSON byte slices by unmarshalling them into
// interface{} and re-marshalling before comparing, so key ordering differences
// introduced by Postgres JSONB storage do not cause false failures.
// If either slice is nil/empty, they must both be nil/empty to be equal.
func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	var va, vb interface{}
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("jsonEqual: unmarshal a: %v", err)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatalf("jsonEqual: unmarshal b: %v", err)
	}
	na, _ := json.Marshal(va)
	nb, _ := json.Marshal(vb)
	return string(na) == string(nb)
}
