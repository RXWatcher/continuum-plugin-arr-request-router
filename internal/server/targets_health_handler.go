package server

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/arr"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/crypto"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/store"
)

// healthProbeTimeout caps a single SystemStatus probe; the per-arr probes
// run concurrently so this is also the upper bound on the whole endpoint.
const healthProbeTimeout = 4 * time.Second

// targetHealthRow is the wire shape for /api/admin/targets/health. Combines
// the configured registry record with a live SystemStatus probe and 24h
// rolling counters from the request store.
type targetHealthRow struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Kind            string     `json:"kind"`
	URL             string     `json:"url"`
	Enabled         bool       `json:"enabled"`
	Priority        int        `json:"priority"`
	Probe           string     `json:"probe"` // "reachable" | "unauthorized" | "unreachable" | "skipped"
	ProbeLatencyMs  int64      `json:"probeLatencyMs"`
	ProbeError      string     `json:"probeError,omitempty"`
	Version         string     `json:"version,omitempty"`
	Submitted24h    int        `json:"submitted24h"`
	Failed24h       int        `json:"failed24h"`
	Imported24h     int        `json:"imported24h"`
	LastSubmittedAt *time.Time `json:"lastSubmittedAt,omitempty"`
	LastFailureAt   *time.Time `json:"lastFailureAt,omitempty"`
	LastFailureMsg  string     `json:"lastFailureMsg,omitempty"`
}

func (s *Server) handleTargetsHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	arrs, err := s.deps.Store.ListArrs(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	healthRows, err := s.deps.Store.TargetHealth(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	byID := make(map[int64]store.ArrHealthRow, len(healthRows))
	for _, h := range healthRows {
		byID[h.ArrID] = h
	}

	out := make([]targetHealthRow, len(arrs))
	var wg sync.WaitGroup
	for i, a := range arrs {
		i, a := i, a
		row := targetHealthRow{
			ID:       a.ID,
			Name:     a.Name,
			Kind:     a.Kind,
			URL:      a.URL,
			Enabled:  a.Enabled,
			Priority: a.Priority,
			Probe:    "skipped",
		}
		if h, ok := byID[a.ID]; ok {
			row.Submitted24h = h.Submitted24h
			row.Failed24h = h.Failed24h
			row.Imported24h = h.Imported24h
			row.LastSubmittedAt = h.LastSubmittedAt
			row.LastFailureAt = h.LastFailureAt
			row.LastFailureMsg = h.LastFailureReason
		}
		out[i] = row
		if !a.Enabled {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
			defer cancel()
			apiKey, err := crypto.Open(s.deps.SecretKey, a.APIKey)
			if err != nil {
				out[i].Probe = "unauthorized"
				out[i].ProbeError = "decrypt api_key failed"
				return
			}
			start := time.Now()
			status, err := arr.SystemStatus(probeCtx, a.URL, apiKey)
			out[i].ProbeLatencyMs = time.Since(start).Milliseconds()
			if err != nil {
				out[i].Probe = "unreachable"
				out[i].ProbeError = err.Error()
				return
			}
			out[i].Probe = "reachable"
			out[i].Version = status.Version
		}()
	}
	wg.Wait()

	// Stable order: priority asc, name asc — same convention rule evaluation uses.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].Name < out[j].Name
	})

	writeJSON(w, http.StatusOK, out)
}
