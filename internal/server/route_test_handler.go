package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

// POST /api/admin/route-test
// Body: { "tmdbId": 603, "mediaType": "movie", "title": "...", "year": 1999 }
// Response: { "chosen": <id>|null, "trace": {...} }
// Read-only: this never writes to the DB.
func (s *Server) handleRouteTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TMDBID    int    `json:"tmdbId"`
		MediaType string `json:"mediaType"`
		Title     string `json:"title"`
		Year      int    `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if in.TMDBID == 0 {
		http.Error(w, "tmdbId required", 400)
		return
	}
	if in.MediaType != "movie" && in.MediaType != "tv" {
		http.Error(w, "mediaType must be movie or tv", 400)
		return
	}

	candidates, err := s.loadCandidates(r.Context(), in.MediaType)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	ev := routing.RequestEvent{
		TMDBID:    in.TMDBID,
		MediaType: in.MediaType,
		Title:     in.Title,
		Year:      in.Year,
	}
	chosen, trace := routing.Decide(r.Context(), candidates, ev, s.deps.Enricher)
	writeJSON(w, 200, map[string]any{"chosen": chosen, "trace": trace})
}

// loadCandidates loads the enabled registered arrs for the given mediaType
// and returns them as routing.Candidate values. Shared with the consumer's
// submit path; duplicated here per task spec.
func (s *Server) loadCandidates(ctx context.Context, mediaType string) ([]routing.Candidate, error) {
	kind := "radarr"
	if mediaType == "tv" {
		kind = "sonarr"
	}
	rows, err := s.deps.Store.ListEnabledArrsByKind(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]routing.Candidate, 0, len(rows))
	for _, row := range rows {
		rules, _ := routing.ParseRules(row.RulesJSON)
		out = append(out, routing.Candidate{ID: row.ID, Name: row.Name, Kind: row.Kind, Rules: rules})
	}
	return out, nil
}
