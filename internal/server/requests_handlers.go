package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/routing"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/store"
)

type requestDTO struct {
	ID               string          `json:"id"`
	TMDBID           int             `json:"tmdb_id"`
	MediaType        string          `json:"media_type"`
	Title            string          `json:"title"`
	Year             int             `json:"year"`
	PosterURL        string          `json:"poster_url,omitempty"`
	RequesterUserID  string          `json:"requester_user_id,omitempty"`
	RequesterIsAdmin bool            `json:"requester_is_admin,omitempty"`
	Status           string          `json:"status"`
	RoutedArrID      *int64          `json:"routed_arr_id,omitempty"`
	RoutedArrName    string          `json:"routed_arr_name,omitempty"`
	ExternalID       *int            `json:"external_id,omitempty"`
	Error            string          `json:"error,omitempty"`
	MatchTrace       json.RawMessage `json:"match_trace,omitempty"`
	SubmittedAt      string          `json:"submitted_at,omitempty"`
	LastPolledAt     string          `json:"last_polled_at,omitempty"`
	CompletedAt      string          `json:"completed_at,omitempty"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

func (s *Server) requestsRoutes(r chi.Router) {
	r.Get("/", s.handleRequestsList)
	r.Get("/{id}", s.handleRequestsGet)
	r.Post("/{id}/retry", s.handleRequestsRetry)
	r.Post("/{id}/re-route", s.handleRequestsReRoute)
	r.Post("/{id}/force-fail", s.handleRequestsForceFail)
}

// routerRoutes is the inter-plugin contract surface, mounted under
// /api/router (manifest access: public) so host CallPluginHTTP calls from
// continuum.requests — which carry no admin user and would 403 against
// /api/admin — can reach the read-only live-status endpoints.
func (s *Server) routerRoutes(r chi.Router) {
	r.Post("/requests/live", s.handleRequestsLiveBulk)
	r.Get("/requests/{id}/live", s.handleRequestsLive)
}

func (s *Server) handleRequestsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	rows, total, err := s.deps.Store.ListRequestsForAdmin(r.Context(), status, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Attach arr_name for each row's routed_arr_id (best-effort; N+1 acceptable
	// since the registry is small per SPEC).
	out := make([]requestDTO, 0, len(rows))
	for _, row := range rows {
		d := toRequestDTO(row)
		if row.RoutedArrID != nil {
			if a, _ := s.deps.Store.GetArr(r.Context(), *row.RoutedArrID); a != nil {
				d.RoutedArrName = a.Name
			}
		}
		out = append(out, d)
	}
	writeJSON(w, 200, map[string]any{"rows": out, "total": total})
}

func (s *Server) handleRequestsGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.deps.Store.GetRequest(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if row == nil {
		http.Error(w, "not found", 404)
		return
	}
	d := toRequestDTO(row)
	if row.RoutedArrID != nil {
		if a, _ := s.deps.Store.GetArr(r.Context(), *row.RoutedArrID); a != nil {
			d.RoutedArrName = a.Name
		}
	}
	writeJSON(w, 200, d)
}

// handleRequestsRetry re-runs Submit on a row in `failed` status.
// It resets the row to 'queued' via MarkRetrying before calling Submit
// so that MarkSubmitted's status guard will accept the transition.
func (s *Server) handleRequestsRetry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.deps.Store.GetRequest(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if row == nil {
		http.Error(w, "not found", 404)
		return
	}
	if row.Status != "failed" {
		http.Error(w, "row is not in failed state", 400)
		return
	}
	if err := s.deps.Store.MarkRetrying(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	ev := routing.RequestEvent{
		RequestID:        row.ID,
		MediaType:        row.MediaType,
		TMDBID:           row.TMDBID,
		Title:            row.Title,
		Year:             row.Year,
		RequesterUserID:  row.RequesterUserID,
		RequesterIsAdmin: row.RequesterIsAdmin,
		PosterURL:        row.PosterURL,
	}
	if err := s.deps.Submit.Submit(r.Context(), ev); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleRequestsReRoute re-runs Submit on a row in `unrouted` status.
// It resets the row to 'queued' via MarkReRouting before calling Submit.
func (s *Server) handleRequestsReRoute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.deps.Store.GetRequest(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if row == nil {
		http.Error(w, "not found", 404)
		return
	}
	if row.Status != "unrouted" {
		http.Error(w, "row is not in unrouted state", 400)
		return
	}
	if err := s.deps.Store.MarkReRouting(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	ev := routing.RequestEvent{
		RequestID:        row.ID,
		MediaType:        row.MediaType,
		TMDBID:           row.TMDBID,
		Title:            row.Title,
		Year:             row.Year,
		RequesterUserID:  row.RequesterUserID,
		RequesterIsAdmin: row.RequesterIsAdmin,
		PosterURL:        row.PosterURL,
	}
	if err := s.deps.Submit.Submit(r.Context(), ev); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleRequestsForceFail manually fails a row stuck in submitted/downloading.
// Useful for orphaned rows whose registered *arr was deleted (routed_arr_id
// became NULL via ON DELETE SET NULL, so the poll loop can't reach them).
//
// Returns 400 if the row is already terminal — there's nothing to fail.
func (s *Server) handleRequestsForceFail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.deps.Store.GetRequest(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if row == nil {
		http.Error(w, "not found", 404)
		return
	}
	switch row.Status {
	case "imported", "failed", "cancelled", "unrouted":
		http.Error(w, "row is already terminal", 400)
		return
	}
	if err := s.deps.Store.MarkFailed(r.Context(), id, "force-failed by admin"); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func toRequestDTO(r *store.Request) requestDTO {
	iso := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	return requestDTO{
		ID:               r.ID,
		TMDBID:           r.TMDBID,
		MediaType:        r.MediaType,
		Title:            r.Title,
		Year:             r.Year,
		PosterURL:        r.PosterURL,
		RequesterUserID:  r.RequesterUserID,
		RequesterIsAdmin: r.RequesterIsAdmin,
		Status:           r.Status,
		RoutedArrID:      r.RoutedArrID,
		ExternalID:       r.ExternalID,
		Error:            r.Error,
		MatchTrace:       json.RawMessage(r.MatchTrace),
		SubmittedAt:      iso(r.SubmittedAt),
		LastPolledAt:     iso(r.LastPolledAt),
		CompletedAt:      iso(r.CompletedAt),
		CreatedAt:        r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
