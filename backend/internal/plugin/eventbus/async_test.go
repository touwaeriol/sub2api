//go:build unit

package eventbus

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

const asyncTopic = "test.async"

func asyncSchema() plugin.EventSchema {
	return plugin.EventSchema{
		Topic:          asyncTopic,
		Kind:           plugin.EventKindAsyncHook,
		PayloadExample: &plugin.Account{},
	}
}

func newAsyncBus(t *testing.T, maxAttempts int) (*Bus, *InMemoryJobQueue, *InMemoryDeadLetterRepo) {
	t.Helper()
	r := NewRegistry()
	if err := r.Register(asyncSchema()); err != nil {
		t.Fatalf("register schema: %v", err)
	}
	queue := NewInMemoryJobQueue(1)
	dl := NewInMemoryDeadLetterRepo()
	b, err := NewBus(BusOptions{
		Registry:        r,
		JobQueue:        queue,
		HandlerRegistry: queue,
		DeadLetterRepo:  dl,
	})
	if err != nil {
		t.Fatalf("NewBus: %v", err)
	}
	// Fast retries so the whole test runs in < 1s.
	b.AsyncDispatcher().SetRetryPolicy(maxAttempts, 5*time.Millisecond, 10*time.Millisecond)
	if err := b.AsyncDispatcher().Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	return b, queue, dl
}

func waitUntil(t *testing.T, fn func() bool, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("waitUntil: condition never satisfied")
}

func TestAsyncDispatcherRetriesThenDeadLetters(t *testing.T) {
	b, queue, dl := newAsyncBus(t, 3)
	defer queue.Stop()

	var calls atomic.Int32
	wantErr := errors.New("boom")
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: asyncTopic, Kind: plugin.EventKindAsyncHook,
		SubscriberTag: "flaky",
		Handler: func(ctx context.Context, p any) error {
			calls.Add(1)
			return wantErr
		},
	})

	if err := b.PublishAsyncHook(context.Background(), asyncTopic, &plugin.Account{ID: 7}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	waitUntil(t, func() bool { return dl.Len() == 1 }, 2*time.Second)
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}

	entries, err := dl.List(context.Background(), DeadLetterFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if entries[0].Topic != asyncTopic {
		t.Fatalf("wrong topic: %s", entries[0].Topic)
	}
	if entries[0].SubscriberTag != "flaky" {
		t.Fatalf("subscriber tag lost: %q", entries[0].SubscriberTag)
	}
	if entries[0].LastError != wantErr.Error() {
		t.Fatalf("last_error mismatch: %q", entries[0].LastError)
	}
}

func TestAsyncDispatcherSucceedsOnFirstTry(t *testing.T) {
	b, queue, dl := newAsyncBus(t, 5)
	defer queue.Stop()

	var calls atomic.Int32
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: asyncTopic, Kind: plugin.EventKindAsyncHook,
		Handler: func(ctx context.Context, p any) error {
			calls.Add(1)
			return nil
		},
	})
	if err := b.PublishAsyncHook(context.Background(), asyncTopic, &plugin.Account{ID: 1}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	waitUntil(t, func() bool { return calls.Load() == 1 }, time.Second)
	if dl.Len() != 0 {
		t.Fatalf("dead-letter unexpectedly populated")
	}
}

func TestAsyncDispatcherPayloadDecoded(t *testing.T) {
	b, queue, _ := newAsyncBus(t, 1)
	defer queue.Stop()

	var seen int64
	done := make(chan struct{})
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: asyncTopic, Kind: plugin.EventKindAsyncHook,
		Handler: func(ctx context.Context, p any) error {
			acct, ok := p.(*plugin.Account)
			if !ok {
				t.Errorf("wrong payload type %T", p)
				close(done)
				return nil
			}
			seen = acct.ID
			close(done)
			return nil
		},
	})
	_ = b.PublishAsyncHook(context.Background(), asyncTopic, &plugin.Account{ID: 99})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler never invoked")
	}
	if seen != 99 {
		t.Fatalf("payload not propagated, seen=%d", seen)
	}
}
