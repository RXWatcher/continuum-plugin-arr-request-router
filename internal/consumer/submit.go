package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/arr"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/store"
)

// SubmitHandler is the workhorse for plugin.continuum.requests.submitted.
type SubmitHandler struct {
	Store     *store.Store
	Enricher  routing.Enricher
	Radarr    func(url, apiKey string) *arr.Radarr // factory; one client per *arr
	Sonarr    func(url, apiKey string) *arr.Sonarr
	Events    *event.Publisher
	SecretKey string
	Log       hclog.Logger
}

// HandleSubmitted satisfies the consumer.Submitter interface. It upserts the
// request row as 'queued' then delegates to Submit for routing + dispatch.
func (h *SubmitHandler) HandleSubmitted(ctx context.Context, p map[string]any) error {
	ev, err := parseSubmitPayload(p)
	if err != nil {
		return fmt.Errorf("submit: parse payload: %w", err)
	}

	r := &store.Request{
		ID:               ev.RequestID,
		TMDBID:           ev.TMDBID,
		MediaType:        ev.MediaType,
		Title:            ev.Title,
		Year:             ev.Year,
		PosterURL:        ev.PosterURL,
		RequesterUserID:  ev.RequesterUserID,
		RequesterIsAdmin: ev.RequesterIsAdmin,
		Status:           "queued",
	}
	if err := h.Store.UpsertRequestQueued(ctx, r); err != nil {
		return err
	}

	return h.Submit(ctx, ev)
}

// Submit runs the routing + arr-dispatch step. Exposed publicly so retry
// (Task 9.5) and re-route handlers can reuse it after the row is already
// in the store. Caller is responsible for the initial UpsertRequestQueued.
func (h *SubmitHandler) Submit(ctx context.Context, ev routing.RequestEvent) error {
	candidates, err := h.Store.LoadCandidates(ctx, ev.MediaType)
	if err != nil {
		return err
	}

	chosen, trace := routing.Decide(ctx, candidates, ev, h.Enricher)
	traceJSON, _ := json.Marshal(trace)

	if chosen == nil {
		if err := h.Store.MarkUnrouted(ctx, ev.RequestID, traceJSON, "no registered *arr matched"); err != nil {
			h.Log.Warn("MarkUnrouted failed", "id", ev.RequestID, "err", err)
		}
		h.Events.Unrouted(ctx, ev.RequestID, "no registered *arr matched")
		return nil
	}

	if err := h.Store.SetRoutedArr(ctx, ev.RequestID, *chosen, traceJSON); err != nil {
		return err
	}

	a, err := h.Store.GetArr(ctx, *chosen)
	if err != nil {
		return fmt.Errorf("get arr %d: %w", *chosen, err)
	}
	if a == nil {
		return fmt.Errorf("registered arr %d not found", *chosen)
	}

	apiKey, err := crypto.Open(h.SecretKey, a.APIKey)
	if err != nil {
		return fmt.Errorf("decrypt api_key: %w", err)
	}

	externalID, addErr := h.submitToArr(ctx, a, apiKey, ev)
	switch {
	case addErr == nil:
		if err := h.Store.MarkSubmitted(ctx, ev.RequestID, externalID); err != nil {
			return err
		}
		h.Events.Submitted(ctx, ev.RequestID)
		return nil
	case arr.IsConflict(addErr):
		// 409 — treat as already submitted. external_id may be 0 here; the
		// poll loop will re-discover it via title/tmdbId on next tick.
		if err := h.Store.MarkSubmitted(ctx, ev.RequestID, externalID); err != nil {
			return err
		}
		h.Events.Submitted(ctx, ev.RequestID)
		return nil
	default:
		// No fall-through to the next-priority *arr — intentional per SPEC.
		if err := h.Store.MarkFailed(ctx, ev.RequestID, addErr.Error()); err != nil {
			h.Log.Warn("MarkFailed failed", "id", ev.RequestID, "err", err)
		}
		h.Events.Failed(ctx, ev.RequestID, addErr.Error())
		return nil
	}
}

