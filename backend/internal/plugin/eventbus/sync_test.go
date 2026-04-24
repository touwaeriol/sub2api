//go:build unit

package eventbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

const testTopic = "test.sync"

func testSchema(kind plugin.EventKind) plugin.EventSchema {
	return plugin.EventSchema{
		Topic:          testTopic,
		Kind:           kind,
		PayloadExample: &plugin.Account{},
	}
}

func newSyncBus(t *testing.T) *Bus {
	t.Helper()
	r := NewRegistry()
	if err := r.Register(testSchema(plugin.EventKindSyncHook)); err != nil {
		t.Fatalf("register: %v", err)
	}
	b, err := NewBus(BusOptions{Registry: r})
	if err != nil {
		t.Fatalf("NewBus: %v", err)
	}
	return b
}

func TestSyncDispatcherPriorityOrder(t *testing.T) {
	b := newSyncBus(t)
	var order []int
	var mu sync.Mutex
	add := func(n int) {
		mu.Lock()
		order = append(order, n)
		mu.Unlock()
	}
	subs := []struct {
		prio int
		tag  string
	}{{prio: 100, tag: "late"}, {prio: 10, tag: "early"}, {prio: 50, tag: "mid"}}
	for i, s := range subs {
		i, s := i, s
		_ = b.Subscribe(plugin.EventSubscription{
			Topic:         testTopic,
			Kind:          plugin.EventKindSyncHook,
			Priority:      s.prio,
			SubscriberTag: s.tag,
			Handler: func(ctx context.Context, payload any) error {
				add(i)
				return nil
			},
		})
	}
	if err := b.PublishSyncHook(context.Background(), testTopic, &plugin.Account{ID: 1}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// Subs were added in the order (late=0, early=1, mid=2); sorted by prio
	// ascending we expect early(1), mid(2), late(0).
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 0 {
		t.Fatalf("priority order wrong: %v", order)
	}
}

func TestSyncDispatcherAbortsOnFirstError(t *testing.T) {
	b := newSyncBus(t)
	var firstRan, secondRan atomic.Bool
	want := errors.New("veto")
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: testTopic, Kind: plugin.EventKindSyncHook, Priority: 1,
		Handler: func(ctx context.Context, p any) error {
			firstRan.Store(true)
			return want
		},
	})
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: testTopic, Kind: plugin.EventKindSyncHook, Priority: 2,
		Handler: func(ctx context.Context, p any) error {
			secondRan.Store(true)
			return nil
		},
	})
	err := b.PublishSyncHook(context.Background(), testTopic, &plugin.Account{})
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped veto error, got %v", err)
	}
	if !firstRan.Load() {
		t.Fatal("first handler did not run")
	}
	if secondRan.Load() {
		t.Fatal("second handler ran despite abort")
	}
}

func TestSyncDispatcherHandlerTimeout(t *testing.T) {
	b := newSyncBus(t)
	b.SyncDispatcher().SetHandlerTimeout(50 * time.Millisecond)
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: testTopic, Kind: plugin.EventKindSyncHook,
		Handler: func(ctx context.Context, p any) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				return nil
			}
		},
	})
	start := time.Now()
	err := b.PublishSyncHook(context.Background(), testTopic, &plugin.Account{})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("timeout not enforced (took %v)", elapsed)
	}
}

func TestSyncHookTxPropagation(t *testing.T) {
	b := newSyncBus(t)
	type fakeTx struct{ id int }
	tx := &fakeTx{id: 42}
	var seenID int
	_ = b.Subscribe(plugin.EventSubscription{
		Topic: testTopic, Kind: plugin.EventKindSyncHook,
		Handler: func(ctx context.Context, p any) error {
			v, ok := plugin.SyncHookTxFrom(ctx)
			if !ok {
				return errors.New("no tx")
			}
			seenID = v.(*fakeTx).id
			return nil
		},
	})
	ctx := plugin.WithSyncHookTx(context.Background(), tx)
	if err := b.PublishSyncHook(ctx, testTopic, &plugin.Account{}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if seenID != 42 {
		t.Fatalf("tx not propagated, seenID=%d", seenID)
	}
}
