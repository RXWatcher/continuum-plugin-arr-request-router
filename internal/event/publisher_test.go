package event

import (
	"context"
	"testing"
)

// nilPublisher returns a Publisher with a nil host. The Publish method guards
// against nil host, so all helpers must return without panicking — this is the
// only seam available without a running gRPC server.
func nilPublisher() *Publisher {
	return New(nil, nil)
}

func TestNew_NilLoggerDoesNotPanic(t *testing.T) {
	p := New(nil, nil)
	if p == nil {
		t.Fatal("New returned nil")
	}
}

func TestSubmitted_NilHost(t *testing.T) {
	nilPublisher().Submitted(context.Background(), "req-1")
}

func TestDownloading_NilHost(t *testing.T) {
	nilPublisher().Downloading(context.Background(), "req-2")
}

func TestImported_NilHost(t *testing.T) {
	nilPublisher().Imported(context.Background(), "req-3")
}

func TestFailed_NilHost(t *testing.T) {
	nilPublisher().Failed(context.Background(), "req-4", "disk full")
}

func TestCancelled_NilHost(t *testing.T) {
	nilPublisher().Cancelled(context.Background(), "req-5")
}

func TestUnrouted_NilHost(t *testing.T) {
	nilPublisher().Unrouted(context.Background(), "req-6", "no registry match for radarr 4K")
}

// TestPublish_LogsAndSkipsWhenNilHost verifies the nil-host guard path does not
// panic; the warn log is swallowed by the null logger injected by New.
func TestPublish_LogsAndSkipsWhenNilHost(t *testing.T) {
	p := nilPublisher()
	p.Publish(context.Background(), "test-event", map[string]any{"k": "v"})
}
