//go:build unit

package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

func newBusWithCoreSchemas(t *testing.T) *Bus {
	t.Helper()
	b, err := NewBus(BusOptions{})
	if err != nil {
		t.Fatalf("NewBus: %v", err)
	}
	return b
}

func TestSubscribeRejectsKindMismatch(t *testing.T) {
	b := newBusWithCoreSchemas(t)
	// TopicAccountBeforeDelete is SyncHook; subscribe as Notify must fail.
	err := b.Subscribe(plugin.EventSubscription{
		Topic:   plugin.TopicAccountBeforeDelete,
		Kind:    plugin.EventKindNotify,
		Handler: func(ctx context.Context, p any) error { return nil },
	})
	if !errors.Is(err, plugin.ErrEventKindMismatch) {
		t.Fatalf("expected ErrEventKindMismatch, got %v", err)
	}
}

func TestSubscribeRejectsUnknownTopic(t *testing.T) {
	b := newBusWithCoreSchemas(t)
	err := b.Subscribe(plugin.EventSubscription{
		Topic:   "not.registered",
		Kind:    plugin.EventKindNotify,
		Handler: func(ctx context.Context, p any) error { return nil },
	})
	if !errors.Is(err, plugin.ErrEventTopicUnknown) {
		t.Fatalf("expected ErrEventTopicUnknown, got %v", err)
	}
}

func TestSubscribeRejectsNilHandler(t *testing.T) {
	b := newBusWithCoreSchemas(t)
	err := b.Subscribe(plugin.EventSubscription{
		Topic: plugin.TopicAccountCreated,
		Kind:  plugin.EventKindNotify,
	})
	if !errors.Is(err, plugin.ErrEventHandlerSignature) {
		t.Fatalf("expected ErrEventHandlerSignature, got %v", err)
	}
}

func TestPublishRejectsUnknownTopic(t *testing.T) {
	b := newBusWithCoreSchemas(t)
	err := b.Publish(context.Background(), "ghost.topic", &plugin.Account{})
	if !errors.Is(err, plugin.ErrEventTopicUnknown) {
		t.Fatalf("expected ErrEventTopicUnknown, got %v", err)
	}
}

func TestPublishRejectsWrongPayloadType(t *testing.T) {
	b := newBusWithCoreSchemas(t)
	// TopicAccountCreated expects *Account; pass OrderEvent.
	err := b.Publish(context.Background(), plugin.TopicAccountCreated, plugin.OrderEvent{Source: "x"})
	if !errors.Is(err, plugin.ErrEventHandlerSignature) {
		t.Fatalf("expected ErrEventHandlerSignature, got %v", err)
	}
}

func TestPublishSyncHookRejectsNonSyncTopic(t *testing.T) {
	b := newBusWithCoreSchemas(t)
	// TopicAccountCreated is Notify, not SyncHook.
	err := b.PublishSyncHook(context.Background(), plugin.TopicAccountCreated, &plugin.Account{})
	if !errors.Is(err, plugin.ErrEventKindMismatch) {
		t.Fatalf("expected ErrEventKindMismatch, got %v", err)
	}
}

func TestPublishAsyncHookWithoutDispatcherErrors(t *testing.T) {
	// Register an Async schema on a bus that has no JobQueue wired.
	r := NewRegistry()
	const topic = "test.async.missing"
	_ = r.Register(plugin.EventSchema{
		Topic: topic, Kind: plugin.EventKindAsyncHook,
		PayloadExample: &plugin.Account{},
	})
	b, err := NewBus(BusOptions{Registry: r})
	if err != nil {
		t.Fatalf("NewBus: %v", err)
	}
	if err := b.PublishAsyncHook(context.Background(), topic, &plugin.Account{}); err == nil {
		t.Fatal("expected error when async dispatcher is not configured")
	}
}

func TestBackoffForGrowsAndClamps(t *testing.T) {
	initial := 100 * time.Millisecond
	max := 800 * time.Millisecond
	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		800 * time.Millisecond,
	}
	for i, attempt := range []int{1, 2, 3, 4, 10} {
		got := backoffFor(attempt, initial, max)
		if got != want[i] {
			t.Fatalf("attempt %d: got %v, want %v", attempt, got, want[i])
		}
	}
}

func TestSyncHookTxFromNilContext(t *testing.T) {
	if _, ok := plugin.SyncHookTxFrom(context.Background()); ok {
		t.Fatal("expected no tx in plain context")
	}
}
