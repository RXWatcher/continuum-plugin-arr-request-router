package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/arr"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/crypto"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/routing"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/store"
)

// registryDTO is the wire shape for /api/admin/registry/*.
// api_key is write-only (never emitted on read).
type registryDTO struct {
	ID                int64           `json:"id,omitempty"`
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	URL               string          `json:"url"`
	APIKey            string          `json:"api_key,omitempty"`       // write-only
	HasAPIKey         bool            `json:"has_api_key,omitempty"`   // read-only
	RootFolderPath    string          `json:"root_folder_path"`
	QualityProfileID  *int            `json:"quality_profile_id,omitempty"`
	LanguageProfileID *int            `json:"language_profile_id,omitempty"`
	Priority          int             `json:"priority"`
	Enabled           bool            `json:"enabled"`
	Rules             json.RawMessage `json:"rules"`
}

func (s *Server) registryRoutes(r chi.Router) {
	r.Get("/", s.handleRegistryList)
	r.Post("/", s.handleRegistryCreate)
	r.Get("/{id}", s.handleRegistryGet)
	r.Patch("/{id}", s.handleRegistryUpdate)
	r.Delete("/{id}", s.handleRegistryDelete)
	r.Post("/{id}/test-connection", s.handleRegistryTestConnection)
}

func toDTO(a *store.RegisteredArr) registryDTO {
	return registryDTO{
		ID:                a.ID,
		Name:              a.Name,
		Kind:              a.Kind,
		URL:               a.URL,
		HasAPIKey:         a.APIKey != "",
		RootFolderPath:    a.RootFolderPath,
		QualityProfileID:  a.QualityProfileID,
		LanguageProfileID: a.LanguageProfileID,
		Priority:          a.Priority,
		Enabled:           a.Enabled,
		Rules:             a.RulesJSON,
	}
}

func (s *Server) handleRegistryList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.Store.ListArrs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := make([]registryDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, toDTO(a))
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleRegistryGet(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	a, err := s.deps.Store.GetArr(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if a == nil {
		http.Error(w, "not found", 404)
		return
	}
	writeJSON(w, 200, toDTO(a))
}

func (s *Server) handleRegistryCreate(w http.ResponseWriter, r *http.Request) {
	var dto registryDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if dto.Kind != "radarr" && dto.Kind != "sonarr" {
		http.Error(w, "bad kind", 400)
		return
	}
	rules, err := routing.ParseRules(dto.Rules)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := routing.ValidateRules(rules); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if dto.APIKey == "" {
		http.Error(w, "api_key required", 400)
		return
	}

	sealed, err := crypto.Seal(s.deps.SecretKey, dto.APIKey)
	if err != nil {
		http.Error(w, "seal: "+err.Error(), 500)
		return
	}

	a := &store.RegisteredArr{
		Name:              dto.Name,
		Kind:              dto.Kind,
		URL:               dto.URL,
		APIKey:            sealed,
		RootFolderPath:    dto.RootFolderPath,
		QualityProfileID:  dto.QualityProfileID,
		LanguageProfileID: dto.LanguageProfileID,
		Priority:          dto.Priority,
		Enabled:           dto.Enabled,
		RulesJSON:         dto.Rules,
	}
	id, err := s.deps.Store.CreateArr(r.Context(), a)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

// handleRegistryUpdate implements PATCH semantics: only update fields present
// in the body. API key only rotates if "api_key" is present and non-empty.
func (s *Server) handleRegistryUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	a, err := s.deps.Store.GetArr(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if a == nil {
		http.Error(w, "not found", 404)
		return
	}

	// Decode into a scratch map so we know which fields are present.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "bad json", 400)
		return
	}

	decode := func(key string, dst any) (bool, error) {
		v, ok := raw[key]
		if !ok {
			return false, nil
		}
		return true, json.Unmarshal(v, dst)
	}

	if _, err := decode("name", &a.Name); err != nil {
		http.Error(w, "bad name", 400)
		return
	}
	if _, err := decode("url", &a.URL); err != nil {
		http.Error(w, "bad url", 400)
		return
	}
	if _, err := decode("root_folder_path", &a.RootFolderPath); err != nil {
		http.Error(w, "bad root_folder_path", 400)
		return
	}
	if _, err := decode("priority", &a.Priority); err != nil {
		http.Error(w, "bad priority", 400)
		return
	}
	if _, err := decode("enabled", &a.Enabled); err != nil {
		http.Error(w, "bad enabled", 400)
		return
	}
	if _, err := decode("quality_profile_id", &a.QualityProfileID); err != nil {
		http.Error(w, "bad quality_profile_id", 400)
		return
	}
	if _, err := decode("language_profile_id", &a.LanguageProfileID); err != nil {
		http.Error(w, "bad language_profile_id", 400)
		return
	}

	if v, ok := raw["kind"]; ok {
		var kind string
		if err := json.Unmarshal(v, &kind); err != nil {
			http.Error(w, "bad kind", 400)
			return
		}
		if kind != "radarr" && kind != "sonarr" {
			http.Error(w, "bad kind", 400)
			return
		}
		a.Kind = kind
	}

	if v, ok := raw["rules"]; ok {
		rules, err := routing.ParseRules(v)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := routing.ValidateRules(rules); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.RulesJSON = []byte(v)
	}

	if v, ok := raw["api_key"]; ok {
		var key string
		if err := json.Unmarshal(v, &key); err != nil {
			http.Error(w, "bad api_key", 400)
			return
		}
		if key != "" {
			// Non-empty api_key in PATCH body → rotate.
			sealed, err := crypto.Seal(s.deps.SecretKey, key)
			if err != nil {
				http.Error(w, "seal: "+err.Error(), 500)
				return
			}
			a.APIKey = sealed
		}
		// Empty string → DO NOT rotate (preserve existing key).
	}

	if err := s.deps.Store.UpdateArr(r.Context(), a); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRegistryDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.deps.Store.DeleteArr(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRegistryTestConnection (Task 9.3) probes the arr instance.
// Body: optional {"api_key": "..."} — overrides the stored key for this
// probe. If absent, decrypt the stored key and use it. Returns 200 with
// SystemStatus on success, 400/502 on failure.
func (s *Server) handleRegistryTestConnection(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	a, err := s.deps.Store.GetArr(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if a == nil {
		http.Error(w, "not found", 404)
		return
	}

	var body struct {
		APIKey string `json:"api_key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // body is optional

	apiKey := body.APIKey
	if apiKey == "" {
		apiKey, err = crypto.Open(s.deps.SecretKey, a.APIKey)
		if err != nil {
			http.Error(w, "decrypt failed", 500)
			return
		}
	}

	status, err := arr.SystemStatus(r.Context(), a.URL, apiKey)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, 200, status)
}

// writeJSON sets Content-Type and encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ErrNotFound is a sentinel for "row not found" conditions.
var ErrNotFound = errors.New("not found")
