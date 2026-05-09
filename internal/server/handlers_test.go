package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/arr"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/consumer"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
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

// ---------------------------------------------------------------------------
// Helpers for route-test + requests tests
// ---------------------------------------------------------------------------

// testEnricher satisfies routing.Enricher with no-op implementations.
// routing.Enricher methods accept context.Context.
type testEnricher struct{}

func (testEnricher) Primary(_ context.Context, _ string, _ int) (*routing.TMDBPrimary, error) {
	return nil, nil
}
func (testEnricher) Keywords(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (testEnricher) ContentRating(_ context.Context, _ string, _ int) (string, error) {
	return "", nil
}

// newTestServerFull builds a server with Enricher + SubmitHandler wired.
// The SubmitHandler uses fakeArr as the radarr backend.
func newTestServerFull(t *testing.T, fakeArrURL string) (http.Handler, *store.Store) {
	t.Helper()
	st := newTestStore(t)
	sh := newSubmitHandler(t, st, fakeArrURL)
	deps := &server.Deps{
		Store:     st,
		SecretKey: testSecretKey,
		Enricher:  testEnricher{},
		Submit:    sh,
	}
	return server.New(deps).Handler(), st
}

// newTestServerRouteTest builds a server with Enricher only (no Submit needed).
func newTestServerRouteTest(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st := newTestStore(t)
	deps := &server.Deps{
		Store:     st,
		SecretKey: testSecretKey,
		Enricher:  testEnricher{},
	}
	return server.New(deps).Handler(), st
}

// newSubmitHandler creates a real consumer.SubmitHandler wired to the given store.
// Each arr's URL is read from the store at dispatch time; fakeArrURL is
// embedded in the arr row inserted by the caller.
func newSubmitHandler(t *testing.T, st *store.Store, fakeArrURL string) *consumer.SubmitHandler {
	t.Helper()
	_ = fakeArrURL // URL is embedded in the registered_arr row, not here
	return &consumer.SubmitHandler{
		Store:     st,
		Enricher:  testEnricher{},
		Radarr:    arr.NewRadarr,
		Sonarr:    arr.NewSonarr,
		Events:    event.New(nil, hclog.NewNullLogger()),
		SecretKey: testSecretKey,
		Log:       hclog.NewNullLogger(),
	}
}

// sealTestKey encrypts key using testSecretKey.
func sealTestKey(t *testing.T, key string) string {
	t.Helper()
	s, err := crypto.Seal(testSecretKey, key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return s
}

// insertEnabledRadarr inserts an enabled radarr at the given URL with catch-all rules.
func insertEnabledRadarr(t *testing.T, st *store.Store, name, url string) int64 {
	t.Helper()
	id, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name:      name,
		Kind:      "radarr",
		URL:       url,
		APIKey:    sealTestKey(t, "test-key"),
		Enabled:   true,
		Priority:  1,
		RulesJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}
	return id
}

// fakeRadarrServer returns an httptest.Server that responds to the minimal
// Radarr API surface used by SubmitHandler. postCount is incremented per POST.
func fakeRadarrServer(t *testing.T, movieID int, postCount *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/rootfolder", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "path": "/movies"}})
	})
	mux.HandleFunc("/api/v3/qualityprofile", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "HD"}})
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
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": movieID, "title": "Test"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// Route-test endpoint tests (Task 9.4)
// ---------------------------------------------------------------------------

func TestRouteTestReturnsChosenAndTrace(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	// Insert an enabled radarr with catch-all rules.
	arrID := insertEnabledRadarr(t, st, "radarr-1", "http://radarr:7878")

	body := mustJSON(t, map[string]any{
		"tmdbId":    603,
		"mediaType": "movie",
		"title":     "The Matrix",
		"year":      1999,
	})
	w := do(handler, adminReq("POST", "/api/admin/route-test", body))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// chosen must be the arr ID.
	chosen, ok := resp["chosen"]
	if !ok {
		t.Fatal("response missing 'chosen' field")
	}
	chosenF, ok := chosen.(float64)
	if !ok || int64(chosenF) != arrID {
		t.Errorf("chosen=%v, want %d", chosen, arrID)
	}
	// trace must be present.
	if _, ok := resp["trace"]; !ok {
		t.Error("response missing 'trace' field")
	}
}

