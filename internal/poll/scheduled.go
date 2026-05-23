package poll

import (
	"context"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// ScheduledServer implements pluginv1.ScheduledTaskServer by delegating to
// Poller.Run. Register it with the gRPC server so the host can trigger poll
// runs on-demand (e.g. from the admin "Run now" button) in addition to the
// periodic background loop driven by poll_interval_seconds.
type ScheduledServer struct {
	pluginv1.UnimplementedScheduledTaskServer
	Poller *Poller
}

// Run satisfies pluginv1.ScheduledTaskServer. It delegates to Poller.Run and
// wraps any error as a gRPC status error.
func (s *ScheduledServer) Run(ctx context.Context, _ *pluginv1.RunScheduledTaskRequest) (*pluginv1.RunScheduledTaskResponse, error) {
	if s.Poller == nil {
		return &pluginv1.RunScheduledTaskResponse{}, nil
	}
	if err := s.Poller.Run(ctx); err != nil {
		return nil, err
	}
	return &pluginv1.RunScheduledTaskResponse{}, nil
}

// Tick is a convenience alias so callers that drive the Poller on an internal
// timer can call a named method without importing the SDK types.
func (p *Poller) Tick(ctx context.Context) error {
	return p.Run(ctx)
}