func (h *SubmitHandler) submitToArr(ctx context.Context, a *store.RegisteredArr, apiKey string, ev routing.RequestEvent) (int, error) {
	qID := 0
	if a.QualityProfileID != nil {
		qID = *a.QualityProfileID
	}
	lID := 0
	if a.LanguageProfileID != nil {
		lID = *a.LanguageProfileID
	}

	switch a.Kind {
	case "radarr":
		c := h.Radarr(a.URL, apiKey)
		root, resolvedQ, err := arr.ResolveRadarrDefaults(ctx, c, a.RootFolderPath, qID)
		if err != nil {
			return 0, err
		}
		movie, err := c.AddMovie(ctx, arr.AddMovieRequest{
			Title:            ev.Title,
			TMDBID:           ev.TMDBID,
			Year:             ev.Year,
			QualityProfileID: resolvedQ,
			RootFolderPath:   root,
			Monitored:        true,
			AddOptions:       map[string]any{"searchForMovie": true},
		})
		if err != nil {
			return 0, err
		}
		return movie.ID, nil

	case "sonarr":
		c := h.Sonarr(a.URL, apiKey)
		want := arr.SonarrDefaults{
			RootFolderPath:    a.RootFolderPath,
			QualityProfileID:  qID,
			LanguageProfileID: lID,
		}
		resolved, err := arr.ResolveSonarrDefaults(ctx, c, want)
		if err != nil {
			return 0, err
		}
		series, err := c.AddSeries(ctx, arr.AddSeriesRequest{
			Title:             ev.Title,
			TMDBID:            ev.TMDBID,
			Year:              ev.Year,
			QualityProfileID:  resolved.QualityProfileID,
			LanguageProfileID: resolved.LanguageProfileID,
			RootFolderPath:    resolved.RootFolderPath,
			Monitored:         true,
			SeasonFolder:      true,
			AddOptions:        map[string]any{"searchForMissingEpisodes": true},
		})
		if err != nil {
			return 0, err
		}
		return series.ID, nil
	}

	return 0, fmt.Errorf("unknown arr kind %q", a.Kind)
}

// parseSubmitPayload converts the type-erased event payload into a
// RequestEvent. Returns an error if any required field is missing or has
// the wrong type. Numeric fields arrive as float64 from JSON unmarshal.
func parseSubmitPayload(p map[string]any) (routing.RequestEvent, error) {
	ev := routing.RequestEvent{}

	if ev.RequestID = stringField(p, "requestId", "request_id"); ev.RequestID == "" {
		return ev, fmt.Errorf("missing requestId")
	}
	if ev.MediaType = stringField(p, "mediaType", "media_type"); ev.MediaType != "movie" && ev.MediaType != "tv" {
		return ev, fmt.Errorf("invalid mediaType: %v", ev.MediaType)
	}
	tmdbF := floatField(p, "tmdbId", "tmdb_id")
	if tmdbF == 0 {
		return ev, fmt.Errorf("missing tmdbId")
	}
	ev.TMDBID = int(tmdbF)

	// Optional fields — best-effort coercion.
	if v, ok := p["title"].(string); ok {
		ev.Title = v
	}
	if v, ok := p["year"].(float64); ok {
		ev.Year = int(v)
	}
	if v := stringField(p, "posterUrl", "poster_url"); v != "" {
		ev.PosterURL = v
	}
	if v := stringField(p, "libraryId", "library_id"); v != "" {
		ev.LibraryID = v
	}
	if v := stringField(p, "requesterUserId", "requester_user_id"); v != "" {
		ev.RequesterUserID = v
	}
	if v, ok := p["requesterIsAdmin"].(bool); ok {
		ev.RequesterIsAdmin = v
	}

	return ev, nil
}

func stringField(p map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, _ := p[key].(string); v != "" {
			return v
		}
	}
	return ""
}

func floatField(p map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if v, _ := p[key].(float64); v != 0 {
			return v
		}
	}
	return 0
}
