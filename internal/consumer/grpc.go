package consumer

import (
	"context"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// EventServer wraps a Dispatcher and implements pluginv1.EventConsumerServer
// so the SDK's gRPC layer can dispatch events into the plugin.
type EventServer struct {
	pluginv1.UnimplementedEventConsumerServer
	d *Dispatcher
}

// NewEventServer returns a new EventServer backed by d.
func NewEventServer(d *Dispatcher) *EventServer {
	return &EventServer{d: d}
}

// HandleEvent satisfies pluginv1.EventConsumerServer. It extracts the event
// name and payload from the gRPC request and delegates to Dispatcher.Handle.
// Errors from the handler are swallowed (logged inside Dispatcher); event
// processing must not block silo's dispatcher goroutine.
func (s *EventServer) HandleEvent(ctx context.Context, req *pluginv1.HandleEventRequest) (*pluginv1.HandleEventResponse, error) {
	if req.GetPayload() == nil {
		return &pluginv1.HandleEventResponse{}, nil
	}
	payload := req.GetPayload().AsMap()
	_ = s.d.Handle(ctx, req.GetEventName(), payload)
	return &pluginv1.HandleEventResponse{}, nil
}
