package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/server"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

const testSecretKey = "test-secret-key-for-handlers-tests"

// newTestServer builds a *server.Server backed by a real Postgres store
// (schema-isolated via testutil_test.go) with the test secret key.
func newTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st := newTestStore(t)
	deps := &server.Deps{
		Store:     st,
		SecretKey: testSecretKey,
	}
	srv := server.New(deps)
	return srv.Handler(), st
}

// adminReq builds an http.Request with the admin identity headers set.
func adminReq(method, target string, body []byte) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	return r
}

// nonAdminReq builds an http.Request with a non-admin role.
func nonAdminReq(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set(auth.HeaderUserID, "user-2")
	r.Header.Set(auth.HeaderRole, "user")
	return r
}

// do executes a request against handler and returns the ResponseRecorder.
func do(handler http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// mustJSON marshals v to JSON bytes, fataling on error.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return b
}

// ---- Admin Guard ----

func TestRequireAdminBlocksNonAdmin(t *testing.T) {
	handler, _ := newTestServer(t)
	w := do(handler, nonAdminReq("GET", "/api/admin/registry/"))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAdminAllowsAdmin(t *testing.T) {
	handler, _ := newTestServer(t)
	w := do(handler, adminReq("GET", "/api/admin/registry/", nil))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ---- List ----

func TestRegistryListReturnsRowsOrderedByKindPriority(t *testing.T) {
	handler, st := newTestServer(t)

	rows := []struct {
		kind     string
		priority int
		name     string
	}{
		{"sonarr", 2, "sonarr-b"},
		{"radarr", 1, "radarr-a"},
		{"radarr", 2, "radarr-b"},
	}
	for _, row := range rows {
		sealed, _ := crypto.Seal(testSecretKey, "key")
		_, err := st.CreateArr(t.Context(), &store.RegisteredArr{
			Name: row.name, Kind: row.kind, URL: "http://x", APIKey: sealed,
			Priority: row.priority, Enabled: true, RulesJSON: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("CreateArr: %v", err)
		}
	}

	w := do(handler, adminReq("GET", "/api/admin/registry/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var out []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(out))
	}

	// Order: (kind ASC, priority ASC, id ASC)
	// radarr-a (1), radarr-b (2), sonarr-b (2)
	wantNames := []string{"radarr-a", "radarr-b", "sonarr-b"}
	for i, row := range out {
		if row["name"] != wantNames[i] {
			t.Errorf("row %d: want name=%s got %v", i, wantNames[i], row["name"])
		}
	}
}

// ---- Get ----

func TestRegistryGetExistingReturnsDTO(t *testing.T) {
	handler, st := newTestServer(t)

	sealed, _ := crypto.Seal(testSecretKey, "my-api-key")
	id, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name: "test-arr", Kind: "radarr", URL: "http://radarr", APIKey: sealed,
		Priority: 1, Enabled: true, RulesJSON: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	w := do(handler, adminReq("GET", fmt.Sprintf("/api/admin/registry/%d", id), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var dto map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// has_api_key should be true
	if dto["has_api_key"] != true {
		t.Errorf("expected has_api_key=true, got %v", dto["has_api_key"])
	}
	// api_key must NOT appear in response
	if _, ok := dto["api_key"]; ok {
		t.Errorf("api_key must not appear in GET response")
	}
	if dto["name"] != "test-arr" {
		t.Errorf("expected name=test-arr, got %v", dto["name"])
	}
}

func TestRegistryGetMissingReturns404(t *testing.T) {
	handler, _ := newTestServer(t)
	w := do(handler, adminReq("GET", "/api/admin/registry/999999", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---- Create ----

func TestRegistryCreateValidatesAndEncrypts(t *testing.T) {
	handler, st := newTestServer(t)

	body := mustJSON(t, map[string]any{
		"name":     "my-radarr",
		"kind":     "radarr",
		"url":      "http://radarr:7878",
		"api_key":  "plaintext-key",
		"priority": 1,
		"enabled":  true,
		"rules":    json.RawMessage(`{}`),
	})

	w := do(handler, adminReq("POST", "/api/admin/registry/", body))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rawID, ok := resp["id"]
	if !ok {
		t.Fatal("response missing 'id' field")
	}
	id := int64(rawID.(float64))

	// Read back from store and verify
	a, err := st.GetArr(t.Context(), id)
	if err != nil || a == nil {
		t.Fatalf("GetArr: %v, a=%v", err, a)
	}
	if a.Kind != "radarr" {
		t.Errorf("kind mismatch: got %s", a.Kind)
	}
	if a.Name != "my-radarr" {
		t.Errorf("name mismatch: got %s", a.Name)
	}
	if a.URL != "http://radarr:7878" {
		t.Errorf("url mismatch: got %s", a.URL)
	}
	// API key must not be stored as plaintext
	if a.APIKey == "plaintext-key" {
		t.Error("api_key stored as plaintext — must be sealed")
	}
	// Must be recoverable with the secret key
	plaintext, err := crypto.Open(testSecretKey, a.APIKey)
	if err != nil {
		t.Fatalf("crypto.Open: %v", err)
	}
	if plaintext != "plaintext-key" {
		t.Errorf("decrypted key mismatch: got %s", plaintext)
	}
	// rules_json should be valid JSON
	if !json.Valid(a.RulesJSON) {
		t.Error("rules_json is not valid JSON")
	}
}

func TestRegistryCreateRejectsBadKind(t *testing.T) {
	handler, _ := newTestServer(t)
	body := mustJSON(t, map[string]any{
		"name": "plex-server", "kind": "plex", "url": "http://plex",
		"api_key": "key", "priority": 1, "enabled": true, "rules": json.RawMessage(`{}`),
	})
	w := do(handler, adminReq("POST", "/api/admin/registry/", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegistryCreateRejectsInvalidRules(t *testing.T) {
	handler, _ := newTestServer(t)
	// rules with unknown field → ValidateRules returns error
	body := mustJSON(t, map[string]any{
		"name": "arr", "kind": "radarr", "url": "http://radarr",
		"api_key": "key", "priority": 1, "enabled": true,
		"rules": json.RawMessage(`{"match":"all","groups":[{"match":"all","rules":[{"field":"not_a_real_field","op":"eq","value":"x"}]}]}`),
	})
	w := do(handler, adminReq("POST", "/api/admin/registry/", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRegistryCreateRequiresAPIKey(t *testing.T) {
	handler, _ := newTestServer(t)
	body := mustJSON(t, map[string]any{
		"name": "arr", "kind": "radarr", "url": "http://radarr",
		"priority": 1, "enabled": true, "rules": json.RawMessage(`{}`),
		// api_key intentionally omitted
	})
	w := do(handler, adminReq("POST", "/api/admin/registry/", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- Update ----

func createTestArr(t *testing.T, st *store.Store, name, kind, apiKey string) int64 {
	t.Helper()
	sealed, err := crypto.Seal(testSecretKey, apiKey)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	id, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name: name, Kind: kind, URL: "http://arr:7878", APIKey: sealed,
		Priority: 1, Enabled: true, RulesJSON: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}
	return id
}

func TestRegistryUpdatePartialFieldsAndAPIKeyNotRotated(t *testing.T) {
	handler, st := newTestServer(t)
	id := createTestArr(t, st, "original-name", "radarr", "original-key")

	body := mustJSON(t, map[string]any{"name": "updated-name"})
	w := do(handler, adminReq("PATCH", fmt.Sprintf("/api/admin/registry/%d", id), body))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}

	a, _ := st.GetArr(t.Context(), id)
	if a.Name != "updated-name" {
		t.Errorf("name not updated: got %s", a.Name)
	}
	// API key must still decrypt to original plaintext
	plaintext, err := crypto.Open(testSecretKey, a.APIKey)
	if err != nil {
		t.Fatalf("crypto.Open: %v", err)
	}
	if plaintext != "original-key" {
		t.Errorf("api_key rotated unexpectedly: got %s", plaintext)
	}
}

func TestRegistryUpdateRotatesAPIKeyOnNonEmptyKey(t *testing.T) {
	handler, st := newTestServer(t)
	id := createTestArr(t, st, "arr", "radarr", "old-key")

	body := mustJSON(t, map[string]any{"api_key": "new-secret"})
	w := do(handler, adminReq("PATCH", fmt.Sprintf("/api/admin/registry/%d", id), body))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}

	a, _ := st.GetArr(t.Context(), id)
	plaintext, err := crypto.Open(testSecretKey, a.APIKey)
	if err != nil {
		t.Fatalf("crypto.Open: %v", err)
	}
	if plaintext != "new-secret" {
		t.Errorf("expected rotated key 'new-secret', got %s", plaintext)
	}
}

func TestRegistryUpdateEmptyAPIKeyDoesNotRotate(t *testing.T) {
	handler, st := newTestServer(t)
	id := createTestArr(t, st, "arr", "radarr", "original-key")

	body := mustJSON(t, map[string]any{"api_key": ""})
	w := do(handler, adminReq("PATCH", fmt.Sprintf("/api/admin/registry/%d", id), body))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}

	a, _ := st.GetArr(t.Context(), id)
	plaintext, err := crypto.Open(testSecretKey, a.APIKey)
	if err != nil {
		t.Fatalf("crypto.Open: %v", err)
	}
	if plaintext != "original-key" {
		t.Errorf("api_key rotated on empty string: got %s", plaintext)
	}
}

func TestRegistryUpdateMissingIDReturns404(t *testing.T) {
	handler, _ := newTestServer(t)
	body := mustJSON(t, map[string]any{"name": "new"})
	w := do(handler, adminReq("PATCH", "/api/admin/registry/999999", body))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---- Delete ----

func TestRegistryDeleteHardDeletesButPreservesRequests(t *testing.T) {
	handler, st := newTestServer(t)
	id := createTestArr(t, st, "arr-to-delete", "radarr", "key")

	// Insert a request row with routed_arr_id pointing at this arr.
	req := &store.Request{
		ID:        "req-delete-test",
		TMDBID:    1234,
		MediaType: "movie",
		Title:     "Test Movie",
		Status:    "queued",
	}
	if err := st.UpsertRequestQueued(t.Context(), req); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	// Set routed_arr_id via SetRoutedArr
	if err := st.SetRoutedArr(t.Context(), req.ID, id, []byte("{}")); err != nil {
		t.Fatalf("SetRoutedArr: %v", err)
	}

	// Delete the arr.
	w := do(handler, adminReq("DELETE", fmt.Sprintf("/api/admin/registry/%d", id), nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}

	// Arr row must be gone.
	a, err := st.GetArr(t.Context(), id)
	if err != nil {
		t.Fatalf("GetArr error: %v", err)
	}
	if a != nil {
		t.Error("arr row not deleted")
	}

	// Request row must still exist with routed_arr_id = NULL (ON DELETE SET NULL).
	r, err := st.GetRequest(t.Context(), req.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if r == nil {
		t.Fatal("request row was deleted — should be preserved")
	}
	if r.RoutedArrID != nil {
		t.Errorf("routed_arr_id should be NULL after arr delete, got %v", *r.RoutedArrID)
	}
}

// ---- Test-connection ----

// fakeArrServer returns an httptest.Server that serves /api/v3/system/status.
// capturedKey will hold the X-Api-Key header value from the last request.
type fakeArrServer struct {
	server      *httptest.Server
	capturedKey string
	statusCode  int
}

func newFakeArrServer(statusCode int) *fakeArrServer {
	f := &fakeArrServer{statusCode: statusCode}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.capturedKey = r.Header.Get("X-Api-Key")
		if f.statusCode != http.StatusOK {
			http.Error(w, "upstream error", f.statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"4.0.0","instanceName":"Radarr","appName":"Radarr","branch":"main"}`))
	}))
	return f
}

func TestTestConnectionWithStoredKeyHits200(t *testing.T) {
	handler, st := newTestServer(t)
	fake := newFakeArrServer(http.StatusOK)
	defer fake.server.Close()

	sealed, _ := crypto.Seal(testSecretKey, "stored-api-key")
	id, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name: "arr", Kind: "radarr", URL: fake.server.URL, APIKey: sealed,
		Priority: 1, Enabled: true, RulesJSON: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	// POST with empty body — uses stored key.
	w := do(handler, adminReq("POST", fmt.Sprintf("/api/admin/registry/%d/test-connection", id), []byte(`{}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	// Verify the fake server received the original plaintext key.
	if fake.capturedKey != "stored-api-key" {
		t.Errorf("fake server saw key %q, want 'stored-api-key'", fake.capturedKey)
	}

	// Response should include version field.
	if !strings.Contains(w.Body.String(), "version") {
		t.Errorf("response missing 'version': %s", w.Body.String())
	}
}

func TestTestConnectionWithBodyKeyOverridesStored(t *testing.T) {
	handler, st := newTestServer(t)
	fake := newFakeArrServer(http.StatusOK)
	defer fake.server.Close()

	sealed, _ := crypto.Seal(testSecretKey, "stored-key")
	id, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name: "arr", Kind: "radarr", URL: fake.server.URL, APIKey: sealed,
		Priority: 1, Enabled: true, RulesJSON: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	body := mustJSON(t, map[string]any{"api_key": "override-key"})
	w := do(handler, adminReq("POST", fmt.Sprintf("/api/admin/registry/%d/test-connection", id), body))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if fake.capturedKey != "override-key" {
		t.Errorf("fake server saw key %q, want 'override-key'", fake.capturedKey)
	}
}

func TestTestConnection502OnUpstream500(t *testing.T) {
	handler, st := newTestServer(t)
	fake := newFakeArrServer(http.StatusInternalServerError)
	defer fake.server.Close()

	sealed, _ := crypto.Seal(testSecretKey, "key")
	id, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name: "arr", Kind: "radarr", URL: fake.server.URL, APIKey: sealed,
		Priority: 1, Enabled: true, RulesJSON: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	w := do(handler, adminReq("POST", fmt.Sprintf("/api/admin/registry/%d/test-connection", id), []byte(`{}`)))
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

func TestTestConnectionMissingArrReturns404(t *testing.T) {
	handler, _ := newTestServer(t)
	w := do(handler, adminReq("POST", "/api/admin/registry/999999/test-connection", []byte(`{}`)))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
