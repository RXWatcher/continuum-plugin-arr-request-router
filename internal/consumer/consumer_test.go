package consumer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/consumer"
)

type fakeSubmitter struct {
	called bool
	err    error
}

func (f *fakeSubmitter) HandleSubmitted(ctx context.Context, p map[string]any) error {
	f.called = true
	return f.err
}

type fakeCanceller struct {
	called bool
	err    error
}

func (f *fakeCanceller) HandleCancelled(ctx context.Context, p map[string]any) error {
	f.called = true
	return f.err
}

func TestDispatchSubmittedRoutesToSubmitter(t *testing.T) {
	s := &fakeSubmitter{}
	c := &fakeCanceller{}
	d := consumer.New(s, c, nil)
	if err := d.Handle(context.Background(), "plugin.continuum.requests.submitted", map[string]any{"requestId": "r1"}); err != nil {
		t.Fatal(err)
	}
	if !s.called || c.called {
		t.Fatalf("submit=%v cancel=%v", s.called, c.called)
	}
}

func TestDispatchCancelledRoutesToCanceller(t *testing.T) {
	s := &fakeSubmitter{}
	c := &fakeCanceller{}
	d := consumer.New(s, c, nil)
	if err := d.Handle(context.Background(), "plugin.continuum.requests.cancelled", map[string]any{"requestId": "r1"}); err != nil {
		t.Fatal(err)
	}
	if s.called || !c.called {
		t.Fatalf("submit=%v cancel=%v", s.called, c.called)
	}
}

func TestDispatchUnknownEventIsNoOpAndDoesNotError(t *testing.T) {
	s := &fakeSubmitter{}
	c := &fakeCanceller{}
	d := consumer.New(s, c, nil)
	if err := d.Handle(context.Background(), "library.media_added", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if s.called || c.called {
		t.Fatalf("expected no handler called: submit=%v cancel=%v", s.called, c.called)
	}
}

func TestDispatchPropagatesHandlerError(t *testing.T) {
	s := &fakeSubmitter{err: errors.New("boom")}
	c := &fakeCanceller{}
	d := consumer.New(s, c, nil)
	err := d.Handle(context.Background(), "plugin.continuum.requests.submitted", map[string]any{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("got err %v", err)
	}
}

func TestDispatchNilLoggerIsAccepted(t *testing.T) {
	// Confirms nil logger is replaced with a null logger inside New.
	d := consumer.New(&fakeSubmitter{}, &fakeCanceller{}, nil)
	if err := d.Handle(context.Background(), "no.such.event", map[string]any{}); err != nil {
		t.Fatal(err)
	}
}
