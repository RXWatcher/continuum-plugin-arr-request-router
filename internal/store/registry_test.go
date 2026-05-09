package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

// sampleArr returns a minimal RegisteredArr suitable for insertion in tests.
// Callers may override fields after calling this.
func sampleArr(name, kind string) *store.RegisteredArr {
	qp := 1
	return &store.RegisteredArr{
		Name:             name,
		Kind:             kind,
		URL:              "http://localhost:7878",
		APIKey:           "testkey",
		RootFolderPath:   "/movies",
		QualityProfileID: &qp,
		Priority:         100,
		Enabled:          true,
		RulesJSON:        []byte(`{"match":"all","groups":[]}`),
	}
}

// TestCreateThenGet inserts a row and reads it back, verifying all fields
// including a set QualityProfileID and a nil LanguageProfileID.
func TestCreateThenGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := sampleArr("My Radarr", "radarr")
	// LanguageProfileID intentionally left nil.

	id, err := s.CreateArr(ctx, a)
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	got, err := s.GetArr(ctx, id)
	if err != nil {
		t.Fatalf("GetArr: %v", err)
	}
	if got == nil {
		t.Fatal("GetArr returned nil for existing row")
	}

	if got.ID != id {
		t.Errorf("ID: got %d, want %d", got.ID, id)
	}
	if got.Name != a.Name {
		t.Errorf("Name: got %q, want %q", got.Name, a.Name)
	}
	if got.Kind != a.Kind {
		t.Errorf("Kind: got %q, want %q", got.Kind, a.Kind)
	}
	if got.URL != a.URL {
		t.Errorf("URL: got %q, want %q", got.URL, a.URL)
	}
	if got.APIKey != a.APIKey {
		t.Errorf("APIKey: got %q, want %q", got.APIKey, a.APIKey)
	}
	if got.RootFolderPath != a.RootFolderPath {
		t.Errorf("RootFolderPath: got %q, want %q", got.RootFolderPath, a.RootFolderPath)
	}
	if got.QualityProfileID == nil {
		t.Error("QualityProfileID: got nil, want non-nil")
	} else if *got.QualityProfileID != *a.QualityProfileID {
		t.Errorf("QualityProfileID: got %d, want %d", *got.QualityProfileID, *a.QualityProfileID)
	}
	if got.LanguageProfileID != nil {
		t.Errorf("LanguageProfileID: got %v, want nil", got.LanguageProfileID)
	}
	if got.Priority != a.Priority {
		t.Errorf("Priority: got %d, want %d", got.Priority, a.Priority)
	}
	if got.Enabled != a.Enabled {
		t.Errorf("Enabled: got %v, want %v", got.Enabled, a.Enabled)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

// TestListOrdersByKindThenPriorityThenID inserts several rows and confirms
// ListArrs returns them sorted by (kind ASC, priority ASC, id ASC).
func TestListOrdersByKindThenPriorityThenID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert in non-sorted order to prove the ORDER BY works.
	insertArr := func(name, kind string, priority int) int64 {
		a := sampleArr(name, kind)
		a.Priority = priority
		id, err := s.CreateArr(ctx, a)
		if err != nil {
			t.Fatalf("CreateArr(%s): %v", name, err)
		}
		return id
	}

	// radarr rows
	id3 := insertArr("Radarr-Low", "radarr", 200)
	id1 := insertArr("Radarr-High", "radarr", 50)
	id2 := insertArr("Radarr-Mid", "radarr", 100)

	// sonarr rows
	id5 := insertArr("Sonarr-B", "sonarr", 100)
	id4 := insertArr("Sonarr-A", "sonarr", 50)

	rows, err := s.ListArrs(ctx)
	if err != nil {
		t.Fatalf("ListArrs: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}

	// Expected order: radarr@50, radarr@100, radarr@200, sonarr@50, sonarr@100
	expected := []int64{id1, id2, id3, id4, id5}
	for i, row := range rows {
		if row.ID != expected[i] {
			t.Errorf("row[%d]: got id=%d, want id=%d", i, row.ID, expected[i])
		}
	}
}

// TestListEnabledArrsByKindFiltersAndOrders confirms that disabled rows and
// rows of the wrong kind are excluded, and that results are ordered by
// (priority ASC, id ASC).
func TestListEnabledArrsByKindFiltersAndOrders(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Helper to create and insert.
	insert := func(name, kind string, priority int, enabled bool) int64 {
		a := sampleArr(name, kind)
		a.Priority = priority
		a.Enabled = enabled
		id, err := s.CreateArr(ctx, a)
		if err != nil {
			t.Fatalf("CreateArr(%s): %v", name, err)
		}
		return id
	}

	// Should appear in results (radarr, enabled).
	idA := insert("Radarr-1", "radarr", 100, true)
	idB := insert("Radarr-2", "radarr", 50, true)

	// Should be excluded: disabled radarr.
	_ = insert("Radarr-Disabled", "radarr", 1, false)

	// Should be excluded: sonarr (wrong kind).
	_ = insert("Sonarr-1", "sonarr", 1, true)

	rows, err := s.ListEnabledArrsByKind(ctx, "radarr")
	if err != nil {
		t.Fatalf("ListEnabledArrsByKind: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d (rows: %+v)", len(rows), rows)
	}

	// idB has priority 50, idA has priority 100 — idB should come first.
	if rows[0].ID != idB {
		t.Errorf("rows[0]: got id=%d, want id=%d (lower priority first)", rows[0].ID, idB)
	}
	if rows[1].ID != idA {
		t.Errorf("rows[1]: got id=%d, want id=%d", rows[1].ID, idA)
	}

	// Confirm all returned rows are enabled radarr.
	for _, row := range rows {
		if row.Kind != "radarr" {
			t.Errorf("row %d: kind=%q, want radarr", row.ID, row.Kind)
		}
		if !row.Enabled {
			t.Errorf("row %d: enabled=false, want true", row.ID)
		}
	}
}

// TestUpdateChangesFieldsAndAdvancesUpdatedAt reads an existing row, updates
// it, reads it again, and confirms the changed field changed and updated_at
// advanced (via now() in the UPDATE SQL).
func TestUpdateChangesFieldsAndAdvancesUpdatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := sampleArr("Original", "radarr")
	id, err := s.CreateArr(ctx, a)
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	before, err := s.GetArr(ctx, id)
	if err != nil {
		t.Fatalf("GetArr (before): %v", err)
	}

	before.Name = "Updated"
	before.Priority = 42

	if err := s.UpdateArr(ctx, before); err != nil {
		t.Fatalf("UpdateArr: %v", err)
	}

	after, err := s.GetArr(ctx, id)
	if err != nil {
		t.Fatalf("GetArr (after): %v", err)
	}
	if after == nil {
		t.Fatal("GetArr returned nil after update")
	}

	if after.Name != "Updated" {
		t.Errorf("Name: got %q, want %q", after.Name, "Updated")
	}
	if after.Priority != 42 {
		t.Errorf("Priority: got %d, want %d", after.Priority, 42)
	}

	// updated_at must not regress: the UPDATE SQL sets updated_at = now().
	if after.UpdatedAt.Before(before.UpdatedAt) {
		t.Errorf("UpdatedAt did not advance: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}

// TestDeleteRemovesRow inserts a row, deletes it, and confirms that a
// subsequent GetArr returns (nil, nil) — not an error.
func TestDeleteRemovesRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := sampleArr("ToDelete", "sonarr")
	id, err := s.CreateArr(ctx, a)
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	if err := s.DeleteArr(ctx, id); err != nil {
		t.Fatalf("DeleteArr: %v", err)
	}

	got, err := s.GetArr(ctx, id)
	if err != nil {
		t.Errorf("GetArr after delete: expected nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("GetArr after delete: expected nil row, got %+v", got)
	}
}

// TestRulesJSONRoundTripsAsJSONB inserts a row with a non-trivial rules_json
// blob, reads it back, and confirms the bytes are valid JSON and equal to what
// was inserted.
func TestRulesJSONRoundTripsAsJSONB(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rulesBlob := []byte(`{"match":"any","groups":[{"field":"genre","op":"contains","value":"Action"}]}`)

	a := sampleArr("RulesTest", "radarr")
	a.RulesJSON = rulesBlob

	id, err := s.CreateArr(ctx, a)
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	got, err := s.GetArr(ctx, id)
	if err != nil {
		t.Fatalf("GetArr: %v", err)
	}
	if got == nil {
		t.Fatal("GetArr returned nil")
	}

	// Must be valid JSON.
	var v interface{}
	if err := json.Unmarshal(got.RulesJSON, &v); err != nil {
		t.Fatalf("RulesJSON is not valid JSON: %v — got: %s", err, got.RulesJSON)
	}

	// Normalize both via round-trip to compare structure, not whitespace.
	var want, have interface{}
	if err := json.Unmarshal(rulesBlob, &want); err != nil {
		t.Fatalf("original blob is invalid JSON: %v", err)
	}
	if err := json.Unmarshal(got.RulesJSON, &have); err != nil {
		t.Fatalf("returned blob is invalid JSON: %v", err)
	}

	wantBytes, _ := json.Marshal(want)
	haveBytes, _ := json.Marshal(have)
	if string(wantBytes) != string(haveBytes) {
		t.Errorf("RulesJSON mismatch:\n  want: %s\n  have: %s", wantBytes, haveBytes)
	}
}

// TestUpdateArrMissingIDIsNoOp confirms that UpdateArr is a silent no-op when
// the supplied ID does not exist — no error, and no phantom row is inserted.
// The HTTP handler (Task 9.2) is responsible for issuing a 404 by checking
// GetArr before calling UpdateArr.
func TestUpdateArrMissingIDIsNoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := &store.RegisteredArr{
		ID:             999999,
		Name:           "ghost",
		Kind:           "radarr",
		URL:            "http://nope",
		APIKey:         "k",
		RootFolderPath: "/movies",
		Priority:       100,
		Enabled:        true,
		RulesJSON:      []byte(`{"match":"all","groups":[]}`),
	}
	if err := s.UpdateArr(ctx, a); err != nil {
		t.Fatalf("UpdateArr on missing id should be a no-op, got err: %v", err)
	}
	// Confirm nothing was inserted as a side-effect.
	got, err := s.GetArr(ctx, 999999)
	if err != nil {
		t.Fatalf("GetArr: %v", err)
	}
	if got != nil {
		t.Fatalf("phantom row appeared: %+v", got)
	}
}
