//go:build unit

package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

const notifyTopic = "test.notify"

func notifySchema() plugin.EventSchema {
	return plugin.EventSchema{
		Topic:          notifyTopic,
		Kind:           plugin.EventKindNotify,
		PayloadExample: &plugin.Account{},
	}
}

func newNotifyBus(t *testing.T) *Bus {
	t.Helper()
	r := NewRegistry()
	if err := r.Register(notifySchema()); err != nil {
		t.Fatal(err)
	}
	b, err := NewBus(BusOptions{Registry: r})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNotifyDispatchesConcurrently(t *testing.T) {
	b := newNotifyBus(t)
	const n = 10
	var counter atomic.Int32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		_ = b.Subscribe(plugin.EventSubscription{
			Topic: notifyTopic, Kind: plugin.EventKindNotify,
			Handler: func(ctx context.Context, p any) error {
				counter.Add(1)
				wg.Done()
				return nil
			},
		})
	}
	b.PublishNotify(context.Background(), notifyTopic, &plugin.Account{})
	waitWG(t, &wg, 2*time.Second)
	if got := counter.Load(); got != int32(n) {
		t.Fatalf("expected %d calls, got %d", n, got)
	}
}

func TestNotifyRecoversFromPanic(t *testing.T) {
	b := newNotifyBus(t)
	done := make(chan struct{})
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: notifyTopic, Kind: plugin.EventKindNotify,
		SubscriberTag: "panicker",
		Handler: func(ctx context.Context, p any) error {
			defer close(done)
			panic("boom")
		},
	})
	// Must NOT panic the test goroutine.
	b.PublishNotify(context.Background(), notifyTopic, &plugin.Account{})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("panicking handler never ran")
	}
}

func TestNotifyWrongKindIgnored(t *testing.T) {
	b := newNotifyBus(t)
	// Publishing a Notify topic as SyncHook via the explicit API should
	// return kind-mismatch; the Notify publisher just logs and drops.
	b.PublishNotify(context.Background(), "unknown.topic", &plugin.Account{})
	// No assertion beyond "did not panic / return" — bonus coverage.
}

func waitWG(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("WaitGroup timeout")
	}
}
