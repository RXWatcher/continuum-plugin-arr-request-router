// Package event publishes named events into continuum's event hub via the
// SDK's RuntimeHost client. The host stamps `plugin.<plugin_id>.` in front of
// the supplied name, so callers pass the unprefixed leaf (e.g. "submitted",
// "cancelled"). Failures are logged but never bubble up to the caller —
// persisted state is the source of truth.
package event

import (
	"context"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtimehost"
)

// Publisher wraps a *runtimehost.Client. Construct once at plugin startup; safe
// for concurrent use.
type Publisher struct {
	host   *runtimehost.Client
	logger hclog.Logger
}

func New(host *runtimehost.Client, logger hclog.Logger) *Publisher {
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	return &Publisher{host: host, logger: logger}
}

// Publish fires an event into the host. If host is nil (broker not yet
// bound — very brief startup window) or the publish fails, the failure is
// logged and Publish returns. Callers do not need to handle errors.
func (p *Publisher) Publish(ctx context.Context, name string, payload map[string]any) {
	if p.host == nil {
		p.logger.Warn("host not bound; skipping event", "name", name)
		return
	}
	if err := p.host.PublishEvent(ctx, name, payload); err != nil {
		p.logger.Warn("publish event", "name", name, "err", err)
	}
}

// Submitted publishes the submitted event when a routing request is accepted.
func (p *Publisher) Submitted(ctx context.Context, requestID string) {
	p.Publish(ctx, "submitted", map[string]any{"requestId": requestID})
}

// Downloading publishes the downloading event when an arr instance begins
// fetching the media.
func (p *Publisher) Downloading(ctx context.Context, requestID string) {
	p.Publish(ctx, "downloading", map[string]any{"requestId": requestID})
}

// Imported publishes the imported event when an arr instance has successfully
// imported the media.
func (p *Publisher) Imported(ctx context.Context, requestID string) {
	p.Publish(ctx, "imported", map[string]any{"requestId": requestID})
}

// Failed publishes the failed event. The error string is included so the
// requests plugin can surface a useful message to the user.
func (p *Publisher) Failed(ctx context.Context, requestID, error string) {
	p.Publish(ctx, "failed", map[string]any{"requestId": requestID, "error": error})
}

// Cancelled publishes the cancelled event when a request is cancelled before
// or during download.
func (p *Publisher) Cancelled(ctx context.Context, requestID string) {
	p.Publish(ctx, "cancelled", map[string]any{"requestId": requestID})
}

// Unrouted publishes the unrouted terminal event when no registry entry
// matches the routing request. The error string is included so the requests
// plugin can surface a useful message to the user.
func (p *Publisher) Unrouted(ctx context.Context, requestID, error string) {
	p.Publish(ctx, "unrouted", map[string]any{"requestId": requestID, "error": error})
}
