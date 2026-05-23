package server

import (
	"encoding/json"
	"net/http"

	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/routing"
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

	candidates, err := s.deps.Store.LoadCandidates(r.Context(), in.MediaType)
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
