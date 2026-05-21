// Package poll runs periodic status reconciliation. It walks rows in status
// submitted/downloading and asks Radarr/Sonarr what's happened since the last
// check. State transitions publish events so consumers can mirror them.
package poll

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/arr"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/crypto"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/event"
	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/store"
)

// Deps is the runtime state the poller needs. A nil return from the deps
// function signals that the plugin is not yet configured; Run treats it as a
// clean no-op.
type Deps struct {
	Store           *store.Store
	Radarr          func(url, apiKey string) *arr.Radarr
	Sonarr          func(url, apiKey string) *arr.Sonarr
	Events          *event.Publisher
	StaleAfterHours int
	SecretKey       string
}

// Poller drives one iteration per Run call. It is stateless across calls; all
// state lives in the DB.
type Poller struct {
	deps func() *Deps
	log  hclog.Logger
	now  func() time.Time // injectable for tests; defaults to time.Now
}

// New constructs a Poller. deps may return nil to signal unconfigured state.
// log may be nil (defaults to null logger).
func New(deps func() *Deps, log hclog.Logger) *Poller {
	return NewWithClock(deps, log, time.Now)
}

// NewWithClock constructs a Poller with an injectable clock. nowFn is called
// each Run to obtain the current time; pass time.Now for production use.
// Useful in tests to simulate the passage of time for stale-threshold checks.
func NewWithClock(deps func() *Deps, log hclog.Logger, nowFn func() time.Time) *Poller {
	if log == nil {
		log = hclog.NewNullLogger()
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Poller{
		deps: deps,
		log:  log,
		now:  nowFn,
	}
}

// Run is the scheduled-task entrypoint. It groups all pollable rows by their
// routed arr and fans out one goroutine per arr; within an arr the rows are
// polled sequentially. A nil Deps result (host not configured yet) is a clean
// no-op.
func (p *Poller) Run(ctx context.Context) error {
	d := p.deps()
	if d == nil {
		return nil
	}

	rows, err := d.Store.ListPollable(ctx)
	if err != nil {
		return err
	}

	byArr := groupByArr(rows)
	now := p.now().UTC()

	var wg sync.WaitGroup
	for arrID, group := range byArr {
		a, err := d.Store.GetArr(ctx, arrID)
		if err != nil || a == nil || !a.Enabled {
			p.log.Debug("skipping arr group", "arr_id", arrID)
			continue
		}
		apiKey, err := crypto.Open(d.SecretKey, a.APIKey)
		if err != nil {
			p.log.Warn("decrypt api_key failed", "arr_id", arrID, "err", err)
			continue
		}
		wg.Add(1)
		go func(a *store.RegisteredArr, apiKey string, group []*store.Request) {
			defer wg.Done()
			for _, r := range group {
				p.pollOne(ctx, d, r, a, apiKey, now)
			}
		}(a, apiKey, group)
	}
	wg.Wait()
	return nil
}

// groupByArr partitions rows by RoutedArrID. Rows with a nil RoutedArrID are
// excluded (nothing to poll against).
func groupByArr(rows []*store.Request) map[int64][]*store.Request {
	out := map[int64][]*store.Request{}
	for _, r := range rows {
		if r.RoutedArrID == nil {
			continue
		}
		out[*r.RoutedArrID] = append(out[*r.RoutedArrID], r)
	}
	return out
}

// pollOne dispatches to the appropriate media-type handler, then unconditionally
// bumps last_polled_at.
func (p *Poller) pollOne(ctx context.Context, d *Deps, r *store.Request, a *store.RegisteredArr, apiKey string, now time.Time) {
	switch r.MediaType {
	case "movie":
		p.pollMovie(ctx, d, r, a, apiKey, now)
	case "tv":
		p.pollTV(ctx, d, r, a, apiKey, now)
	}
	if err := d.Store.UpdateLastPolled(ctx, r.ID, now); err != nil {
		p.log.Warn("UpdateLastPolled failed", "id", r.ID, "err", err)
	}
}

// pollMovie handles one movie row against Radarr.
func (p *Poller) pollMovie(ctx context.Context, d *Deps, r *store.Request, a *store.RegisteredArr, apiKey string, now time.Time) {
	// 1. No external ID → nothing to query yet.
	if r.ExternalID == nil {
		return
	}

	c := d.Radarr(a.URL, apiKey)

	// 2. Fetch movie from Radarr.
	movie, err := c.GetMovie(ctx, *r.ExternalID)
	if err != nil {
		p.log.Warn("radarr GetMovie error", "id", r.ID, "external_id", *r.ExternalID, "err", err)
		return
	}

	// 3. HasFile → imported transition (fire event only on actual transition).
	if movie.HasFile {
		if r.Status != "imported" {
			if err := d.Store.MarkImported(ctx, r.ID); err != nil {
				p.log.Warn("MarkImported failed", "id", r.ID, "err", err)
				return
			}
			d.Events.Imported(ctx, r.ID)
		}
		return
	}

	// 4. Not imported — check queue.
	queue, err := c.QueueByMovie(ctx, *r.ExternalID)
	if err != nil {
		p.log.Warn("radarr QueueByMovie error", "id", r.ID, "external_id", *r.ExternalID, "err", err)
		return
	}
	if len(queue) > 0 {
		transitioned, err := d.Store.MarkDownloading(ctx, r.ID)
		if err != nil {
			p.log.Warn("MarkDownloading failed", "id", r.ID, "err", err)
			return
		}
		if transitioned {
			d.Events.Downloading(ctx, r.ID)
		}
		return
	}

	// 5. Not imported, not in queue — stale check.
	p.maybeMarkStale(ctx, d, r, now)
}

// pollTV handles one TV row against Sonarr.
func (p *Poller) pollTV(ctx context.Context, d *Deps, r *store.Request, a *store.RegisteredArr, apiKey string, now time.Time) {
	// 1. No external ID → nothing to query yet.
	if r.ExternalID == nil {
		return
	}

	c := d.Sonarr(a.URL, apiKey)

	// 2. Fetch series from Sonarr.
	series, err := c.GetSeries(ctx, *r.ExternalID)
	if err != nil {
		p.log.Warn("sonarr GetSeries error", "id", r.ID, "external_id", *r.ExternalID, "err", err)
		return
	}

	// 3. 100% imported → imported transition.
	if series.Statistics.PercentOfEpisodes >= 100 {
		if r.Status != "imported" {
			if err := d.Store.MarkImported(ctx, r.ID); err != nil {
				p.log.Warn("MarkImported failed", "id", r.ID, "err", err)
				return
			}
			d.Events.Imported(ctx, r.ID)
		}
		return
	}

	// 4. Not fully imported — check queue.
	queue, err := c.QueueBySeries(ctx, *r.ExternalID)
	if err != nil {
		p.log.Warn("sonarr QueueBySeries error", "id", r.ID, "external_id", *r.ExternalID, "err", err)
		return
	}
	if len(queue) > 0 {
		transitioned, err := d.Store.MarkDownloading(ctx, r.ID)
		if err != nil {
			p.log.Warn("MarkDownloading failed", "id", r.ID, "err", err)
			return
		}
		if transitioned {
			d.Events.Downloading(ctx, r.ID)
		}
		return
	}

	// 5. Not imported, not in queue — stale check.
	p.maybeMarkStale(ctx, d, r, now)
}

// maybeMarkStale marks the row failed when it has been submitted long enough to
// be considered stuck. Uses the injected clock (p.now) so tests can control time.
func (p *Poller) maybeMarkStale(ctx context.Context, d *Deps, r *store.Request, now time.Time) {
	hours := d.StaleAfterHours
	if hours <= 0 {
		return
	}
	at := r.SubmittedAt
	if at == nil {
		at = &r.CreatedAt
	}
	cutoff := now.Add(-time.Duration(hours) * time.Hour)
	if at.Before(cutoff) {
		const msg = "stuck past staleness threshold"
		if err := d.Store.MarkFailed(ctx, r.ID, msg); err != nil {
			p.log.Warn("MarkFailed failed", "id", r.ID, "err", err)
			return
		}
		d.Events.Failed(ctx, r.ID, msg)
	}
}
