package consumer

import (
	"context"

	"github.com/hashicorp/go-hclog"
)

// Submitter handles plugin.continuum.requests.submitted events.
// Implemented by *SubmitHandler in submit.go (Task 7.2).
type Submitter interface {
	HandleSubmitted(ctx context.Context, payload map[string]any) error
}

// Canceller handles plugin.continuum.requests.cancelled events.
// Implemented by *CancelHandler in cancel.go (Task 7.4).
type Canceller interface {
	HandleCancelled(ctx context.Context, payload map[string]any) error
}

// Dispatcher routes events from the SDK's event_consumer.v1 capability
// to the right handler. Unknown events are silently ignored (logged at
// debug).
type Dispatcher struct {
	submit Submitter
	cancel Canceller
	log    hclog.Logger
}

// New constructs a Dispatcher. A nil log is replaced with a null logger.
func New(s Submitter, c Canceller, log hclog.Logger) *Dispatcher {
	if log == nil {
		log = hclog.NewNullLogger()
	}
	return &Dispatcher{submit: s, cancel: c, log: log}
}

// Handle is the SDK-facing entrypoint. Returns an error if the matched
// handler returned one; nil for unknown events.
func (d *Dispatcher) Handle(ctx context.Context, eventName string, payload map[string]any) error {
	switch eventName {
	case "plugin.continuum.requests.submitted":
		return d.submit.HandleSubmitted(ctx, payload)
	case "plugin.continuum.requests.cancelled":
		return d.cancel.HandleCancelled(ctx, payload)
	}
	d.log.Debug("ignoring event", "name", eventName)
	return nil
}
