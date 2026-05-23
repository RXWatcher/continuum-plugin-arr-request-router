package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/arr"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/crypto"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/store"
)

// liveItem is one in-flight download: a movie's file or one series episode.
type liveItem struct {
	Label    string  `json:"label"`
	Size     int64   `json:"size"`
	SizeLeft int64   `json:"size_left"`
	Pct      float64 `json:"pct"`
	State    string  `json:"state"`
	Error    string  `json:"error,omitempty"`
}

// liveStatus is the on-demand, freshly-probed status of one request — the
// shared contract every request_router.v1 plugin exposes so continuum.requests
// can reconcile against ground truth. Shape matches continuum.arrproxy's.
type liveStatus struct {
	RequestID   string     `json:"request_id"`
	Found       bool       `json:"found"`
	Status      string     `json:"status"`
	MediaType   string     `json:"media_type"`
	ExternalID  int        `json:"external_id"`
	ProgressPct float64    `json:"progress_pct"`
	TotalCount  int        `json:"total_count"`
	HaveCount   int        `json:"have_count"`
	Items       []liveItem `json:"items"`
	Error       string     `json:"error,omitempty"`
	CheckedAt   string     `json:"checked_at"`
}

func itemState(q arr.QueueItem) string {
	if q.ErrorMessage != "" || q.Status == "warning" || q.Status == "failed" {
		return "stalled"
	}
	switch q.TrackedDownloadState {
	case "importPending", "importing", "imported":
		return "importing"
	}
	if q.Status == "queued" || q.Status == "delay" || q.Status == "paused" {
		return "queued"
	}
	return "downloading"
}

func itemPct(size, left int64) float64 {
	if size <= 0 || left < 0 || left > size {
		return 0
	}
	return float64(size-left) / float64(size) * 100
}

func episodeLabel(q arr.QueueItem, seriesTitle string) string {
	if q.Episode != nil {
		base := fmt.Sprintf("S%02dE%02d", q.Episode.SeasonNumber, q.Episode.EpisodeNumber)
		if q.Episode.Title != "" {
			return base + " · " + q.Episode.Title
		}
		return base
	}
	if q.Title != "" {
		return q.Title
	}
	return seriesTitle
}

// resolveLive does a fresh probe against whichever registered *arr routed the
// request. Read-only: never mutates the store or publishes events.
func (s *Server) resolveLive(ctx context.Context, r *store.Request) liveStatus {
	out := liveStatus{
		RequestID: r.ID,
		Found:     true,
		Status:    r.Status,
		MediaType: r.MediaType,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if r.ExternalID != nil {
		out.ExternalID = *r.ExternalID
	}
	if r.Status == "cancelled" {
		return out
	}
	if r.RoutedArrID == nil || r.ExternalID == nil {
		return out // not yet routed/submitted — nothing live to probe
	}
	a, err := s.deps.Store.GetArr(ctx, *r.RoutedArrID)
	if err != nil || a == nil {
		out.Error = "routed *arr not found"
		return out
	}
	apiKey, err := crypto.Open(s.deps.SecretKey, a.APIKey)
	if err != nil {
		out.Error = "decrypt api key failed"
		return out
	}
	switch r.MediaType {
	case "movie":
		s.resolveLiveMovie(ctx, r, a.URL, apiKey, &out)
	case "tv":
		s.resolveLiveSeries(ctx, r, a.URL, apiKey, &out)
	}
	return out
}

func (s *Server) resolveLiveMovie(ctx context.Context, r *store.Request, url, apiKey string, out *liveStatus) {
	if s.deps.Radarr == nil {
		return
	}
	c := s.deps.Radarr(url, apiKey)
	movie, err := c.GetMovie(ctx, *r.ExternalID)
	if err != nil {
		if errors.Is(err, arr.ErrNotFound) {
			out.Status = "cancelled"
			return
		}
		out.Error = err.Error()
		return
	}
	out.TotalCount = 1
	if movie.HasFile {
		out.Status = "imported"
		out.HaveCount = 1
		out.ProgressPct = 100
		return
	}
	queue, err := c.QueueByMovie(ctx, *r.ExternalID)
	if err != nil {
		out.Error = err.Error()
		return
	}
	for _, q := range queue {
		label := movie.Title
		if label == "" {
			label = r.Title
		}
		out.Items = append(out.Items, liveItem{
			Label:    label,
			Size:     q.Size,
			SizeLeft: q.SizeLeft,
			Pct:      itemPct(q.Size, q.SizeLeft),
			State:    itemState(q),
			Error:    q.ErrorMessage,
		})
	}
	if len(out.Items) > 0 {
		out.Status = "downloading"
		out.ProgressPct = out.Items[0].Pct
	}
}

func (s *Server) resolveLiveSeries(ctx context.Context, r *store.Request, url, apiKey string, out *liveStatus) {
	if s.deps.Sonarr == nil {
		return
	}
	c := s.deps.Sonarr(url, apiKey)
	series, err := c.GetSeries(ctx, *r.ExternalID)
	if err != nil {
		if errors.Is(err, arr.ErrNotFound) {
			out.Status = "cancelled"
			return
		}
		out.Error = err.Error()
		return
	}
	out.TotalCount = series.Statistics.EpisodeCount
	out.HaveCount = series.Statistics.EpisodeFileCount
	out.ProgressPct = series.Statistics.PercentOfEpisodes
	if series.Statistics.PercentOfEpisodes >= 100 {
		out.Status = "imported"
		return
	}
	queue, err := c.QueueBySeries(ctx, *r.ExternalID)
	if err != nil {
		out.Error = err.Error()
		return
	}
	for _, q := range queue {
		out.Items = append(out.Items, liveItem{
			Label:    episodeLabel(q, r.Title),
			Size:     q.Size,
			SizeLeft: q.SizeLeft,
			Pct:      itemPct(q.Size, q.SizeLeft),
			State:    itemState(q),
			Error:    q.ErrorMessage,
		})
	}
	if len(out.Items) > 0 {
		out.Status = "downloading"
	}
}

// handleRequestsLive serves GET /api/admin/requests/{id}/live.
func (s *Server) handleRequestsLive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.deps.Store.GetRequest(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if row == nil {
		writeJSON(w, http.StatusOK, liveStatus{
			RequestID: id,
			Found:     false,
			Status:    "not_found",
			CheckedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.resolveLive(ctx, row))
}

type liveBulkRequest struct {
	IDs []string `json:"ids"`
}

// handleRequestsLiveBulk serves POST /api/admin/requests/live.
func (s *Server) handleRequestsLiveBulk(w http.ResponseWriter, r *http.Request) {
	var body liveBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	const maxIDs = 200
	if len(body.IDs) > maxIDs {
		body.IDs = body.IDs[:maxIDs]
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	items := make([]liveStatus, 0, len(body.IDs))
	for _, id := range body.IDs {
		row, err := s.deps.Store.GetRequest(ctx, id)
		if err != nil || row == nil {
			items = append(items, liveStatus{
				RequestID: id,
				Found:     false,
				Status:    "not_found",
				CheckedAt: time.Now().UTC().Format(time.RFC3339),
			})
			continue
		}
		items = append(items, s.resolveLive(ctx, row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