func TestRouteTestReturnsNullChosenWhenNoMatch(t *testing.T) {
	handler, _ := newTestServerRouteTest(t)
	// Empty registry → no candidates → chosen == null.
	body := mustJSON(t, map[string]any{
		"tmdbId":    603,
		"mediaType": "movie",
	})
	w := do(handler, adminReq("POST", "/api/admin/route-test", body))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["chosen"] != nil {
		t.Errorf("chosen=%v, want null", resp["chosen"])
	}
	trace, ok := resp["trace"].(map[string]any)
	if !ok {
		t.Fatal("trace is not an object")
	}
	cands, ok := trace["candidates"]
	if !ok {
		t.Fatal("trace missing candidates")
	}
	if cands != nil {
		// candidates may be nil/null or an empty array — both acceptable.
		arr, ok := cands.([]any)
		if ok && len(arr) != 0 {
			t.Errorf("candidates=%v, want empty", cands)
		}
	}
}

func TestRouteTestRequiresValidMediaType(t *testing.T) {
	handler, _ := newTestServerRouteTest(t)
	body := mustJSON(t, map[string]any{
		"tmdbId":    603,
		"mediaType": "podcast",
	})
	w := do(handler, adminReq("POST", "/api/admin/route-test", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRouteTestRequiresTMDBID(t *testing.T) {
	handler, _ := newTestServerRouteTest(t)
	body := mustJSON(t, map[string]any{
		"mediaType": "movie",
	})
	w := do(handler, adminReq("POST", "/api/admin/route-test", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRouteTestDoesNotWriteToDB(t *testing.T) {
	handler, st := newTestServerRouteTest(t)
	insertEnabledRadarr(t, st, "radarr-ro", "http://radarr:7878")

	body := mustJSON(t, map[string]any{
		"tmdbId":    999,
		"mediaType": "movie",
		"title":     "Read-only test",
	})
	w := do(handler, adminReq("POST", "/api/admin/route-test", body))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	// No rows should exist in the request table.
	rows, total, err := st.ListRequestsForAdmin(t.Context(), "", 100, 0)
	if err != nil {
		t.Fatalf("ListRequestsForAdmin: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Errorf("route-test wrote %d row(s) to DB, expected 0", total)
	}
}

// ---------------------------------------------------------------------------
// Requests list/detail tests (Task 9.5)
// ---------------------------------------------------------------------------

func TestRequestsListPaginatesAndFilters(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	// Insert 5 requests: 3 queued + 2 failed.
	for i := 0; i < 3; i++ {
		r := &store.Request{
			ID:        fmt.Sprintf("req-list-queued-%d", i),
			TMDBID:    1000 + i,
			MediaType: "movie",
			Title:     "Movie",
			Status:    "queued",
		}
		if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
			t.Fatalf("UpsertRequestQueued: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("req-list-failed-%d", i)
		r := &store.Request{
			ID:        id,
			TMDBID:    2000 + i,
			MediaType: "movie",
			Title:     "Movie",
			Status:    "queued",
		}
		if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
			t.Fatalf("UpsertRequestQueued: %v", err)
		}
		if err := st.MarkFailed(t.Context(), id, "err"); err != nil {
			t.Fatalf("MarkFailed: %v", err)
		}
	}

	// Filter to failed only.
	w := do(handler, adminReq("GET", "/api/admin/requests/?status=failed&limit=10", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("total=%d, want 2", total)
	}
	rowsRaw := resp["rows"].([]any)
	if len(rowsRaw) != 2 {
		t.Errorf("rows count=%d, want 2", len(rowsRaw))
	}
	for _, row := range rowsRaw {
		m := row.(map[string]any)
		if m["status"] != "failed" {
			t.Errorf("unexpected status %q in filtered results", m["status"])
		}
	}
}

func TestRequestsListIncludesRoutedArrName(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	// Insert an arr.
	arrID, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name:      "my-radarr",
		Kind:      "radarr",
		URL:       "http://radarr",
		APIKey:    sealTestKey(t, "k"),
		Enabled:   true,
		Priority:  1,
		RulesJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	r := &store.Request{
		ID:        "req-arrname-test",
		TMDBID:    1001,
		MediaType: "movie",
		Title:     "Movie",
		Status:    "queued",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.SetRoutedArr(t.Context(), r.ID, arrID, []byte(`{}`)); err != nil {
		t.Fatalf("SetRoutedArr: %v", err)
	}

	w := do(handler, adminReq("GET", "/api/admin/requests/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rows := resp["rows"].([]any)
	if len(rows) == 0 {
		t.Fatal("expected at least one row")
	}
	row := rows[0].(map[string]any)
	if row["routed_arr_name"] != "my-radarr" {
		t.Errorf("routed_arr_name=%v, want my-radarr", row["routed_arr_name"])
	}
}

func TestRequestsGetReturnsTraceAndDetails(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	r := &store.Request{
		ID:        "req-get-trace",
		TMDBID:    5050,
		MediaType: "movie",
		Title:     "Traced Movie",
		Status:    "queued",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	trace := []byte(`{"candidates":[],"chosen_arr_id":null}`)
	if err := st.MarkUnrouted(t.Context(), r.ID, trace, "no match"); err != nil {
		t.Fatalf("MarkUnrouted: %v", err)
	}

	w := do(handler, adminReq("GET", "/api/admin/requests/req-get-trace", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var dto map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dto["id"] != "req-get-trace" {
		t.Errorf("id=%v, want req-get-trace", dto["id"])
	}
	if dto["match_trace"] == nil {
		t.Error("match_trace is null, expected non-empty")
	}
}

func TestRequestsGetMissingReturns404(t *testing.T) {
	handler, _ := newTestServerRouteTest(t)
	w := do(handler, adminReq("GET", "/api/admin/requests/does-not-exist", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Retry endpoint tests (Task 9.5)
// ---------------------------------------------------------------------------

func TestRetryFailedRowResubmits(t *testing.T) {
	var postCount atomic.Int32
	fakeArr := fakeRadarrServer(t, 77, &postCount)

	handler, st := newTestServerFull(t, fakeArr.URL)

	// Insert a radarr at the fake URL.
	arrID := insertEnabledRadarr(t, st, "radarr-retry", fakeArr.URL)
	_ = arrID

	// Insert + fail a request.
	r := &store.Request{
		ID:        "req-retry-test",
		TMDBID:    603,
		MediaType: "movie",
		Title:     "Matrix",
		Year:      1999,
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.MarkFailed(t.Context(), r.ID, "network error"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-retry-test/retry", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
	}

	// The fake arr should have received the POST.
	if postCount.Load() < 1 {
		t.Errorf("fake arr not called (postCount=%d)", postCount.Load())
	}

	// Row should now be submitted (or still queued if there was a race, but
	// since this is synchronous, it should be submitted).
	row, err := st.GetRequest(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "submitted" {
		t.Errorf("status=%q, want submitted", row.Status)
	}
}

func TestRetryNonFailedReturns400(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	r := &store.Request{
		ID:        "req-retry-nonfailed",
		TMDBID:    100,
		MediaType: "movie",
		Title:     "Movie",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.MarkSubmitted(t.Context(), r.ID, 5); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-retry-nonfailed/retry", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRetryMissingIDReturns404(t *testing.T) {
	handler, _ := newTestServerRouteTest(t)
	w := do(handler, adminReq("POST", "/api/admin/requests/does-not-exist/retry", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Re-route endpoint tests (Task 9.5)
// ---------------------------------------------------------------------------

func TestReRouteUnroutedRowReRunsRouting(t *testing.T) {
	var postCount atomic.Int32
	fakeArr := fakeRadarrServer(t, 88, &postCount)

	handler, st := newTestServerFull(t, fakeArr.URL)

	// The new arr added during re-route attempt.
	arrID := insertEnabledRadarr(t, st, "radarr-reroute", fakeArr.URL)
	_ = arrID

	// Insert + unroute a request (originally had no arr to match).
	r := &store.Request{
		ID:        "req-reroute-test",
		TMDBID:    9000,
		MediaType: "movie",
		Title:     "To Re-Route",
		Year:      2020,
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.MarkUnrouted(t.Context(), r.ID, []byte(`{"candidates":[]}`), "no arr"); err != nil {
		t.Fatalf("MarkUnrouted: %v", err)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-reroute-test/re-route", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
	}

	// Fake arr should have been called.
	if postCount.Load() < 1 {
		t.Errorf("fake arr not called (postCount=%d)", postCount.Load())
	}

	row, err := st.GetRequest(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "submitted" {
		t.Errorf("status=%q, want submitted", row.Status)
	}
}

func TestReRouteNonUnroutedReturns400(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	r := &store.Request{
		ID:        "req-reroute-nonunrouted",
		TMDBID:    200,
		MediaType: "movie",
		Title:     "Movie",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.MarkSubmitted(t.Context(), r.ID, 9); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-reroute-nonunrouted/re-route", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestReRouteMissingIDReturns404(t *testing.T) {
	handler, _ := newTestServerRouteTest(t)
	w := do(handler, adminReq("POST", "/api/admin/requests/ghost-id/re-route", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Force-fail endpoint tests
// ---------------------------------------------------------------------------

func TestForceFailSubmittedRowMarksFailed(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	r := &store.Request{
		ID:        "req-forcefail-submitted",
		TMDBID:    111,
		MediaType: "movie",
		Title:     "Stuck Movie",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.MarkSubmitted(t.Context(), r.ID, 42); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-forcefail-submitted/force-fail", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
	}

	row, err := st.GetRequest(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "failed" {
		t.Errorf("status=%q, want failed", row.Status)
	}
	if row.Error != "force-failed by admin" {
		t.Errorf("error=%q, want 'force-failed by admin'", row.Error)
	}
}

func TestForceFailDownloadingRowMarksFailed(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	r := &store.Request{
		ID:        "req-forcefail-downloading",
		TMDBID:    222,
		MediaType: "movie",
		Title:     "Downloading Movie",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.MarkSubmitted(t.Context(), r.ID, 43); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}
	if _, err := st.MarkDownloading(t.Context(), r.ID); err != nil {
		t.Fatalf("MarkDownloading: %v", err)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-forcefail-downloading/force-fail", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
	}

	row, err := st.GetRequest(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "failed" {
		t.Errorf("status=%q, want failed", row.Status)
	}
}

func TestForceFailOrphanedRowMarksFailed(t *testing.T) {
	// Simulate orphaned row: status=submitted, routed_arr_id=NULL (arr was deleted).
	handler, st := newTestServerRouteTest(t)

	// Insert an arr, link request to it, then delete the arr.
	arrID, err := st.CreateArr(t.Context(), &store.RegisteredArr{
		Name:      "arr-to-delete",
		Kind:      "radarr",
		URL:       "http://radarr",
		APIKey:    sealTestKey(t, "k"),
		Enabled:   true,
		Priority:  1,
		RulesJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateArr: %v", err)
	}

	r := &store.Request{
		ID:        "req-forcefail-orphaned",
		TMDBID:    333,
		MediaType: "movie",
		Title:     "Orphaned Movie",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.SetRoutedArr(t.Context(), r.ID, arrID, []byte(`{}`)); err != nil {
		t.Fatalf("SetRoutedArr: %v", err)
	}
	if err := st.MarkSubmitted(t.Context(), r.ID, 55); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}
	// Delete the arr — routed_arr_id becomes NULL via ON DELETE SET NULL.
	if err := st.DeleteArr(t.Context(), arrID); err != nil {
		t.Fatalf("DeleteArr: %v", err)
	}

	// Confirm orphaned state.
	row, err := st.GetRequest(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.RoutedArrID != nil {
		t.Fatalf("expected routed_arr_id=NULL after arr delete, got %v", *row.RoutedArrID)
	}
	if row.Status != "submitted" {
		t.Fatalf("expected status=submitted, got %s", row.Status)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-forcefail-orphaned/force-fail", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
	}

	row, err = st.GetRequest(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if row.Status != "failed" {
		t.Errorf("status=%q, want failed", row.Status)
	}
}

func TestForceFailTerminalReturns400(t *testing.T) {
	handler, st := newTestServerRouteTest(t)

	r := &store.Request{
		ID:        "req-forcefail-terminal",
		TMDBID:    444,
		MediaType: "movie",
		Title:     "Already Done",
	}
	if err := st.UpsertRequestQueued(t.Context(), r); err != nil {
		t.Fatalf("UpsertRequestQueued: %v", err)
	}
	if err := st.MarkSubmitted(t.Context(), r.ID, 66); err != nil {
		t.Fatalf("MarkSubmitted: %v", err)
	}
	if err := st.MarkImported(t.Context(), r.ID); err != nil {
		t.Fatalf("MarkImported: %v", err)
	}

	w := do(handler, adminReq("POST", "/api/admin/requests/req-forcefail-terminal/force-fail", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestForceFailMissingReturns404(t *testing.T) {
	handler, _ := newTestServerRouteTest(t)
	w := do(handler, adminReq("POST", "/api/admin/requests/no-such-id/force-fail", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Prerender / theme-injection tests (Task 10.2)
// ---------------------------------------------------------------------------

const spaHTML = `<!doctype html><html lang="en"><head></head><body><div id="root"></div></body></html>`

// spaWebFS returns an http.FileSystem backed by an in-memory index.html.
func spaWebFS(html string) http.FileSystem {
	return http.FS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(html)},
	})
}

// newTestServerSPA builds a server with a fake in-memory SPA filesystem.
func newTestServerSPA(t *testing.T, html string) http.Handler {
	t.Helper()
	st := newTestStore(t)
	deps := &server.Deps{
		Store:     st,
		SecretKey: testSecretKey,
		WebFS:     spaWebFS(html),
	}
	return server.New(deps).Handler()
}

func TestPrerenderInjectsThemeFromQuery(t *testing.T) {
	handler := newTestServerSPA(t, spaHTML)
	r := httptest.NewRequest("GET", "/admin/?theme=midnight-cinema", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	w := do(handler, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `data-theme="midnight-cinema"`) {
		t.Errorf("body missing data-theme=midnight-cinema: %s", w.Body.String())
	}
}

func TestPrerenderInjectsThemeFromHeader(t *testing.T) {
	handler := newTestServerSPA(t, spaHTML)
	r := httptest.NewRequest("GET", "/admin/", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	r.Header.Set("X-Continuum-Theme", "arctic-frost")
	w := do(handler, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `data-theme="arctic-frost"`) {
		t.Errorf("body missing data-theme=arctic-frost: %s", w.Body.String())
	}
}

func TestPrerenderHeaderTakesPrecedenceOverQuery(t *testing.T) {
	handler := newTestServerSPA(t, spaHTML)
	r := httptest.NewRequest("GET", "/admin/?theme=query-theme", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	r.Header.Set("X-Continuum-Theme", "header-theme")
	w := do(handler, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `data-theme="header-theme"`) {
		t.Errorf("header theme not injected: %s", body)
	}
	if strings.Contains(body, `data-theme="query-theme"`) {
		t.Errorf("query theme should not appear when header is set: %s", body)
	}
}

func TestPrerenderFallsBackToDefault(t *testing.T) {
	handler := newTestServerSPA(t, spaHTML)
	r := httptest.NewRequest("GET", "/admin/", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	// Neither header nor query param set.
	w := do(handler, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `data-theme="default"`) {
		t.Errorf("body missing data-theme=default: %s", w.Body.String())
	}
}

func TestPrerenderNoStoreCacheControl(t *testing.T) {
	handler := newTestServerSPA(t, spaHTML)
	r := httptest.NewRequest("GET", "/admin/", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	w := do(handler, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control=%q, want no-store", cc)
	}
}

func TestPrerenderEscapesQuotesInTheme(t *testing.T) {
	handler := newTestServerSPA(t, spaHTML)
	r := httptest.NewRequest("GET", "/admin/", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	r.Header.Set("X-Continuum-Theme", `mid"night"`)
	w := do(handler, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Raw double-quotes must not appear inside the attribute value.
	if strings.Contains(body, `data-theme="mid"night"`) {
		t.Errorf("unescaped quotes in theme attribute: %s", body)
	}
	if !strings.Contains(body, `&quot;`) {
		t.Errorf("expected &quot; escape in theme attribute: %s", body)
	}
}

// TestPrerenderReplacesExistingHtmlAttributes documents the known behavior:
// the regex replaces the WHOLE <html ...> tag, so existing attributes (e.g.
// lang="en") are lost in the rewrite. The SPA template must not rely on
// attributes other than data-theme. See prerender_handler.go for the constraint
// comment.
func TestPrerenderReplacesExistingHtmlAttributes(t *testing.T) {
	handler := newTestServerSPA(t, spaHTML) // spaHTML has <html lang="en">
	r := httptest.NewRequest("GET", "/admin/", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	r.Header.Set("X-Continuum-Theme", "cobalt-studio")
	w := do(handler, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// data-theme must be injected.
	if !strings.Contains(body, `data-theme="cobalt-studio"`) {
		t.Errorf("data-theme not injected: %s", body)
	}
	// Known behavior: lang="en" is lost because the whole tag is replaced.
	// This is intentional — the SPA template does not use lang= or other attrs.
	if strings.Contains(body, `lang="en"`) {
		t.Errorf("unexpected lang attr preserved (impl changed — update test): %s", body)
	}
}
