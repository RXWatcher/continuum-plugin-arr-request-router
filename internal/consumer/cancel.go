package consumer

import (
	"context"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/arr"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/store"
)

// CancelHandler processes plugin.continuum.requests.cancelled events.
type CancelHandler struct {
	Store     *store.Store
	Radarr    func(url, apiKey string) *arr.Radarr
	Sonarr    func(url, apiKey string) *arr.Sonarr
	Events    *event.Publisher
	SecretKey string
	Log       hclog.Logger
}

// HandleCancelled implements consumer.Canceller.
//
// Steps:
//  1. Parse requestId from payload — missing → silent no-op.
//  2. Load the request row — not found → no-op.
//  3. Terminal status (imported/failed/cancelled/unrouted) → no-op.
//  4. If routed_arr_id AND external_id are both set → best-effort *arr DELETE.
//  5. MarkCancelled in the store (store-side guard prevents double-cancel).
//  6. Publish Events.Cancelled (void).
func (h *CancelHandler) HandleCancelled(ctx context.Context, p map[string]any) error {
	id, _ := p["requestId"].(string)
	if id == "" {
		return nil
	}

	r, err := h.Store.GetRequest(ctx, id)
	if err != nil {
		return err
	}
	if r == nil {
		return nil
	}

	switch r.Status {
	case "imported", "failed", "cancelled", "unrouted":
		return nil
	}

	if r.RoutedArrID != nil && r.ExternalID != nil {
		h.tryArrDelete(ctx, r)
	}

	if err := h.Store.MarkCancelled(ctx, id); err != nil {
		h.Log.Warn("MarkCancelled failed", "id", id, "err", err)
		return err
	}
	h.Events.Cancelled(ctx, id)
	return nil
}

// tryArrDelete is best-effort: it logs every failure and swallows errors so
// that MarkCancelled always runs regardless of *arr availability.
func (h *CancelHandler) tryArrDelete(ctx context.Context, r *store.Request) {
	a, err := h.Store.GetArr(ctx, *r.RoutedArrID)
	if err != nil {
		h.Log.Warn("cancel: get arr", "routed_arr_id", *r.RoutedArrID, "err", err)
		return
	}
	if a == nil {
		// arr was deleted; ON DELETE SET NULL propagated — nothing to do upstream.
		return
	}

	apiKey, err := crypto.Open(h.SecretKey, a.APIKey)
	if err != nil {
		h.Log.Warn("cancel: decrypt api_key", "arr_id", a.ID, "err", err)
		return
	}

	switch a.Kind {
	case "radarr":
		if err := h.Radarr(a.URL, apiKey).DeleteMovie(ctx, *r.ExternalID); err != nil {
			h.Log.Warn("cancel: radarr DeleteMovie", "external_id", *r.ExternalID, "err", err)
		}
	case "sonarr":
		if err := h.Sonarr(a.URL, apiKey).DeleteSeries(ctx, *r.ExternalID); err != nil {
			h.Log.Warn("cancel: sonarr DeleteSeries", "external_id", *r.ExternalID, "err", err)
		}
	default:
		h.Log.Warn("cancel: unknown arr kind", "kind", a.Kind)
	}
}
